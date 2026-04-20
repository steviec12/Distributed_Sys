package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type deploymentMode string

const (
	deploymentModeLocal deploymentMode = "local"
	deploymentModeAWS   deploymentMode = "aws"
)

func loadConfig() config {
	cfg, err := parseConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	return cfg
}

func parseConfigFromEnv(getenv func(string) string) (config, error) {
	mode, err := parseDeploymentMode(getenv("DEPLOYMENT_MODE"))
	if err != nil {
		return config{}, err
	}

	redisDB, err := parseIntEnv(getenv, "REDIS_DB", 0)
	if err != nil {
		return config{}, err
	}
	heartbeatSeconds, err := parseIntEnv(getenv, "HEARTBEAT_INTERVAL_SECONDS", 2)
	if err != nil {
		return config{}, err
	}
	maxRetries, err := parseInt64Env(getenv, "MAX_RETRIES", 3)
	if err != nil {
		return config{}, err
	}

	workerID := getEnv(getenv, "WORKER_ID", "")
	if workerID == "" {
		generated, err := newToken("worker-", 4)
		if err != nil {
			return config{}, fmt.Errorf("create worker id: %w", err)
		}
		workerID = generated
	}

	cfg := config{
		redisAddr:          getEnv(getenv, "REDIS_ADDR", "localhost:6379"),
		redisPassword:      getEnv(getenv, "REDIS_PASSWORD", ""),
		redisDB:            redisDB,
		pendingQueueKey:    getEnv(getenv, "QUEUE_KEY", "queue:pending"),
		processingQueueKey: getEnv(getenv, "PROCESSING_QUEUE_KEY", "queue:processing"),
		retryableFailedSet: getEnv(getenv, "RETRYABLE_FAILED_SET_KEY", "set:retryable_failed"),
		workerID:           workerID,
		heartbeatInterval:  time.Duration(heartbeatSeconds) * time.Second,
		maxRetries:         maxRetries,
		dynamoEnabled:      parseBoolEnv(getenv, "DYNAMO_ENABLED", false),
		awsRegion:          getEnv(getenv, "AWS_REGION", "us-west-2"),
		dynamoEndpoint:     getEnv(getenv, "DYNAMO_ENDPOINT", ""),
		dynamoTableName:    getEnv(getenv, "DYNAMO_TABLE_NAME", "workout_jobs"),
		s3Enabled:          parseBoolEnv(getenv, "S3_ENABLED", false),
		s3Endpoint:         getEnv(getenv, "S3_ENDPOINT", ""),
		s3Bucket:           getEnv(getenv, "S3_BUCKET", ""),
		s3AccessKeyID:      getEnv(getenv, "S3_ACCESS_KEY_ID", ""),
		s3SecretAccessKey:  getEnv(getenv, "S3_SECRET_ACCESS_KEY", ""),
		s3UsePathStyle:     parseBoolEnv(getenv, "S3_USE_PATH_STYLE", false),
	}

	if err := validateConfig(mode, cfg); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func validateConfig(mode deploymentMode, cfg config) error {
	if strings.TrimSpace(cfg.redisAddr) == "" {
		return fmt.Errorf("REDIS_ADDR is required")
	}
	if !cfg.dynamoEnabled {
		return fmt.Errorf("DYNAMO_ENABLED=true is required for the worker")
	}
	if !cfg.s3Enabled {
		return fmt.Errorf("S3_ENABLED=true is required for the worker")
	}
	if strings.TrimSpace(cfg.awsRegion) == "" {
		return fmt.Errorf("AWS_REGION is required")
	}
	if strings.TrimSpace(cfg.dynamoTableName) == "" {
		return fmt.Errorf("DYNAMO_TABLE_NAME is required when DYNAMO_ENABLED=true")
	}
	if strings.TrimSpace(cfg.s3Bucket) == "" {
		return fmt.Errorf("S3_BUCKET is required when S3_ENABLED=true")
	}
	if cfg.heartbeatInterval <= 0 {
		return fmt.Errorf("HEARTBEAT_INTERVAL_SECONDS must be greater than 0")
	}
	if cfg.maxRetries < 0 {
		return fmt.Errorf("MAX_RETRIES must be greater than or equal to 0")
	}
	if err := validateCredentialPair(cfg.s3AccessKeyID, cfg.s3SecretAccessKey); err != nil {
		return err
	}

	switch mode {
	case deploymentModeLocal:
		if strings.TrimSpace(cfg.dynamoEndpoint) == "" {
			return fmt.Errorf("DYNAMO_ENDPOINT is required when DEPLOYMENT_MODE=local")
		}
		if strings.TrimSpace(cfg.s3Endpoint) == "" {
			return fmt.Errorf("S3_ENDPOINT is required when DEPLOYMENT_MODE=local")
		}
		if cfg.s3AccessKeyID == "" || cfg.s3SecretAccessKey == "" {
			return fmt.Errorf("S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY are required when DEPLOYMENT_MODE=local")
		}
	case deploymentModeAWS:
		if cfg.dynamoEndpoint != "" {
			return fmt.Errorf("DYNAMO_ENDPOINT must be empty when DEPLOYMENT_MODE=aws")
		}
		if cfg.s3Endpoint != "" {
			return fmt.Errorf("S3_ENDPOINT must be empty when DEPLOYMENT_MODE=aws")
		}
		if cfg.s3UsePathStyle {
			return fmt.Errorf("S3_USE_PATH_STYLE must be false when DEPLOYMENT_MODE=aws")
		}
	default:
		return fmt.Errorf("unsupported DEPLOYMENT_MODE %q", mode)
	}
	return nil
}

func parseDeploymentMode(value string) (deploymentMode, error) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return deploymentModeLocal, nil
	}
	switch deploymentMode(normalized) {
	case deploymentModeLocal, deploymentModeAWS:
		return deploymentMode(normalized), nil
	default:
		return "", fmt.Errorf("DEPLOYMENT_MODE must be one of: local, aws")
	}
}

func getEnv(getenv func(string) string, key, fallback string) string {
	value := getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func parseBoolEnv(getenv func(string) string, key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "true"
}

func parseIntEnv(getenv func(string) string, key string, fallback int) (int, error) {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func parseInt64Env(getenv func(string) string, key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func validateCredentialPair(accessKeyID, secretAccessKey string) error {
	if (accessKeyID == "") != (secretAccessKey == "") {
		return fmt.Errorf("S3 credentials must provide both access key and secret key or neither")
	}
	return nil
}
