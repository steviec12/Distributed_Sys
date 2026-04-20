package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type objectStore interface {
	DownloadToTemp(ctx context.Context, s3Key string) (string, func(), error)
}

type noopObjectStore struct{}

func (noopObjectStore) DownloadToTemp(context.Context, string) (string, func(), error) {
	return "", nil, fmt.Errorf("S3 object storage is required for worker processing")
}

type s3ObjectStore struct {
	client *s3.Client
	bucket string
}

func newObjectStore(ctx context.Context, cfg config) (objectStore, error) {
	if !cfg.s3Enabled {
		return nil, fmt.Errorf("S3 object storage must be enabled for the worker")
	}
	if cfg.s3Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required for the worker")
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.awsRegion),
	}
	if cfg.s3AccessKeyID != "" || cfg.s3SecretAccessKey != "" {
		loadOptions = append(
			loadOptions,
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.s3AccessKeyID, cfg.s3SecretAccessKey, "")),
		)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.s3Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.s3Endpoint)
		}
		options.UsePathStyle = cfg.s3UsePathStyle
	})

	return &s3ObjectStore{
		client: client,
		bucket: cfg.s3Bucket,
	}, nil
}

func (s *s3ObjectStore) DownloadToTemp(ctx context.Context, s3Key string) (string, func(), error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return "", nil, fmt.Errorf("download S3 object %s: %w", s3Key, err)
	}
	defer result.Body.Close()

	tempFile, err := os.CreateTemp("", "workout-video-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(tempFile, result.Body); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return "", nil, fmt.Errorf("copy S3 object to temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}

	cleanup := func() {
		_ = os.Remove(tempFile.Name())
	}
	return tempFile.Name(), cleanup, nil
}
