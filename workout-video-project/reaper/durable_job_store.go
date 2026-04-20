package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type durableJobUpdate struct {
	JobID       string
	Status      string
	RetryCount  int64
	CompletedAt string
	FailureType string
	Error       string
}

type durableJobStore interface {
	MarkRecoveryOutcome(ctx context.Context, update durableJobUpdate) error
}

type noopDurableJobStore struct{}

func (noopDurableJobStore) MarkRecoveryOutcome(context.Context, durableJobUpdate) error { return nil }

type dynamoDurableJobStore struct {
	client    *dynamodb.Client
	tableName string
}

func newDurableJobStore(ctx context.Context, cfg config) (durableJobStore, error) {
	if !cfg.dynamoEnabled {
		return noopDurableJobStore{}, nil
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

func (s *dynamoDurableJobStore) MarkRecoveryOutcome(ctx context.Context, update durableJobUpdate) error {
	setFields := map[string]any{
		"status":       update.Status,
		"retry_count":  update.RetryCount,
		"failure_type": update.FailureType,
		"error":        update.Error,
	}
	removeFields := []string{"worker_id", "processing_started_at", "result_json"}

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
		ConditionExpression: aws.String("attribute_exists(job_id)"),
		UpdateExpression:    aws.String(updateExpression),
		ExpressionAttributeNames: func() map[string]string {
			if len(expressionAttributeNames) == 0 {
				return nil
			}
			return expressionAttributeNames
		}(),
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
