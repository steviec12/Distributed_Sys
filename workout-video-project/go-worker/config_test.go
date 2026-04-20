package main

import "testing"

func TestParseConfigFromEnvLocal(t *testing.T) {
	cfg, err := parseConfigFromEnv(mapEnv(map[string]string{
		"DEPLOYMENT_MODE":             "local",
		"REDIS_ADDR":                  "redis:6379",
		"DYNAMO_ENABLED":              "true",
		"AWS_REGION":                  "us-west-2",
		"DYNAMO_ENDPOINT":             "http://dynamodb-local:8000",
		"DYNAMO_TABLE_NAME":           "workout_jobs",
		"S3_ENABLED":                  "true",
		"S3_ENDPOINT":                 "http://minio:9000",
		"S3_BUCKET":                   "workout-videos",
		"S3_ACCESS_KEY_ID":            "minioadmin",
		"S3_SECRET_ACCESS_KEY":        "minioadmin",
		"S3_USE_PATH_STYLE":           "true",
		"HEARTBEAT_INTERVAL_SECONDS":  "2",
		"MAX_RETRIES":                 "3",
	}))
	if err != nil {
		t.Fatalf("parseConfigFromEnv returned error: %v", err)
	}
	if cfg.redisAddr != "redis:6379" {
		t.Fatalf("expected redis address to be preserved")
	}
}

func TestParseConfigFromEnvAWSRejectsLocalEndpoint(t *testing.T) {
	_, err := parseConfigFromEnv(mapEnv(map[string]string{
		"DEPLOYMENT_MODE":            "aws",
		"REDIS_ADDR":                 "redis:6379",
		"DYNAMO_ENABLED":             "true",
		"AWS_REGION":                 "us-west-2",
		"DYNAMO_TABLE_NAME":          "workout_jobs",
		"S3_ENABLED":                 "true",
		"S3_ENDPOINT":                "http://minio:9000",
		"S3_BUCKET":                  "workout-videos",
		"HEARTBEAT_INTERVAL_SECONDS": "2",
		"MAX_RETRIES":                "3",
	}))
	if err == nil {
		t.Fatal("expected config validation error, got nil")
	}
}

func mapEnv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
