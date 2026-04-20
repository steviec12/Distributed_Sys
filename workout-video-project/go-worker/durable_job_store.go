package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var errDurableJobNotFound = errors.New("durable job not found")

type durableJobRecord struct {
	JobID         string
	Status        string
	FileName      string
	FileSizeBytes int64
	ContentType   string
	S3Key         string
	RetryCount    int64
}

type durableJobUpdate struct {
	JobID               string
	StartedAt           string
	ProcessingStartedAt string
	CompletedAt         string
	WorkerID            string
	FailureType         string
	Error               string
	ResultJSON          string
}

type durableJobStore interface {
	GetJob(ctx context.Context, jobID string) (durableJobRecord, error)
	MarkInProgress(ctx context.Context, update durableJobUpdate) error
	MarkCompleted(ctx context.Context, update durableJobUpdate) error
	MarkFailed(ctx context.Context, update durableJobUpdate) error
}

type noopDurableJobStore struct{}

func (noopDurableJobStore) GetJob(context.Context, string) (durableJobRecord, error) {
	return durableJobRecord{}, fmt.Errorf("durable job store is required")
}
func (noopDurableJobStore) MarkInProgress(context.Context, durableJobUpdate) error { return nil }
func (noopDurableJobStore) MarkCompleted(context.Context, durableJobUpdate) error  { return nil }
func (noopDurableJobStore) MarkFailed(context.Context, durableJobUpdate) error     { return nil }

type dynamoDurableJobStore struct {
	client    *dynamodb.Client
	tableName string
}

func newDurableJobStore(ctx context.Context, cfg config) (durableJobStore, error) {
	if !cfg.dynamoEnabled {
		return nil, fmt.Errorf("DynamoDB durable job store must be enabled for the worker")
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.awsRegion),
	}
	if cfg.dynamoEndpoint != "" {
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
		if cfg.dynamoEndpoint != "" {
			options.BaseEndpoint = aws.String(cfg.dynamoEndpoint)
		}
	})

	return &dynamoDurableJobStore{
		client:    client,
		tableName: cfg.dynamoTableName,
	}, nil
}

func (s *dynamoDurableJobStore) GetJob(ctx context.Context, jobID string) (durableJobRecord, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"job_id": &types.AttributeValueMemberS{Value: jobID},
		},
	})
	if err != nil {
		return durableJobRecord{}, fmt.Errorf("get durable job: %w", err)
	}
	if len(result.Item) == 0 {
		return durableJobRecord{}, errDurableJobNotFound
	}

	var item map[string]any
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return durableJobRecord{}, fmt.Errorf("unmarshal durable job: %w", err)
	}

	fileSizeBytes, err := parseAnyInt64(item["file_size_bytes"])
	if err != nil {
		return durableJobRecord{}, fmt.Errorf("parse durable file_size_bytes: %w", err)
	}
	retryCount, err := parseAnyInt64(item["retry_count"])
	if err != nil {
		return durableJobRecord{}, fmt.Errorf("parse durable retry_count: %w", err)
	}

	return durableJobRecord{
		JobID:         stringValue(item["job_id"]),
		Status:        stringValue(item["status"]),
		FileName:      stringValue(item["file_name"]),
		FileSizeBytes: fileSizeBytes,
		ContentType:   stringValue(item["content_type"]),
		S3Key:         stringValue(item["s3_key"]),
		RetryCount:    retryCount,
	}, nil
}

func (s *dynamoDurableJobStore) MarkInProgress(ctx context.Context, update durableJobUpdate) error {
	names := map[string]string{
		"#status":                "status",
		"#started_at":            "started_at",
		"#processing_started_at": "processing_started_at",
		"#worker_id":             "worker_id",
		"#failure_type":          "failure_type",
		"#completed_at":          "completed_at",
		"#error":                 "error",
		"#result_json":           "result_json",
	}
	values := map[string]types.AttributeValue{
		":status":                &types.AttributeValueMemberS{Value: statusInProgress},
		":started_at":            &types.AttributeValueMemberS{Value: update.StartedAt},
		":processing_started_at": &types.AttributeValueMemberS{Value: update.ProcessingStartedAt},
		":worker_id":             &types.AttributeValueMemberS{Value: update.WorkerID},
		":failure_type":          &types.AttributeValueMemberS{Value: update.FailureType},
	}
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"job_id": &types.AttributeValueMemberS{Value: update.JobID},
		},
		ConditionExpression: aws.String("attribute_exists(job_id)"),
		UpdateExpression: aws.String(
			"SET #status = :status, #started_at = if_not_exists(#started_at, :started_at), " +
				"#processing_started_at = :processing_started_at, #worker_id = :worker_id, #failure_type = :failure_type " +
				"REMOVE #completed_at, #error, #result_json",
		),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	})
	if err != nil {
		return fmt.Errorf("mark durable job in progress: %w", err)
	}
	return nil
}

func (s *dynamoDurableJobStore) MarkCompleted(ctx context.Context, update durableJobUpdate) error {
	return s.updateJob(ctx, update.JobID, map[string]any{
		"status":       statusCompleted,
		"completed_at": update.CompletedAt,
		"failure_type": update.FailureType,
		"result_json":  update.ResultJSON,
	}, []string{"error"})
}

func (s *dynamoDurableJobStore) MarkFailed(ctx context.Context, update durableJobUpdate) error {
	setFields := map[string]any{
		"status":       statusFailed,
		"failure_type": update.FailureType,
		"error":        update.Error,
	}
	removeFields := []string{"result_json"}

	if update.CompletedAt != "" {
		setFields["completed_at"] = update.CompletedAt
	} else {
		removeFields = append(removeFields, "completed_at")
	}

	return s.updateJob(ctx, update.JobID, setFields, removeFields)
}

func (s *dynamoDurableJobStore) updateJob(ctx context.Context, jobID string, setFields map[string]any, removeFields []string) error {
	updateParts := make([]string, 0, len(setFields)+1)
	expressionAttributeNames := map[string]string{}
	expressionAttributeValues := map[string]types.AttributeValue{}

	index := 0
	for field, value := range setFields {
		nameKey := fmt.Sprintf("#n%d", index)
		valueKey := fmt.Sprintf(":v%d", index)
		expressionAttributeNames[nameKey] = field
		marshaledValue, err := attributevalue.Marshal(value)
		if err != nil {
			return fmt.Errorf("marshal durable field %s: %w", field, err)
		}
		expressionAttributeValues[valueKey] = marshaledValue
		updateParts = append(updateParts, fmt.Sprintf("%s = %s", nameKey, valueKey))
		index++
	}

	updateExpression := ""
	if len(updateParts) > 0 {
		updateExpression = "SET " + strings.Join(updateParts, ", ")
	}
	if len(removeFields) > 0 {
		removeParts := make([]string, 0, len(removeFields))
		for _, field := range removeFields {
			nameKey := fmt.Sprintf("#r%d", index)
			expressionAttributeNames[nameKey] = field
			removeParts = append(removeParts, nameKey)
			index++
		}
		if updateExpression != "" {
			updateExpression += " "
		}
		updateExpression += "REMOVE " + strings.Join(removeParts, ", ")
	}

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"job_id": &types.AttributeValueMemberS{Value: jobID},
		},
		ConditionExpression:      aws.String("attribute_exists(job_id)"),
		UpdateExpression:         aws.String(updateExpression),
		ExpressionAttributeNames: expressionAttributeNames,
		ExpressionAttributeValues: func() map[string]types.AttributeValue {
			if len(expressionAttributeValues) == 0 {
				return nil
			}
			return expressionAttributeValues
		}(),
	})
	if err != nil {
		return fmt.Errorf("update durable job %s: %w", jobID, err)
	}
	return nil
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
		return 0, fmt.Errorf("unsupported numeric type %T", value)
	}
}
