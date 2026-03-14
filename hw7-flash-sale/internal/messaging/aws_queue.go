package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	appconfig "hw7-flash-sale/internal/config"
	"hw7-flash-sale/internal/models"
)

type SNSPublisher struct {
	client   *sns.Client
	topicARN string
	logger   *log.Logger
}

func NewSNSPublisher(ctx context.Context, cfg appconfig.Config, logger *log.Logger) (*SNSPublisher, error) {
	if cfg.SNSTopicARN == "" {
		return nil, fmt.Errorf("SNS_TOPIC_ARN is required when MESSAGING_BACKEND=aws")
	}

	if logger == nil {
		logger = log.Default()
	}

	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &SNSPublisher{
		client:   sns.NewFromConfig(awsCfg),
		topicARN: cfg.SNSTopicARN,
		logger:   logger,
	}, nil
}

func (p *SNSPublisher) PublishOrderCreated(ctx context.Context, event models.OrderCreatedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	output, err := p.client.Publish(ctx, &sns.PublishInput{
		TopicArn: &p.topicARN,
		Message:  stringPtr(string(payload)),
	})
	if err != nil {
		return err
	}

	p.logger.Printf(
		"published sns order event topic=%s order_id=%s message_id=%s",
		p.topicARN,
		event.Order.OrderID,
		safeString(output.MessageId),
	)
	return nil
}

type SQSConsumer struct {
	client          *sqs.Client
	queueURL        string
	waitTimeSeconds int32
	logger          *log.Logger
}

func NewSQSConsumer(ctx context.Context, cfg appconfig.Config, logger *log.Logger) (*SQSConsumer, error) {
	if cfg.SQSQueueURL == "" {
		return nil, fmt.Errorf("SQS_QUEUE_URL is required when MESSAGING_BACKEND=aws")
	}

	if logger == nil {
		logger = log.Default()
	}

	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &SQSConsumer{
		client:          sqs.NewFromConfig(awsCfg),
		queueURL:        cfg.SQSQueueURL,
		waitTimeSeconds: int32(cfg.SQSWaitTimeSeconds),
		logger:          logger,
	}, nil
}

func (c *SQSConsumer) Receive(ctx context.Context) (ReceivedOrderCreatedEvent, error) {
	for {
		output, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &c.queueURL,
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     c.waitTimeSeconds,
		})
		if err != nil {
			return ReceivedOrderCreatedEvent{}, err
		}

		if len(output.Messages) == 0 {
			select {
			case <-ctx.Done():
				return ReceivedOrderCreatedEvent{}, ctx.Err()
			default:
				continue
			}
		}

		message := output.Messages[0]
		event, err := parseSQSEvent(message)
		if err != nil {
			return ReceivedOrderCreatedEvent{}, err
		}

		receiptHandle := safeString(message.ReceiptHandle)
		messageID := safeString(message.MessageId)
		c.logger.Printf("received sqs order event queue=%s order_id=%s message_id=%s", c.queueURL, event.Order.OrderID, messageID)

		return ReceivedOrderCreatedEvent{
			Event: event,
			Ack: func() error {
				deleteCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				_, err := c.client.DeleteMessage(deleteCtx, &sqs.DeleteMessageInput{
					QueueUrl:      &c.queueURL,
					ReceiptHandle: &receiptHandle,
				})
				return err
			},
		}, nil
	}
}

func parseSQSEvent(message sqstypes.Message) (models.OrderCreatedEvent, error) {
	body := safeString(message.Body)

	var directEvent models.OrderCreatedEvent
	if err := json.Unmarshal([]byte(body), &directEvent); err == nil && directEvent.Order.OrderID != "" {
		return directEvent, nil
	}

	var envelope struct {
		Type      string `json:"Type"`
		MessageID string `json:"MessageId"`
		TopicARN  string `json:"TopicArn"`
		Message   string `json:"Message"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return models.OrderCreatedEvent{}, fmt.Errorf("decode sqs message body: %w", err)
	}

	if envelope.Message == "" {
		return models.OrderCreatedEvent{}, fmt.Errorf("sqs message body did not contain an order event")
	}

	var nestedEvent models.OrderCreatedEvent
	if err := json.Unmarshal([]byte(envelope.Message), &nestedEvent); err != nil {
		return models.OrderCreatedEvent{}, fmt.Errorf("decode sns envelope message: %w", err)
	}
	if nestedEvent.Order.OrderID == "" {
		return models.OrderCreatedEvent{}, fmt.Errorf("sns envelope message did not contain a valid order")
	}

	return nestedEvent, nil
}

func loadAWSConfig(ctx context.Context, cfg appconfig.Config) (aws.Config, error) {
	if cfg.AWSRegion != "" {
		return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	}
	return awsconfig.LoadDefaultConfig(ctx)
}

func stringPtr(value string) *string {
	return &value
}

func safeString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
