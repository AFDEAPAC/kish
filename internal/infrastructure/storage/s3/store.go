// Package s3 implements ObjectStore backed by AWS S3 or an S3-compatible service.
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

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

// PutObject writes r to S3 under key, replacing any existing object.
func (s *Store) PutObject(ctx context.Context, key string, r io.Reader, _ int64, contentType string) error {
	objectKey, err := s.objectKey(key)
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = s.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        r,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put %q: %w", key, err)
	}
	return nil
}

// GetObject retrieves an object stream and metadata from S3.
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
