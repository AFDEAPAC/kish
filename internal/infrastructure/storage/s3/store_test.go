package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/AFDEAPAC/kish/internal/infrastructure/storage"
)

type fakeClient struct {
	putInput    *awss3.PutObjectInput
	getInput    *awss3.GetObjectInput
	deleteInput *awss3.DeleteObjectInput
	getOutput   *awss3.GetObjectOutput
	getErr      error
}

func (c *fakeClient) PutObject(_ context.Context, in *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	c.putInput = in
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
