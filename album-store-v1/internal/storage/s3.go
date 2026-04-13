package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// ObjectStorage is the S3-backed boundary shared by the API and worker layers.
type ObjectStorage interface {
	PutObject(ctx context.Context, key string, body io.Reader, contentLength int64, contentType string) error
	ObjectExists(ctx context.Context, key string) (bool, error)
	DeleteObject(ctx context.Context, key string) error
	PublicURL(key string) string
}

// s3API keeps the AWS client mockable in tests.
type s3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3Storage is the concrete object storage implementation used in V1.
type S3Storage struct {
	client        s3API
	bucket        string
	publicBaseURL string
}

// NewS3Storage builds an AWS-backed storage client from runtime config.
func NewS3Storage(ctx context.Context, region, bucket, publicBaseURL string) (*S3Storage, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        200,
			MaxIdleConnsPerHost: 200,
			IdleConnTimeout:     90 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UseAccelerate = true
	})
	return NewS3StorageWithClient(client, bucket, publicBaseURL), nil
}

// NewS3StorageWithClient is the test-friendly constructor.
func NewS3StorageWithClient(client s3API, bucket, publicBaseURL string) *S3Storage {
	return &S3Storage{
		client:        client,
		bucket:        bucket,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}
}

// PutObject stores the uploaded file bytes under one deterministic object key.
// When contentLength > 0 the SDK sends a single PUT with a known Content-Length
// instead of falling back to chunked transfer encoding.
func (s *S3Storage) PutObject(ctx context.Context, key string, body io.Reader, contentLength int64, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentLength > 0 {
		input.ContentLength = aws.Int64(contentLength)
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	if _, err := s.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("put object: %w", err)
	}

	return nil
}

// ObjectExists lets the worker distinguish missing objects from transport errors.
func (s *S3Storage) ObjectExists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey") {
		return false, nil
	}

	return false, fmt.Errorf("head object: %w", err)
}

// DeleteObject removes the public object so the old URL stops returning 200.
func (s *S3Storage) DeleteObject(ctx context.Context, key string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}

	return nil
}

// PublicURL derives the direct, fetchable URL returned by completed photos.
func (s *S3Storage) PublicURL(key string) string {
	escapedKey := (&url.URL{Path: key}).EscapedPath()
	return s.publicBaseURL + "/" + strings.TrimPrefix(escapedKey, "/")
}
