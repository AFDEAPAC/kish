// Package s3 implements ObjectStore backed by AWS S3 or an S3-compatible
// service such as MinIO.
//
// Operational assumptions baked into this implementation:
//
//   - Credentials come from the AWS SDK default chain unless static
//     AccessKeyID/SecretAccessKey are provided in Options. Operators that
//     deploy in EKS/EC2 should leave the static fields empty and rely on
//     IRSA / instance profiles.
//   - TLS verification follows the configured TLSOptions. Self-signed
//     buckets (MinIO behind an internal CA) require storage.s3.tls.ca_file
//     to be set; the package detects unknown-authority errors and rewrites
//     them with that hint.
//   - Outbound HTTP timeout is 60s (see tls.go). There is no automatic
//     retry loop; transient failures surface to the application layer.
//   - PutObject must know the content length up front, so non-seekable
//     readers are buffered fully in memory before being sent. Callers that
//     stream large artifacts should pre-wrap the body in an io.Seeker.
//   - GetObject returns the S3 LastModified timestamp when present and
//     falls back to time.Now() so newly written objects whose metadata has
//     not yet propagated still produce a non-zero ObjectInfo.UpdatedAt.
//   - DeleteObject treats missing keys as success because S3 itself does;
//     no extra HEAD round-trip is performed.
//   - IAM permissions required: s3:GetObject, s3:PutObject, s3:DeleteObject
//     against the configured bucket and optional prefix. Listing is not
//     used.
//
// All Store methods are safe for concurrent use because the underlying
// aws-sdk-go-v2 client is.
package s3

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/AFDEAPAC/kish/internal/infrastructure/storage"
)

// Options configures an S3-compatible ObjectStore.
type Options struct {
	Bucket          string
	Region          string
	Endpoint        string
	Prefix          string
	ForcePathStyle  bool
	AccessKeyID     string
	SecretAccessKey string
	TLS             TLSOptions
}

// Client is the narrow subset of the AWS S3 client used by Store.
type Client interface {
	PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
	GetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	DeleteObject(context.Context, *awss3.DeleteObjectInput, ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error)
}

// Store is an S3-compatible implementation of storage.ObjectStore.
type Store struct {
	client Client
	bucket string
	prefix string
}

// New constructs an S3 ObjectStore using the AWS SDK default credential chain
// unless static credentials are provided in Options.
func New(ctx context.Context, opts Options) (*Store, error) {
	if opts.Region == "" {
		opts.Region = "us-east-1"
	}
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(opts.Region),
	}
	httpClient, err := NewHTTPClient(opts.TLS)
	if err != nil {
		return nil, fmt.Errorf("s3 tls: %w", err)
	}
	loadOpts = append(loadOpts, awsconfig.WithHTTPClient(httpClient))
	if opts.AccessKeyID != "" || opts.SecretAccessKey != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(opts.AccessKeyID, opts.SecretAccessKey, "")))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3 config: %w", err)
	}
	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		if opts.Endpoint != "" {
			o.BaseEndpoint = aws.String(opts.Endpoint)
		}
		o.UsePathStyle = opts.ForcePathStyle
		// MinIO and other S3-compatible stores can reject SDK checksum trailers
		// on simple PUT requests. Keep checksums only when S3 requires them.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})
	return NewWithClient(client, opts)
}

// NewWithClient constructs a Store with a caller-provided client. It is used by tests.
func NewWithClient(client Client, opts Options) (*Store, error) {
	if client == nil {
		return nil, fmt.Errorf("s3 client is required")
	}
	if opts.Bucket == "" {
		return nil, fmt.Errorf("storage.s3.bucket is required")
	}
	prefix, err := normalizePrefix(opts.Prefix)
	if err != nil {
		return nil, err
	}
	return &Store{client: client, bucket: opts.Bucket, prefix: prefix}, nil
}

// PutObject uploads r to S3 under key as a single-shot PUT, overwriting any
// existing object. The implementation does not use multipart upload.
//
// Non-seekable readers are fully buffered in memory before the request is
// dispatched because S3 requires a known ContentLength on PutObject and
// the SDK cannot rewind a streaming reader on retry. Callers uploading
// large artifacts should pass an io.Seeker (e.g. *os.File or *bytes.Reader)
// to keep memory bounded.
//
// On error PutObject maps three categories before returning:
//   - storage backend reports out-of-space → wraps storage.ErrInsufficientStorage.
//   - TLS unknown-authority → rewritten with a hint to configure
//     storage.s3.tls.ca_file (production deployments behind self-signed
//     buckets always hit this once before being configured).
//   - everything else is returned verbatim with a contextualised wrap.
func (s *Store) PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	objectKey, err := s.objectKey(key)
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	body, resolvedSize, err := seekableBody(r, size)
	if err != nil {
		return fmt.Errorf("prepare s3 body %q: %w", key, err)
	}
	input := &awss3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        body,
		ContentType: aws.String(contentType),
	}
	if resolvedSize >= 0 {
		input.ContentLength = aws.Int64(resolvedSize)
	}
	_, err = s.client.PutObject(ctx, input)
	if err != nil {
		if isInsufficientStorage(err) {
			return fmt.Errorf("s3 put %q: %w: %v", key, storage.ErrInsufficientStorage, err)
		}
		if isTLSVerificationError(err) {
			return fmt.Errorf("s3 tls verification failed while putting %q: configure storage.s3.tls.ca_file or ensure the image contains CA certificates: %w", key, err)
		}
		return fmt.Errorf("s3 put %q: %w", key, err)
	}
	return nil
}

func seekableBody(r io.Reader, size int64) (io.Reader, int64, error) {
	if _, ok := r.(io.Seeker); ok {
		return r, size, nil
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, -1, err
	}
	return bytes.NewReader(data), int64(len(data)), nil
}

// GetObject opens an S3 GetObject stream and returns its metadata. The
// returned io.ReadCloser must be closed by the caller even on early
// returns from the HTTP handler.
//
// Returns storage.ErrObjectNotFound when S3 reports NoSuchKey / 404. TLS
// unknown-authority errors are rewritten with the same hint as PutObject.
// The reported ObjectInfo.UpdatedAt prefers S3's LastModified header; if
// the response omits it (some S3-compatible backends do for very fresh
// objects) UpdatedAt falls back to time.Now() so callers always observe a
// non-zero timestamp.
func (s *Store) GetObject(ctx context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	objectKey, err := s.objectKey(key)
	if err != nil {
		return nil, storage.ObjectInfo{}, err
	}
	out, err := s.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, storage.ObjectInfo{}, storage.ErrObjectNotFound
		}
		if isTLSVerificationError(err) {
			return nil, storage.ObjectInfo{}, fmt.Errorf("s3 tls verification failed while getting %q: configure storage.s3.tls.ca_file or ensure the image contains CA certificates: %w", key, err)
		}
		return nil, storage.ObjectInfo{}, fmt.Errorf("s3 get %q: %w", key, err)
	}
	info := storage.ObjectInfo{
		Key:         key,
		Size:        aws.ToInt64(out.ContentLength),
		ContentType: aws.ToString(out.ContentType),
		UpdatedAt:   time.Now().UTC(),
	}
	if out.LastModified != nil {
		info.UpdatedAt = out.LastModified.UTC()
	}
	return out.Body, info, nil
}

// DeleteObject removes an object from S3. S3 delete is idempotent for missing keys.
func (s *Store) DeleteObject(ctx context.Context, key string) error {
	objectKey, err := s.objectKey(key)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		if isTLSVerificationError(err) {
			return fmt.Errorf("s3 tls verification failed while deleting %q: configure storage.s3.tls.ca_file or ensure the image contains CA certificates: %w", key, err)
		}
		return fmt.Errorf("s3 delete %q: %w", key, err)
	}
	return nil
}

func (s *Store) objectKey(key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	if s.prefix == "" {
		return key, nil
	}
	return s.prefix + "/" + key, nil
}

func normalizePrefix(prefix string) (string, error) {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return "", nil
	}
	if err := validateKey(prefix); err != nil {
		return "", fmt.Errorf("storage.s3.prefix: %w", err)
	}
	return prefix, nil
}

func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("storage key must not be empty")
	}
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("storage key %q must not start with /", key)
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" {
			return fmt.Errorf("storage key %q must not contain empty path segments", key)
		}
		if part == "." || part == ".." {
			return fmt.Errorf("storage key %q must not contain relative path segments", key)
		}
	}
	return nil
}

func isNotFound(err error) bool {
	if errors.Is(err, storage.ErrObjectNotFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}

func isInsufficientStorage(err error) bool {
	if errors.Is(err, storage.ErrInsufficientStorage) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "XMinioStorageFull":
			return true
		}
	}
	var responseErr *smithyhttp.ResponseError
	return errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == http.StatusInsufficientStorage
}

func isTLSVerificationError(err error) bool {
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return true
	}
	return strings.Contains(err.Error(), "certificate signed by unknown authority")
}
