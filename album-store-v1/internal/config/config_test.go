package config

import "testing"

func TestLoadUsesEnvironment(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/album_store")
	t.Setenv("AWS_REGION", "us-west-2")
	t.Setenv("S3_BUCKET", "album-store-test")
	t.Setenv("SQS_QUEUE_URL", "https://sqs.us-west-2.amazonaws.com/123/photos")
	t.Setenv("PUBLIC_S3_BASE_URL", "https://cdn.example.com")
	t.Setenv("WORKER_CONCURRENCY", "42")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}

	if cfg.Port != "9090" {
		t.Fatalf("expected port from env, got %q", cfg.Port)
	}
	if cfg.DatabaseURL == "" || cfg.AWSRegion == "" || cfg.S3Bucket == "" || cfg.SQSQueueURL == "" {
		t.Fatalf("expected required fields to be populated: %+v", cfg)
	}
	if cfg.WorkerConcurrency != 42 {
		t.Fatalf("expected worker concurrency from env, got %d", cfg.WorkerConcurrency)
	}
	if cfg.PublicS3BaseURL != "https://cdn.example.com" {
		t.Fatalf("expected explicit public S3 base URL, got %q", cfg.PublicS3BaseURL)
	}
}

func TestLoadDefaultsPortAndPublicS3BaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/album_store")
	t.Setenv("AWS_REGION", "us-west-2")
	t.Setenv("S3_BUCKET", "album-store-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}

	if cfg.Port != "8080" {
		t.Fatalf("expected default port, got %q", cfg.Port)
	}
	if cfg.PublicS3BaseURL != "https://album-store-test.s3.us-west-2.amazonaws.com" {
		t.Fatalf("expected generated public S3 base URL, got %q", cfg.PublicS3BaseURL)
	}
	if cfg.WorkerConcurrency != 30 {
		t.Fatalf("expected default worker concurrency, got %d", cfg.WorkerConcurrency)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-2")
	t.Setenv("S3_BUCKET", "album-store-test")

	if _, err := Load(); err == nil {
		t.Fatal("expected missing DATABASE_URL to fail")
	}
}
