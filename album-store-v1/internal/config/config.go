package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds the runtime settings shared by the API and worker services.
type Config struct {
	Port              string
	DatabaseURL       string
	AWSRegion         string
	S3Bucket          string
	SQSQueueURL       string
	PublicS3BaseURL   string
	WorkerConcurrency int
}

// Load reads environment variables and validates the required service settings.
func Load() (Config, error) {
	cfg := Config{
		Port:              envOrDefault("PORT", "8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		AWSRegion:         os.Getenv("AWS_REGION"),
		S3Bucket:          os.Getenv("S3_BUCKET"),
		SQSQueueURL:       os.Getenv("SQS_QUEUE_URL"),
		PublicS3BaseURL:   os.Getenv("PUBLIC_S3_BASE_URL"),
		WorkerConcurrency: envOrDefaultInt("WORKER_CONCURRENCY", 30),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.AWSRegion == "" {
		return Config{}, fmt.Errorf("AWS_REGION is required")
	}
	if cfg.S3Bucket == "" {
		return Config{}, fmt.Errorf("S3_BUCKET is required")
	}
	if cfg.PublicS3BaseURL == "" {
		cfg.PublicS3BaseURL = fmt.Sprintf("https://%s.s3.%s.amazonaws.com", cfg.S3Bucket, cfg.AWSRegion)
	}
	if cfg.WorkerConcurrency <= 0 {
		return Config{}, fmt.Errorf("WORKER_CONCURRENCY must be greater than zero")
	}

	return cfg, nil
}

// envOrDefault reads one environment variable and falls back when it is unset.
func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envOrDefaultInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
