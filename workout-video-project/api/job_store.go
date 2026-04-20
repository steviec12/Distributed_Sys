package main

import "context"

type JobStore interface {
	CreateUploadJob(ctx context.Context, job JobRecord) error
	FinalizeUploadJob(ctx context.Context, job JobRecord) error
	GetJob(ctx context.Context, jobID string) (JobRecord, error)
	Ping(ctx context.Context) error
	GetMetrics(ctx context.Context) (MetricsResponse, error)
}
