package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/AFDEAPAC/kish/internal/infrastructure/storage"
)

type fakeClient struct {
	putInput             *awss3.PutObjectInput
	getInput             *awss3.GetObjectInput
	deleteInput          *awss3.DeleteObjectInput
	getOutput            *awss3.GetObjectOutput
	putErr               error
	getErr               error
	requireContentLength bool
	requireSeekableBody  bool
}

func (c *fakeClient) PutObject(_ context.Context, in *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	c.putInput = in
	if c.requireContentLength && in.ContentLength == nil {
		return nil, errors.New("content-length required")
	}
	if c.requireSeekableBody {
		if _, ok := in.Body.(io.Seeker); !ok {
			return nil, errors.New("seekable body required")
		}
	}
	if c.putErr != nil {
		return nil, c.putErr
	}
	return &awss3.PutObjectOutput{}, nil
}

func (c *fakeClient) GetObject(_ context.Context, in *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	c.getInput = in
	if c.getErr != nil {
		return nil, c.getErr
	}
	return c.getOutput, nil
}

func (c *fakeClient) DeleteObject(_ context.Context, in *awss3.DeleteObjectInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	c.deleteInput = in
	return &awss3.DeleteObjectOutput{}, nil
}

func TestPutObjectUsesPrefixedKeyAndContentType(t *testing.T) {
	client := &fakeClient{}
	store, err := NewWithClient(client, Options{Bucket: "bucket", Prefix: "/kish/artifacts/"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.PutObject(context.Background(), "testcases/tc1/artifacts/result.txt", bytes.NewReader([]byte("hello")), 5, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(client.putInput.Bucket); got != "bucket" {
		t.Fatalf("bucket = %q", got)
	}
	if got := aws.ToString(client.putInput.Key); got != "kish/artifacts/testcases/tc1/artifacts/result.txt" {
		t.Fatalf("key = %q", got)
	}
	if got := aws.ToString(client.putInput.ContentType); got != "text/plain" {
		t.Fatalf("content-type = %q", got)
	}
	if got := aws.ToInt64(client.putInput.ContentLength); got != 5 {
		t.Fatalf("content-length = %d", got)
	}
}

func TestPutObjectSetsKnownContentLengthForCompatibleStores(t *testing.T) {
	client := &fakeClient{requireContentLength: true}
	store, err := NewWithClient(client, Options{Bucket: "bucket"})
	if err != nil {
		t.Fatal(err)
	}

	err = store.PutObject(context.Background(), "testcases/tc1/artifacts/env.json", bytes.NewReader([]byte("hello")), 5, "application/json")
	if err != nil {
		t.Fatalf("expected known-size put to satisfy compatible store, got %v", err)
	}
	if got := aws.ToInt64(client.putInput.ContentLength); got != 5 {
		t.Fatalf("content-length = %d", got)
	}
}

func TestPutObjectBuffersNonSeekableBody(t *testing.T) {
	client := &fakeClient{requireSeekableBody: true}
	store, err := NewWithClient(client, Options{Bucket: "bucket"})
	if err != nil {
		t.Fatal(err)
	}

	err = store.PutObject(context.Background(), "testcases/tc1/artifacts/env.json", bytes.NewBufferString("hello"), 5, "application/json")
	if err != nil {
		t.Fatalf("expected non-seekable body to be buffered, got %v", err)
	}
	if _, ok := client.putInput.Body.(io.Seeker); !ok {
		t.Fatal("expected buffered body to be seekable")
	}
	if got := aws.ToInt64(client.putInput.ContentLength); got != 5 {
		t.Fatalf("content-length = %d", got)
	}
}

func TestPutObjectBuffersUnknownSizeBody(t *testing.T) {
	client := &fakeClient{requireContentLength: true, requireSeekableBody: true}
	store, err := NewWithClient(client, Options{Bucket: "bucket"})
	if err != nil {
		t.Fatal(err)
	}

	err = store.PutObject(context.Background(), "testcases/tc1/artifacts/env.json", bytes.NewBufferString("hello"), -1, "application/json")
	if err != nil {
		t.Fatalf("expected unknown-size body to be buffered with resolved length, got %v", err)
	}
	if got := aws.ToInt64(client.putInput.ContentLength); got != 5 {
		t.Fatalf("content-length = %d", got)
	}
	body, err := io.ReadAll(client.putInput.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q", body)
	}
}

func TestPutObjectMapsMinioStorageFull(t *testing.T) {
	client := &fakeClient{putErr: &smithy.GenericAPIError{Code: "XMinioStorageFull", Message: "storage full"}}
	store, err := NewWithClient(client, Options{Bucket: "bucket"})
	if err != nil {
		t.Fatal(err)
	}

	err = store.PutObject(context.Background(), "testcases/tc1/artifacts/env.json", bytes.NewReader([]byte("hello")), 5, "application/json")
	if !errors.Is(err, storage.ErrInsufficientStorage) {
		t.Fatalf("expected ErrInsufficientStorage, got %v", err)
	}
	if !strings.Contains(err.Error(), "XMinioStorageFull") {
		t.Fatalf("expected original provider code in error, got %v", err)
	}
}

func TestPutObjectMapsHTTPInsufficientStorage(t *testing.T) {
	client := &fakeClient{putErr: &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusInsufficientStorage}},
		Err:      &smithy.GenericAPIError{Code: "StorageFull", Message: "storage full"},
	}}
	store, err := NewWithClient(client, Options{Bucket: "bucket"})
	if err != nil {
		t.Fatal(err)
	}

	err = store.PutObject(context.Background(), "testcases/tc1/artifacts/env.json", bytes.NewReader([]byte("hello")), 5, "application/json")
	if !errors.Is(err, storage.ErrInsufficientStorage) {
		t.Fatalf("expected ErrInsufficientStorage, got %v", err)
	}
}

func TestGetObjectReturnsContentAndMetadata(t *testing.T) {
	updatedAt := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{getOutput: &awss3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader([]byte("artifact"))),
		ContentLength: aws.Int64(8),
		ContentType:   aws.String("text/plain"),
		LastModified:  &updatedAt,
	}}
	store, err := NewWithClient(client, Options{Bucket: "bucket"})
	if err != nil {
		t.Fatal(err)
	}

	rc, info, err := store.GetObject(context.Background(), "testcases/tc1/artifacts/result.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if string(body) != "artifact" {
		t.Fatalf("body = %q", body)
	}
	if info.Size != 8 || info.ContentType != "text/plain" || !info.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestGetObjectMapsNoSuchKey(t *testing.T) {
	client := &fakeClient{getErr: &smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}}
	store, err := NewWithClient(client, Options{Bucket: "bucket"})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = store.GetObject(context.Background(), "testcases/tc1/artifacts/missing.txt")
	if !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

func TestDeleteObjectUsesPrefixedKey(t *testing.T) {
	client := &fakeClient{}
	store, err := NewWithClient(client, Options{Bucket: "bucket", Prefix: "prefix"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteObject(context.Background(), "testcases/tc1/artifacts/result.txt"); err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(client.deleteInput.Key); got != "prefix/testcases/tc1/artifacts/result.txt" {
		t.Fatalf("delete key = %q", got)
	}
}

func TestValidateKeyRejectsUnsafeKeys(t *testing.T) {
	for _, key := range []string{"", "/absolute", "testcases//artifact", "../secret", "testcases/../secret", "testcases/./secret"} {
		if err := validateKey(key); err == nil {
			t.Fatalf("expected error for key %q", key)
		}
	}
}

func TestNewWithClientRequiresBucket(t *testing.T) {
	_, err := NewWithClient(&fakeClient{}, Options{})
	if err == nil {
		t.Fatal("expected missing bucket error")
	}
}
