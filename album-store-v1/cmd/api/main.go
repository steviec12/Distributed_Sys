package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"album-store-v1/internal/api"
	"album-store-v1/internal/config"
	"album-store-v1/internal/pipeline"
	"album-store-v1/internal/storage"
	"album-store-v1/internal/store"
)

// main boots the HTTP API with its storage dependencies.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
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
	photoPipeline := pipeline.New(dbStore, objectStorage, tempStorage, 0)
	if err := photoPipeline.RecoverStale(ctx); err != nil {
		log.Fatalf("recover stale photos: %v", err)
	}
	photoPipeline.Start(ctx, cfg.WorkerConcurrency)

	router := api.NewRouter(api.Dependencies{
		Albums:    dbStore,
		Photos:    dbStore,
		Objects:   objectStorage,
		TempFiles: tempStorage,
		Pipeline:  photoPipeline,
	})

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	var serveErr error
	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
		stop()
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("api shutdown error: %v", err)
	}

	photoPipeline.Stop()

	if serveErr != nil {
		log.Fatalf("api server failed: %v", serveErr)
	}
}
