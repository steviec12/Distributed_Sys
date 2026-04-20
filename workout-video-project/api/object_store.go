package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const minMultipartPartSize int64 = 5 * 1024 * 1024

type ObjectStoreConfig struct {
	Enabled                  bool
	Region                   string
	Endpoint                 string
	PublicEndpoint           string
	Bucket                   string
	AccessKeyID              string
	SecretAccessKey          string
	UsePathStyle             bool
	AutoCreateBucket         bool
	PresignExpirationSeconds int64
	MultipartPartSizeBytes   int64
}

type MultipartUploadSession struct {
	S3Key    string
	UploadID string
	PartSize int64
	Parts    []UploadPart
}

type CompletedUploadPart struct {
	PartNumber int32
	ETag       string
}

type ObjectStore interface {
	CreateMultipartUploadSession(ctx context.Context, jobID, fileName string, fileSizeBytes int64, contentType string) (MultipartUploadSession, error)
	CompleteMultipartUpload(ctx context.Context, s3Key, uploadID string, parts []CompletedUploadPart) error
	AbortMultipartUpload(ctx context.Context, s3Key, uploadID string) error
}

type S3ObjectStore struct {
	client            *s3.Client
	presignClient     *s3.PresignClient
	bucket            string
	presignExpires    time.Duration
	multipartPartSize int64
}

func NewObjectStore(ctx context.Context, cfg ObjectStoreConfig) (ObjectStore, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("object storage is disabled")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}
	if cfg.MultipartPartSizeBytes < minMultipartPartSize {
		cfg.MultipartPartSizeBytes = minMultipartPartSize
	}
	if cfg.PresignExpirationSeconds <= 0 {
		cfg.PresignExpirationSeconds = 3600
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		loadOptions = append(
			loadOptions,
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
		)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		options.UsePathStyle = cfg.UsePathStyle
	})
	presignSourceClient := client
	if cfg.PublicEndpoint != "" && cfg.PublicEndpoint != cfg.Endpoint {
		presignSourceClient = s3.NewFromConfig(awsCfg, func(options *s3.Options) {
			options.BaseEndpoint = aws.String(cfg.PublicEndpoint)
			options.UsePathStyle = cfg.UsePathStyle
		})
	}

	store := &S3ObjectStore{
		client:            client,
		presignClient:     s3.NewPresignClient(presignSourceClient),
		bucket:            cfg.Bucket,
		presignExpires:    time.Duration(cfg.PresignExpirationSeconds) * time.Second,
		multipartPartSize: cfg.MultipartPartSizeBytes,
	}

	if cfg.AutoCreateBucket {
		if err := store.EnsureBucket(ctx); err != nil {
			return nil, err
		}
	}

	return store, nil
}

func (s *S3ObjectStore) EnsureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err == nil {
		return nil
	}

	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil && !isBucketAlreadyOwnedError(err) {
		return fmt.Errorf("create S3 bucket %s: %w", s.bucket, err)
	}
	return nil
}

func (s *S3ObjectStore) CreateMultipartUploadSession(ctx context.Context, jobID, fileName string, fileSizeBytes int64, contentType string) (MultipartUploadSession, error) {
	if fileSizeBytes <= 0 {
		return MultipartUploadSession{}, fmt.Errorf("file_size_bytes must be greater than 0")
	}
	partCount := int((fileSizeBytes + s.multipartPartSize - 1) / s.multipartPartSize)
	if partCount <= 0 {
		partCount = 1
	}
	if partCount > 10000 {
		return MultipartUploadSession{}, fmt.Errorf("file requires %d parts, which exceeds the S3 limit", partCount)
	}

	s3Key := buildObjectKey(jobID, fileName)

	createResult, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s3Key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return MultipartUploadSession{}, fmt.Errorf("create multipart upload: %w", err)
	}

	parts := make([]UploadPart, 0, partCount)
	for partNumber := 1; partNumber <= partCount; partNumber++ {
		presigned, err := s.presignClient.PresignUploadPart(
			ctx,
			&s3.UploadPartInput{
				Bucket:     aws.String(s.bucket),
				Key:        aws.String(s3Key),
				UploadId:   createResult.UploadId,
				PartNumber: aws.Int32(int32(partNumber)),
			},
			func(options *s3.PresignOptions) {
				options.Expires = s.presignExpires
			},
		)
		if err != nil {
			if abortErr := s.AbortMultipartUpload(ctx, s3Key, aws.ToString(createResult.UploadId)); abortErr != nil {
				return MultipartUploadSession{}, fmt.Errorf("presign upload part %d: %w (abort failed: %v)", partNumber, err, abortErr)
			}
			return MultipartUploadSession{}, fmt.Errorf("presign upload part %d: %w", partNumber, err)
		}

		parts = append(parts, UploadPart{
			PartNumber: int32(partNumber),
			UploadURL:  presigned.URL,
		})
	}

	return MultipartUploadSession{
		S3Key:    s3Key,
		UploadID: aws.ToString(createResult.UploadId),
		PartSize: s.multipartPartSize,
		Parts:    parts,
	}, nil
}

func (s *S3ObjectStore) CompleteMultipartUpload(ctx context.Context, s3Key, uploadID string, parts []CompletedUploadPart) error {
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	completedParts := make([]s3types.CompletedPart, 0, len(parts))
	for _, part := range parts {
		completedParts = append(completedParts, s3types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(part.PartNumber),
		})
	}

	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(s3Key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("complete multipart upload: %w", err)
	}
	return nil
}

func (s *S3ObjectStore) AbortMultipartUpload(ctx context.Context, s3Key, uploadID string) error {
	_, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(s3Key),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		return fmt.Errorf("abort multipart upload: %w", err)
	}
	return nil
}

func buildObjectKey(jobID, fileName string) string {
	safeName := sanitizeFileName(fileName)
	return fmt.Sprintf("uploads/%s/%s", jobID, safeName)
}

func isBucketAlreadyOwnedError(err error) bool {
	if err == nil {
		return false
	}
	parsed := err.Error()
	return strings.Contains(parsed, "BucketAlreadyOwnedByYou") || strings.Contains(parsed, "BucketAlreadyExists")
}
