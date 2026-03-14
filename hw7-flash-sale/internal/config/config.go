package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port                  string
	SyncPaymentSlots      int
	PaymentDelay          time.Duration
	MessagingBackend      string
	EventsDir             string
	AWSRegion             string
	SNSTopicARN           string
	SQSQueueURL           string
	SQSWaitTimeSeconds    int
	ProcessorWorkerCount  int
	ProcessorPollInterval time.Duration
}

func Load() (Config, error) {
	port := getEnv("PORT", "8080")

	slots, err := getEnvInt("SYNC_PAYMENT_SLOTS", 1)
	if err != nil {
		return Config{}, fmt.Errorf("parse SYNC_PAYMENT_SLOTS: %w", err)
	}
	if slots < 1 {
		return Config{}, fmt.Errorf("SYNC_PAYMENT_SLOTS must be at least 1")
	}

	delaySeconds, err := getEnvInt("PAYMENT_DELAY_SECONDS", 3)
	if err != nil {
		return Config{}, fmt.Errorf("parse PAYMENT_DELAY_SECONDS: %w", err)
	}
	if delaySeconds < 1 {
		return Config{}, fmt.Errorf("PAYMENT_DELAY_SECONDS must be at least 1")
	}

	backend := getEnv("MESSAGING_BACKEND", "file")
	if backend != "file" && backend != "aws" {
		return Config{}, fmt.Errorf("MESSAGING_BACKEND must be 'file' or 'aws'")
	}

	eventsDir := getEnv("EVENTS_DIR", "./runtime/events")
	awsRegion := getEnv("AWS_REGION", "")
	snsTopicARN := getEnv("SNS_TOPIC_ARN", "")
	sqsQueueURL := getEnv("SQS_QUEUE_URL", "")

	sqsWaitTimeSeconds, err := getEnvInt("SQS_WAIT_TIME_SECONDS", 20)
	if err != nil {
		return Config{}, fmt.Errorf("parse SQS_WAIT_TIME_SECONDS: %w", err)
	}
	if sqsWaitTimeSeconds < 1 || sqsWaitTimeSeconds > 20 {
		return Config{}, fmt.Errorf("SQS_WAIT_TIME_SECONDS must be between 1 and 20")
	}

	workerCount, err := getEnvInt("PROCESSOR_WORKER_COUNT", 1)
	if err != nil {
		return Config{}, fmt.Errorf("parse PROCESSOR_WORKER_COUNT: %w", err)
	}
	if workerCount < 1 {
		return Config{}, fmt.Errorf("PROCESSOR_WORKER_COUNT must be at least 1")
	}

	pollMilliseconds, err := getEnvInt("PROCESSOR_POLL_MILLISECONDS", 500)
	if err != nil {
		return Config{}, fmt.Errorf("parse PROCESSOR_POLL_MILLISECONDS: %w", err)
	}
	if pollMilliseconds < 1 {
		return Config{}, fmt.Errorf("PROCESSOR_POLL_MILLISECONDS must be at least 1")
	}

	return Config{
		Port:                  port,
		SyncPaymentSlots:      slots,
		PaymentDelay:          time.Duration(delaySeconds) * time.Second,
		MessagingBackend:      backend,
		EventsDir:             eventsDir,
		AWSRegion:             awsRegion,
		SNSTopicARN:           snsTopicARN,
		SQSQueueURL:           sqsQueueURL,
		SQSWaitTimeSeconds:    sqsWaitTimeSeconds,
		ProcessorWorkerCount:  workerCount,
		ProcessorPollInterval: time.Duration(pollMilliseconds) * time.Millisecond,
	}, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}
