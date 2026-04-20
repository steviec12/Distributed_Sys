package main

import (
	"context"
	"fmt"
)

type HybridJobStore struct {
	redisStore   *RedisJobStore
	durableStore DurableJobStore
}

func NewHybridJobStore(redisStore *RedisJobStore, durableStore DurableJobStore) *HybridJobStore {
	return &HybridJobStore{
		redisStore:   redisStore,
		durableStore: durableStore,
	}
}

func (s *HybridJobStore) CreateUploadJob(ctx context.Context, job JobRecord) error {
	return s.durableStore.PutJob(ctx, job)
}

func (s *HybridJobStore) FinalizeUploadJob(ctx context.Context, job JobRecord) error {
	if err := s.durableStore.PutJob(ctx, job); err != nil {
		return err
	}

	if err := s.redisStore.FinalizeUploadJob(ctx, job); err != nil {
		return fmt.Errorf("finalize redis job: %w", err)
	}

	return nil
}

func (s *HybridJobStore) GetJob(ctx context.Context, jobID string) (JobRecord, error) {
	job, err := s.durableStore.GetJob(ctx, jobID)
	if err != nil {
		return JobRecord{}, err
	}

	liveJob, err := s.redisStore.GetJob(ctx, jobID)
	if err == nil {
		job.LastHeartbeatAt = liveJob.LastHeartbeatAt
	}

	return job, nil
}

func (s *HybridJobStore) Ping(ctx context.Context) error {
	if err := s.redisStore.Ping(ctx); err != nil {
		return err
	}
	return s.durableStore.Ping(ctx)
}

func (s *HybridJobStore) GetMetrics(ctx context.Context) (MetricsResponse, error) {
	return s.redisStore.GetMetrics(ctx)
}
