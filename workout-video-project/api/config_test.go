package main

import "testing"

func TestParseConfigFromEnvLocal(t *testing.T) {
	cfg, err := parseConfigFromEnv(mapEnv(map[string]string{
		"DEPLOYMENT_MODE":               "local",
		"PORT":                          "8080",
		"REDIS_ADDR":                    "redis:6379",
		"DYNAMO_ENABLED":                "true",
		"AWS_REGION":                    "us-west-2",
		"DYNAMO_ENDPOINT":               "http://dynamodb-local:8000",
		"DYNAMO_TABLE_NAME":             "workout_jobs",
		"DYNAMO_AUTO_CREATE_TABLE":      "true",
		"S3_ENABLED":                    "true",
		"S3_ENDPOINT":                   "http://minio:9000",
		"S3_PUBLIC_ENDPOINT":            "http://localhost:9000",
		"S3_BUCKET":                     "workout-videos",
		"S3_ACCESS_KEY_ID":              "minioadmin",
		"S3_SECRET_ACCESS_KEY":          "minioadmin",
		"S3_USE_PATH_STYLE":             "true",
		"S3_AUTO_CREATE_BUCKET":         "true",
		"S3_PRESIGN_EXPIRATION_SECONDS": "3600",
		"MULTIPART_PART_SIZE_BYTES":     "5242880",
	}))
	if err != nil {
		t.Fatalf("parseConfigFromEnv returned error: %v", err)
	}

	if cfg.DeploymentMode != deploymentModeLocal {
		t.Fatalf("expected local deployment mode, got %s", cfg.DeploymentMode)
	}
}

func TestParseConfigFromEnvAWSRejectsLocalOverrides(t *testing.T) {
	_, err := parseConfigFromEnv(mapEnv(map[string]string{
		"DEPLOYMENT_MODE":               "aws",
		"PORT":                          "8080",
		"REDIS_ADDR":                    "redis:6379",
		"DYNAMO_ENABLED":                "true",
		"AWS_REGION":                    "us-west-2",
		"DYNAMO_TABLE_NAME":             "workout_jobs",
		"S3_ENABLED":                    "true",
		"S3_ENDPOINT":                   "http://minio:9000",
		"S3_BUCKET":                     "workout-videos",
		"S3_PRESIGN_EXPIRATION_SECONDS": "3600",
		"MULTIPART_PART_SIZE_BYTES":     "5242880",
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
