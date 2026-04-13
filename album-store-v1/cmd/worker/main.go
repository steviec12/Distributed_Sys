package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"album-store-v1/internal/config"
	"album-store-v1/internal/queue"
	"album-store-v1/internal/storage"
	"album-store-v1/internal/store"
	"album-store-v1/internal/worker"
)

// main boots the background worker that publishes pending jobs and finalizes photos.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.SQSQueueURL == "" {
		log.Fatalf("SQS_QUEUE_URL is required for cmd/worker")
	}

	dbStore, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer dbStore.Close()

	objectStorage, err := storage.NewS3Storage(ctx, cfg.AWSRegion, cfg.S3Bucket, cfg.PublicS3BaseURL)
	if err != nil {
		log.Fatalf("open object storage: %v", err)
	}
	tempStorage := storage.NewLocalTempStorage("")

	sqsClient, err := queue.NewSQSPublisher(ctx, cfg.AWSRegion, cfg.SQSQueueURL)
	if err != nil {
		log.Fatalf("open queue: %v", err)
	}

	service := worker.Service{
		Jobs:      dbStore,
		Photos:    dbStore,
		Objects:   objectStorage,
		TempFiles: tempStorage,
		Queue:     sqsClient,
		Publish:   sqsClient,
	}

	if err := service.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("worker failed: %v", err)
	}
}
