package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamoConfig struct {
	Enabled         bool
	Region          string
	Endpoint        string
	TableName       string
	AutoCreateTable bool
}

type DurableJobStore interface {
	PutJob(ctx context.Context, job JobRecord) error
	GetJob(ctx context.Context, jobID string) (JobRecord, error)
	DeleteJob(ctx context.Context, jobID string) error
	Ping(ctx context.Context) error
}

type DynamoJobStore struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamoJobStore(ctx context.Context, cfg DynamoConfig) (*DynamoJobStore, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.Endpoint != "" {
		loadOptions = append(
			loadOptions,
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("local", "local", "")),
		)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := dynamodb.NewFromConfig(awsCfg, func(options *dynamodb.Options) {
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})

	store := &DynamoJobStore{
		client:    client,
		tableName: cfg.TableName,
	}
	if cfg.AutoCreateTable {
		if err := store.EnsureTable(ctx); err != nil {
			return nil, err
		}
	}

	return store, nil
}

func (s *DynamoJobStore) EnsureTable(ctx context.Context) error {
	_, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.tableName),
	})
	if err == nil {
		return nil
	}

	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return fmt.Errorf("describe dynamodb table %s: %w", s.tableName, err)
	}

	_, err = s.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(s.tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("job_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("job_id"),
				KeyType:       types.KeyTypeHash,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			return fmt.Errorf("create dynamodb table %s: %w", s.tableName, err)
		}
	}

	waiter := dynamodb.NewTableExistsWaiter(s.client)
	if err := waiter.Wait(
		ctx,
		&dynamodb.DescribeTableInput{TableName: aws.String(s.tableName)},
		30*time.Second,
	); err != nil {
		return fmt.Errorf("wait for dynamodb table %s: %w", s.tableName, err)
	}

	return nil
}

func (s *DynamoJobStore) PutJob(ctx context.Context, job JobRecord) error {
	item, err := attributevalue.MarshalMap(marshalDurableJob(job))
	if err != nil {
		return fmt.Errorf("marshal durable job: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("put durable job: %w", err)
	}

	return nil
}

func (s *DynamoJobStore) GetJob(ctx context.Context, jobID string) (JobRecord, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"job_id": &types.AttributeValueMemberS{Value: jobID},
		},
	})
	if err != nil {
		return JobRecord{}, fmt.Errorf("get durable job: %w", err)
	}
	if len(result.Item) == 0 {
		return JobRecord{}, ErrJobNotFound
	}

	var item map[string]any
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return JobRecord{}, fmt.Errorf("unmarshal durable job: %w", err)
	}

	return decodeDurableJob(item)
}

func (s *DynamoJobStore) DeleteJob(ctx context.Context, jobID string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"job_id": &types.AttributeValueMemberS{Value: jobID},
		},
	})
	if err != nil {
		return fmt.Errorf("delete durable job: %w", err)
	}

	return nil
}

func (s *DynamoJobStore) Ping(ctx context.Context) error {
	_, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.tableName),
	})
	if err != nil {
		return fmt.Errorf("describe dynamodb table %s: %w", s.tableName, err)
	}

	return nil
}

func marshalDurableJob(job JobRecord) map[string]any {
	item := map[string]any{
		"job_id":          job.JobID,
		"status":          job.Status,
		"file_name":       job.FileName,
		"file_size_bytes": job.FileSizeBytes,
		"retry_count":     job.RetryCount,
	}

	putOptionalString(item, "content_type", job.ContentType)
	putOptionalString(item, "s3_key", job.S3Key)
	putOptionalString(item, "upload_id", job.UploadID)
	putOptionalString(item, "created_at", job.CreatedAt)
	putOptionalString(item, "started_at", job.StartedAt)
	putOptionalString(item, "completed_at", job.CompletedAt)
	putOptionalString(item, "processing_started_at", job.ProcessingStartedAt)
	putOptionalString(item, "worker_id", job.WorkerID)
	putOptionalString(item, "failure_type", job.FailureType)
	putOptionalString(item, "error", job.Error)
	putOptionalString(item, "result_json", job.ResultJSON)
	return item
}

func decodeDurableJob(item map[string]any) (JobRecord, error) {
	fileSize, err := parseAnyInt64(item["file_size_bytes"])
	if err != nil {
		return JobRecord{}, fmt.Errorf("parse durable file_size_bytes: %w", err)
	}

	retryCount, err := parseAnyInt64(item["retry_count"])
	if err != nil {
		return JobRecord{}, fmt.Errorf("parse durable retry_count: %w", err)
	}

	return JobRecord{
		JobID:               stringValue(item["job_id"]),
		Status:              stringValue(item["status"]),
		FileName:            stringValue(item["file_name"]),
		FileSizeBytes:       fileSize,
		ContentType:         stringValue(item["content_type"]),
		S3Key:               stringValue(item["s3_key"]),
		UploadID:            stringValue(item["upload_id"]),
		CreatedAt:           stringValue(item["created_at"]),
		StartedAt:           stringValue(item["started_at"]),
		CompletedAt:         stringValue(item["completed_at"]),
		ProcessingStartedAt: stringValue(item["processing_started_at"]),
		WorkerID:            stringValue(item["worker_id"]),
		RetryCount:          retryCount,
		FailureType:         stringValue(item["failure_type"]),
		Error:               stringValue(item["error"]),
		ResultJSON:          stringValue(item["result_json"]),
	}, nil
}

func putOptionalString(item map[string]any, key, value string) {
	if value != "" {
		item[key] = value
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func parseAnyInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case nil:
		return 0, nil
	case int64:
		return typed, nil
	case int32:
		return int64(typed), nil
	case int:
		return int64(typed), nil
	case float64:
		return int64(typed), nil
	case string:
		if typed == "" {
			return 0, nil
		}
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported int type %T", value)
	}
}
