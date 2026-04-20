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
	maxRetries, err := parseInt64Env(getenv, "MAX_RETRIES", 3)
	if err != nil {
		return config{}, err
	}
	reaperIntervalSeconds, err := parseIntEnv(getenv, "REAPER_INTERVAL_SECONDS", 3)
	if err != nil {
		return config{}, err
	}
	staleTimeoutSeconds, err := parseIntEnv(getenv, "STALE_TIMEOUT_SECONDS", 12)
	if err != nil {
		return config{}, err
	}

	reaperID := getEnv(getenv, "REAPER_ID", "")
	if reaperID == "" {
		generated, err := newToken("reaper-", 4)
		if err != nil {
			return config{}, fmt.Errorf("create reaper id: %w", err)
		}
		reaperID = generated
	}

	cfg := config{
		redisAddr:          getEnv(getenv, "REDIS_ADDR", "localhost:6379"),
		redisPassword:      getEnv(getenv, "REDIS_PASSWORD", ""),
		redisDB:            redisDB,
		pendingQueueKey:    getEnv(getenv, "QUEUE_KEY", "queue:pending"),
		processingQueueKey: getEnv(getenv, "PROCESSING_QUEUE_KEY", "queue:processing"),
		retryableFailedSet: getEnv(getenv, "RETRYABLE_FAILED_SET_KEY", "set:retryable_failed"),
		reaperID:           reaperID,
		maxRetries:         maxRetries,
		reaperInterval:     time.Duration(reaperIntervalSeconds) * time.Second,
		staleTimeout:       time.Duration(staleTimeoutSeconds) * time.Second,
		dynamoEnabled:      parseBoolEnv(getenv, "DYNAMO_ENABLED", false),
		awsRegion:          getEnv(getenv, "AWS_REGION", "us-west-2"),
		dynamoEndpoint:     getEnv(getenv, "DYNAMO_ENDPOINT", ""),
		dynamoTableName:    getEnv(getenv, "DYNAMO_TABLE_NAME", "workout_jobs"),
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
		return fmt.Errorf("DYNAMO_ENABLED=true is required for the reaper")
	}
	if strings.TrimSpace(cfg.awsRegion) == "" {
		return fmt.Errorf("AWS_REGION is required")
	}
	if strings.TrimSpace(cfg.dynamoTableName) == "" {
		return fmt.Errorf("DYNAMO_TABLE_NAME is required when DYNAMO_ENABLED=true")
	}
	if cfg.reaperInterval <= 0 {
		return fmt.Errorf("REAPER_INTERVAL_SECONDS must be greater than 0")
	}
	if cfg.staleTimeout <= 0 {
		return fmt.Errorf("STALE_TIMEOUT_SECONDS must be greater than 0")
	}
	if cfg.maxRetries < 0 {
		return fmt.Errorf("MAX_RETRIES must be greater than or equal to 0")
	}

	switch mode {
	case deploymentModeLocal:
		if strings.TrimSpace(cfg.dynamoEndpoint) == "" {
			return fmt.Errorf("DYNAMO_ENDPOINT is required when DEPLOYMENT_MODE=local")
		}
	case deploymentModeAWS:
		if cfg.dynamoEndpoint != "" {
			return fmt.Errorf("DYNAMO_ENDPOINT must be empty when DEPLOYMENT_MODE=aws")
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
