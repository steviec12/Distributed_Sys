package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type deploymentMode string

const (
	deploymentModeLocal deploymentMode = "local"
	deploymentModeAWS   deploymentMode = "aws"
)

type Config struct {
	DeploymentMode deploymentMode
	Port           string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	Dynamo         DynamoConfig
	ObjectStore    ObjectStoreConfig
}

func loadConfig() Config {
	cfg, err := parseConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	return cfg
}

func parseConfigFromEnv(getenv func(string) string) (Config, error) {
	mode, err := parseDeploymentMode(getenv("DEPLOYMENT_MODE"))
	if err != nil {
		return Config{}, err
	}

	redisDB, err := parseIntEnv(getenv, "REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		DeploymentMode: mode,
		Port:           getEnv(getenv, "PORT", "8080"),
		RedisAddr:      getEnv(getenv, "REDIS_ADDR", "localhost:6379"),
		RedisPassword:  getEnv(getenv, "REDIS_PASSWORD", ""),
		RedisDB:        redisDB,
		Dynamo: DynamoConfig{
			Enabled:         parseBoolEnv(getenv, "DYNAMO_ENABLED", false),
			Region:          getEnv(getenv, "AWS_REGION", "us-west-2"),
			Endpoint:        getEnv(getenv, "DYNAMO_ENDPOINT", ""),
			TableName:       getEnv(getenv, "DYNAMO_TABLE_NAME", "workout_jobs"),
			AutoCreateTable: parseBoolEnv(getenv, "DYNAMO_AUTO_CREATE_TABLE", false),
		},
		ObjectStore: ObjectStoreConfig{
			Enabled:                  parseBoolEnv(getenv, "S3_ENABLED", false),
			Region:                   getEnv(getenv, "AWS_REGION", "us-west-2"),
			Endpoint:                 getEnv(getenv, "S3_ENDPOINT", ""),
			PublicEndpoint:           getEnv(getenv, "S3_PUBLIC_ENDPOINT", getEnv(getenv, "S3_ENDPOINT", "")),
			Bucket:                   getEnv(getenv, "S3_BUCKET", ""),
			AccessKeyID:              getEnv(getenv, "S3_ACCESS_KEY_ID", ""),
			SecretAccessKey:          getEnv(getenv, "S3_SECRET_ACCESS_KEY", ""),
			UsePathStyle:             parseBoolEnv(getenv, "S3_USE_PATH_STYLE", false),
			AutoCreateBucket:         parseBoolEnv(getenv, "S3_AUTO_CREATE_BUCKET", false),
			PresignExpirationSeconds: 3600,
			MultipartPartSizeBytes:   minMultipartPartSize,
		},
	}
	cfg.ObjectStore.PresignExpirationSeconds, err = parseInt64Env(getenv, "S3_PRESIGN_EXPIRATION_SECONDS", 3600)
	if err != nil {
		return Config{}, err
	}
	cfg.ObjectStore.MultipartPartSizeBytes, err = parseInt64Env(getenv, "MULTIPART_PART_SIZE_BYTES", minMultipartPartSize)
	if err != nil {
		return Config{}, err
	}

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Port) == "" {
		return fmt.Errorf("PORT is required")
	}
	if strings.TrimSpace(cfg.RedisAddr) == "" {
		return fmt.Errorf("REDIS_ADDR is required")
	}
	if !cfg.Dynamo.Enabled {
		return fmt.Errorf("DYNAMO_ENABLED=true is required for the API")
	}
	if !cfg.ObjectStore.Enabled {
		return fmt.Errorf("S3_ENABLED=true is required for the API")
	}
	if strings.TrimSpace(cfg.Dynamo.Region) == "" {
		return fmt.Errorf("AWS_REGION is required when DYNAMO_ENABLED=true")
	}
	if strings.TrimSpace(cfg.Dynamo.TableName) == "" {
		return fmt.Errorf("DYNAMO_TABLE_NAME is required when DYNAMO_ENABLED=true")
	}
	if strings.TrimSpace(cfg.ObjectStore.Bucket) == "" {
		return fmt.Errorf("S3_BUCKET is required when S3_ENABLED=true")
	}
	if cfg.ObjectStore.PresignExpirationSeconds <= 0 {
		return fmt.Errorf("S3_PRESIGN_EXPIRATION_SECONDS must be greater than 0")
	}
	if cfg.ObjectStore.MultipartPartSizeBytes < minMultipartPartSize {
		return fmt.Errorf("MULTIPART_PART_SIZE_BYTES must be at least %d", minMultipartPartSize)
	}
	if err := validateCredentialPair("S3", cfg.ObjectStore.AccessKeyID, cfg.ObjectStore.SecretAccessKey); err != nil {
		return err
	}

	switch cfg.DeploymentMode {
	case deploymentModeLocal:
		if strings.TrimSpace(cfg.Dynamo.Endpoint) == "" {
			return fmt.Errorf("DYNAMO_ENDPOINT is required when DEPLOYMENT_MODE=local")
		}
		if strings.TrimSpace(cfg.ObjectStore.Endpoint) == "" {
			return fmt.Errorf("S3_ENDPOINT is required when DEPLOYMENT_MODE=local")
		}
		if cfg.ObjectStore.AccessKeyID == "" || cfg.ObjectStore.SecretAccessKey == "" {
			return fmt.Errorf("S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY are required when DEPLOYMENT_MODE=local")
		}
	case deploymentModeAWS:
		if cfg.Dynamo.Endpoint != "" {
			return fmt.Errorf("DYNAMO_ENDPOINT must be empty when DEPLOYMENT_MODE=aws")
		}
		if cfg.Dynamo.AutoCreateTable {
			return fmt.Errorf("DYNAMO_AUTO_CREATE_TABLE must be false when DEPLOYMENT_MODE=aws")
		}
		if cfg.ObjectStore.Endpoint != "" {
			return fmt.Errorf("S3_ENDPOINT must be empty when DEPLOYMENT_MODE=aws")
		}
		if cfg.ObjectStore.PublicEndpoint != "" {
			return fmt.Errorf("S3_PUBLIC_ENDPOINT must be empty when DEPLOYMENT_MODE=aws")
		}
		if cfg.ObjectStore.UsePathStyle {
			return fmt.Errorf("S3_USE_PATH_STYLE must be false when DEPLOYMENT_MODE=aws")
		}
		if cfg.ObjectStore.AutoCreateBucket {
			return fmt.Errorf("S3_AUTO_CREATE_BUCKET must be false when DEPLOYMENT_MODE=aws")
		}
	default:
		return fmt.Errorf("unsupported DEPLOYMENT_MODE %q", cfg.DeploymentMode)
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

func validateCredentialPair(prefix, accessKeyID, secretAccessKey string) error {
	if (accessKeyID == "") != (secretAccessKey == "") {
		return fmt.Errorf("%s credentials must provide both access key and secret key or neither", prefix)
	}
	return nil
}
