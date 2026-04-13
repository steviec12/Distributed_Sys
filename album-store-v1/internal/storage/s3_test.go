package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type fakeAPIError struct {
	code string
	msg  string
}

func (e fakeAPIError) Error() string     { return e.msg }
func (e fakeAPIError) ErrorCode() string { return e.code }
func (e fakeAPIError) ErrorMessage() string {
	return e.msg
}
func (e fakeAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

type fakeS3Client struct {
	putInput    *s3.PutObjectInput
	putErr      error
	headInput   *s3.HeadObjectInput
	headErr     error
	deleteInput *s3.DeleteObjectInput
	deleteErr   error
}

func (f *fakeS3Client) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putInput = params
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3Client) HeadObject(_ context.Context, params *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	f.headInput = params
	if f.headErr != nil {
		return nil, f.headErr
	}
	return &s3.HeadObjectOutput{}, nil
}

func (f *fakeS3Client) DeleteObject(_ context.Context, params *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.deleteInput = params
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &s3.DeleteObjectOutput{}, nil
}

func TestPutObjectPassesBucketKeyBodyAndContentType(t *testing.T) {
	client := &fakeS3Client{}
	storage := NewS3StorageWithClient(client, "album-store-bucket", "https://album-store-bucket.s3.us-west-2.amazonaws.com/")

	body := strings.NewReader("image-bytes")
	if err := storage.PutObject(context.Background(), "uploads/photo 1.jpg", body, 11, "image/jpeg"); err != nil {
		t.Fatalf("put object: %v", err)
	}

	if client.putInput == nil {
		t.Fatal("expected put object to be called")
	}
	if aws.ToString(client.putInput.Bucket) != "album-store-bucket" {
		t.Fatalf("unexpected bucket %q", aws.ToString(client.putInput.Bucket))
	}
	if aws.ToString(client.putInput.Key) != "uploads/photo 1.jpg" {
		t.Fatalf("unexpected key %q", aws.ToString(client.putInput.Key))
	}
	if aws.ToString(client.putInput.ContentType) != "image/jpeg" {
		t.Fatalf("unexpected content type %q", aws.ToString(client.putInput.ContentType))
	}
	if client.putInput.Body != body {
		t.Fatal("expected original body reader to be passed through")
	}
}

func TestPutObjectPropagatesErrors(t *testing.T) {
	client := &fakeS3Client{putErr: errors.New("s3 down")}
	storage := NewS3StorageWithClient(client, "album-store-bucket", "https://album-store-bucket.s3.us-west-2.amazonaws.com/")

	if err := storage.PutObject(context.Background(), "uploads/photo.jpg", strings.NewReader("x"), 1, "image/jpeg"); err == nil {
		t.Fatal("expected put error")
	}
}

func TestObjectExistsReturnsTrueWhenHeadSucceeds(t *testing.T) {
	client := &fakeS3Client{}
	storage := NewS3StorageWithClient(client, "album-store-bucket", "https://album-store-bucket.s3.us-west-2.amazonaws.com/")

	exists, err := storage.ObjectExists(context.Background(), "uploads/photo.jpg")
	if err != nil {
		t.Fatalf("object exists: %v", err)
	}
	if !exists {
		t.Fatal("expected object to exist")
	}
	if client.headInput == nil || aws.ToString(client.headInput.Key) != "uploads/photo.jpg" {
		t.Fatalf("unexpected head input: %+v", client.headInput)
	}
}

func TestObjectExistsReturnsFalseForNotFound(t *testing.T) {
	client := &fakeS3Client{headErr: fakeAPIError{code: "NotFound", msg: "missing"}}
	storage := NewS3StorageWithClient(client, "album-store-bucket", "https://album-store-bucket.s3.us-west-2.amazonaws.com/")

	exists, err := storage.ObjectExists(context.Background(), "uploads/photo.jpg")
	if err != nil {
		t.Fatalf("object exists: %v", err)
	}
	if exists {
		t.Fatal("expected object to be reported missing")
	}
}

func TestObjectExistsPropagatesUnexpectedHeadErrors(t *testing.T) {
	client := &fakeS3Client{headErr: io.EOF}
	storage := NewS3StorageWithClient(client, "album-store-bucket", "https://album-store-bucket.s3.us-west-2.amazonaws.com/")

	if _, err := storage.ObjectExists(context.Background(), "uploads/photo.jpg"); err == nil {
		t.Fatal("expected head error")
	}
}

func TestDeleteObjectPassesBucketAndKey(t *testing.T) {
	client := &fakeS3Client{}
	storage := NewS3StorageWithClient(client, "album-store-bucket", "https://album-store-bucket.s3.us-west-2.amazonaws.com/")

	if err := storage.DeleteObject(context.Background(), "uploads/photo.jpg"); err != nil {
		t.Fatalf("delete object: %v", err)
	}

	if client.deleteInput == nil {
		t.Fatal("expected delete object to be called")
	}
	if aws.ToString(client.deleteInput.Bucket) != "album-store-bucket" || aws.ToString(client.deleteInput.Key) != "uploads/photo.jpg" {
		t.Fatalf("unexpected delete input: %+v", client.deleteInput)
	}
}

func TestDeleteObjectPropagatesErrors(t *testing.T) {
	client := &fakeS3Client{deleteErr: errors.New("delete failed")}
	storage := NewS3StorageWithClient(client, "album-store-bucket", "https://album-store-bucket.s3.us-west-2.amazonaws.com/")

	if err := storage.DeleteObject(context.Background(), "uploads/photo.jpg"); err == nil {
		t.Fatal("expected delete error")
	}
}

func TestPublicURLUsesConfiguredBaseAndEscapesKey(t *testing.T) {
	storage := NewS3StorageWithClient(&fakeS3Client{}, "album-store-bucket", "https://album-store-bucket.s3.us-west-2.amazonaws.com/")

	got := storage.PublicURL("uploads/photo 1.jpg")
	want := "https://album-store-bucket.s3.us-west-2.amazonaws.com/uploads/photo%201.jpg"
	if got != want {
		t.Fatalf("expected public URL %q, got %q", want, got)
	}
}
