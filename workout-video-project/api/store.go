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
var ErrUploadSessionUnsupported = errors.New("upload sessions require durable store support")

type RedisJobStore struct {
	client *redis.Client
}

func NewRedisJobStore(client *redis.Client) *RedisJobStore {
	return &RedisJobStore{client: client}
}

func jobKey(jobID string) string {
	return fmt.Sprintf("job:%s", jobID)
}

func (s *RedisJobStore) CreateUploadJob(context.Context, JobRecord) error {
	return ErrUploadSessionUnsupported
}

func (s *RedisJobStore) FinalizeUploadJob(ctx context.Context, job JobRecord) error {
	fields := map[string]any{
		"job_id":                job.JobID,
		"status":                job.Status,
		"file_name":             job.FileName,
		"file_size_bytes":       job.FileSizeBytes,
		"content_type":          job.ContentType,
		"s3_key":                job.S3Key,
		"created_at":            job.CreatedAt,
		"started_at":            job.StartedAt,
		"completed_at":          job.CompletedAt,
		"processing_started_at": job.ProcessingStartedAt,
		"last_heartbeat_at":     job.LastHeartbeatAt,
		"worker_id":             job.WorkerID,
		"retry_count":           job.RetryCount,
		"failure_type":          job.FailureType,
		"error":                 job.Error,
		"result_json":           job.ResultJSON,
	}

	pipe := s.client.TxPipeline()
	pipe.HSet(ctx, jobKey(job.JobID), fields)
	pipe.RPush(ctx, PendingQueueKey, job.JobID)
	pipe.Incr(ctx, StatsJobsSubmitted)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisJobStore) GetJob(ctx context.Context, jobID string) (JobRecord, error) {
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

	retryCount, err := parseOptionalInt64(values["retry_count"])
	if err != nil {
		return JobRecord{}, fmt.Errorf("parse retry count: %w", err)
	}

	return JobRecord{
		JobID:               values["job_id"],
		Status:              values["status"],
		FileName:            values["file_name"],
		FileSizeBytes:       fileSize,
		ContentType:         values["content_type"],
		S3Key:               values["s3_key"],
		UploadID:            values["upload_id"],
		CreatedAt:           values["created_at"],
		StartedAt:           values["started_at"],
		CompletedAt:         values["completed_at"],
		ProcessingStartedAt: values["processing_started_at"],
		LastHeartbeatAt:     values["last_heartbeat_at"],
		WorkerID:            values["worker_id"],
		RetryCount:          retryCount,
		FailureType:         values["failure_type"],
		Error:               values["error"],
		ResultJSON:          values["result_json"],
	}, nil
}

func (s *RedisJobStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *RedisJobStore) GetMetrics(ctx context.Context) (MetricsResponse, error) {
	pipe := s.client.Pipeline()
	submitted := pipe.Get(ctx, StatsJobsSubmitted)
	completed := pipe.Get(ctx, StatsJobsCompleted)
	failed := pipe.Get(ctx, StatsJobsFailed)
	rejected := pipe.Get(ctx, StatsJobsRejected)
	recoveredStale := pipe.Get(ctx, StatsJobsRecoveredStale)
	recoveredFailed := pipe.Get(ctx, StatsJobsRecoveredFailed)
	poisonPill := pipe.Get(ctx, StatsJobsPoisonPill)
	pendingDepth := pipe.LLen(ctx, PendingQueueKey)
	processingDepth := pipe.LLen(ctx, ProcessingQueueKey)

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return MetricsResponse{}, err
	}

	return MetricsResponse{
		JobsSubmitted:        intCmdValue(submitted),
		JobsCompleted:        intCmdValue(completed),
		JobsFailed:           intCmdValue(failed),
		JobsRejected:         intCmdValue(rejected),
		JobsRecoveredStale:   intCmdValue(recoveredStale),
		JobsRecoveredFailed:  intCmdValue(recoveredFailed),
		JobsPoisonPill:       intCmdValue(poisonPill),
		PendingQueueDepth:    pendingDepth.Val(),
		ProcessingQueueDepth: processingDepth.Val(),
	}, nil
}

func buildJobResponse(job JobRecord) (JobResponse, error) {
	response := JobResponse{
		JobID:               job.JobID,
		Status:              job.Status,
		FileName:            job.FileName,
		FileSizeBytes:       job.FileSizeBytes,
		CreatedAt:           job.CreatedAt,
		ProcessingStartedAt: emptyStringToNil(job.ProcessingStartedAt),
		CompletedAt:         emptyStringToNil(job.CompletedAt),
		Error:               emptyStringToNil(job.Error),
		Result:              nil,
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

func parseOptionalInt64(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func intCmdValue(cmd *redis.StringCmd) int64 {
	value, err := cmd.Int64()
	if err != nil {
		return 0
	}
	return value
}
