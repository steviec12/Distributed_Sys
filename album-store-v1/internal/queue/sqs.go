package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"album-store-v1/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// PhotoJobPublisher is the send-side queue boundary used by the outbox publisher.
type PhotoJobPublisher interface {
	PublishPhotoJob(ctx context.Context, message models.PhotoJobMessage) error
}

// PhotoJobConsumer is the receive-side queue boundary used by the worker loop.
type PhotoJobConsumer interface {
	ReceivePhotoJobs(ctx context.Context, maxMessages int32, waitTimeSeconds int32) ([]ReceivedPhotoJob, error)
	DeleteMessage(ctx context.Context, receiptHandle string) error
}

// ReceivedPhotoJob carries the decoded queue payload plus the SQS receipt handle.
type ReceivedPhotoJob struct {
	Message       models.PhotoJobMessage
	ReceiptHandle string
}

// sqsAPI keeps the AWS client mockable in tests.
type sqsAPI interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

// SQSPublisher is the concrete SQS client used by both publishing and consuming code.
type SQSPublisher struct {
	client   sqsAPI
	queueURL string
}

// NewSQSPublisher builds an AWS-backed SQS client from runtime config.
func NewSQSPublisher(ctx context.Context, region, queueURL string) (*SQSPublisher, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return NewSQSPublisherWithClient(sqs.NewFromConfig(cfg), queueURL), nil
}

// NewSQSPublisherWithClient is the test-friendly constructor.
func NewSQSPublisherWithClient(client sqsAPI, queueURL string) *SQSPublisher {
	return &SQSPublisher{
		client:   client,
		queueURL: queueURL,
	}
}

// PublishPhotoJob sends the minimal photo id payload to SQS.
func (p *SQSPublisher) PublishPhotoJob(ctx context.Context, message models.PhotoJobMessage) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal photo job message: %w", err)
	}

	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		MessageBody: aws.String(string(body)),
		QueueUrl:    aws.String(p.queueURL),
	})
	if err != nil {
		return fmt.Errorf("publish photo job: %w", err)
	}

	return nil
}

// ReceivePhotoJobs long-polls SQS and decodes messages into typed payloads.
func (p *SQSPublisher) ReceivePhotoJobs(ctx context.Context, maxMessages int32, waitTimeSeconds int32) ([]ReceivedPhotoJob, error) {
	output, err := p.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(p.queueURL),
		MaxNumberOfMessages: maxMessages,
		WaitTimeSeconds:     waitTimeSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("receive photo jobs: %w", err)
	}

	jobs := make([]ReceivedPhotoJob, 0, len(output.Messages))
	for _, message := range output.Messages {
		// Decode queue messages here so worker logic only sees typed job payloads.
		var decoded models.PhotoJobMessage
		if err := json.Unmarshal([]byte(aws.ToString(message.Body)), &decoded); err != nil {
			return nil, fmt.Errorf("decode photo job message: %w", err)
		}

		jobs = append(jobs, ReceivedPhotoJob{
			Message:       decoded,
			ReceiptHandle: aws.ToString(message.ReceiptHandle),
		})
	}

	return jobs, nil
}

// DeleteMessage acknowledges successful handling of one SQS delivery.
func (p *SQSPublisher) DeleteMessage(ctx context.Context, receiptHandle string) error {
	_, err := p.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(p.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("delete photo job message: %w", err)
	}

	return nil
}
