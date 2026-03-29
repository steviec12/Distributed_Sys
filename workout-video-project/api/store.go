package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

var ErrJobNotFound = errors.New("job not found")

type JobStore struct {
	client *redis.Client
}

func NewJobStore(client *redis.Client) *JobStore {
	return &JobStore{client: client}
}

func jobKey(jobID string) string {
	return fmt.Sprintf("job:%s", jobID)
}

func (s *JobStore) CreateJob(ctx context.Context, job JobRecord) error {
	fields := map[string]any{
		"job_id":          job.JobID,
		"status":          job.Status,
		"file_path":       job.FilePath,
		"file_name":       job.FileName,
		"file_size_bytes": job.FileSizeBytes,
		"created_at":      job.CreatedAt,
		"started_at":      job.StartedAt,
		"completed_at":    job.CompletedAt,
		"error":           job.Error,
		"result_json":     job.ResultJSON,
	}

	pipe := s.client.TxPipeline()
	pipe.HSet(ctx, jobKey(job.JobID), fields)
	pipe.RPush(ctx, PendingQueueKey, job.JobID)
	pipe.Incr(ctx, StatsJobsSubmitted)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *JobStore) GetJob(ctx context.Context, jobID string) (JobRecord, error) {
	values, err := s.client.HGetAll(ctx, jobKey(jobID)).Result()
	if err != nil {
		return JobRecord{}, err
	}
	if len(values) == 0 {
		return JobRecord{}, ErrJobNotFound
	}

	fileSize, err := strconv.ParseInt(values["file_size_bytes"], 10, 64)
	if err != nil {
		return JobRecord{}, fmt.Errorf("parse file size: %w", err)
	}

	return JobRecord{
		JobID:         values["job_id"],
		Status:        values["status"],
		FilePath:      values["file_path"],
		FileName:      values["file_name"],
		FileSizeBytes: fileSize,
		CreatedAt:     values["created_at"],
		StartedAt:     values["started_at"],
		CompletedAt:   values["completed_at"],
		Error:         values["error"],
		ResultJSON:    values["result_json"],
	}, nil
}

func (s *JobStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func buildJobResponse(job JobRecord) (JobResponse, error) {
	response := JobResponse{
		JobID:         job.JobID,
		Status:        job.Status,
		FileName:      job.FileName,
		FileSizeBytes: job.FileSizeBytes,
		CreatedAt:     job.CreatedAt,
		StartedAt:     emptyStringToNil(job.StartedAt),
		CompletedAt:   emptyStringToNil(job.CompletedAt),
		Error:         emptyStringToNil(job.Error),
		Result:        nil,
	}

	if job.ResultJSON == "" {
		return response, nil
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(job.ResultJSON), &result); err != nil {
		return JobResponse{}, fmt.Errorf("decode result_json: %w", err)
	}

	response.Result = result
	return response, nil
}

func emptyStringToNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
