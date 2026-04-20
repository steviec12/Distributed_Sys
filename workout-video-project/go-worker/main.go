package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	statusQueued     = "queued"
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
	statusFailed     = "failed"

	failureTypeNone          = "none"
	failureTypeProcessingErr = "processing_error"
	failureTypePoisonPill    = "poison_pill"

	statsJobsCompleted  = "stats:jobs_completed"
	statsJobsFailed     = "stats:jobs_failed"
	statsJobsPoisonPill = "stats:jobs_poison_pill"
)

var errAttemptLost = errors.New("job attempt ownership lost")

type config struct {
	redisAddr          string
	redisPassword      string
	redisDB            int
	pendingQueueKey    string
	processingQueueKey string
	retryableFailedSet string
	workerID           string
	heartbeatInterval  time.Duration
	maxRetries         int64
	dynamoEnabled      bool
	awsRegion          string
	dynamoEndpoint     string
	dynamoTableName    string
	s3Enabled          bool
	s3Endpoint         string
	s3Bucket           string
	s3AccessKeyID      string
	s3SecretAccessKey  string
	s3UsePathStyle     bool
}

type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

type jobResult struct {
	DurationSeconds          float64 `json:"duration_seconds"`
	SimulatedAnalysisSeconds float64 `json:"simulated_analysis_seconds"`
}

func main() {
	cfg := loadConfig()
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.redisAddr,
		Password: cfg.redisPassword,
		DB:       cfg.redisDB,
	})
	durableStore, err := newDurableJobStore(context.Background(), cfg)
	if err != nil {
		log.Fatalf("create durable job store: %v", err)
	}
	objectStore, err := newObjectStore(context.Background(), cfg)
	if err != nil {
		log.Fatalf("create object store: %v", err)
	}

	dequeueCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stopSignals()

	log.Printf(
		"worker_started worker_id=%s pending_queue=%s processing_queue=%s retryable_failed_set=%s heartbeat_interval=%s max_retries=%d redis_addr=%s durable_store_enabled=%t",
		cfg.workerID,
		cfg.pendingQueueKey,
		cfg.processingQueueKey,
		cfg.retryableFailedSet,
		cfg.heartbeatInterval,
		cfg.maxRetries,
		cfg.redisAddr,
		cfg.dynamoEnabled,
	)

dequeueLoop:
	for {
		jobID, err := client.BLMove(dequeueCtx, cfg.pendingQueueKey, cfg.processingQueueKey, "LEFT", "RIGHT", 0).Result()
		if err != nil {
			if dequeueCtx.Err() != nil {
				log.Printf("worker_shutdown_requested worker_id=%s reason=%q", cfg.workerID, dequeueCtx.Err())
				break
			}
			log.Printf("queue_pop_failed worker_id=%s error=%q", cfg.workerID, err)
			select {
			case <-time.After(time.Second):
			case <-dequeueCtx.Done():
				log.Printf("worker_shutdown_requested worker_id=%s reason=%q", cfg.workerID, dequeueCtx.Err())
				break dequeueLoop
			}
			continue
		}

		log.Printf("job_dequeued worker_id=%s job_id=%s", cfg.workerID, jobID)
		processJob(context.Background(), client, durableStore, objectStore, cfg, jobID)

		if dequeueCtx.Err() != nil {
			log.Printf("worker_shutdown_complete worker_id=%s", cfg.workerID)
			break
		}
	}

	log.Printf("worker_stopped worker_id=%s", cfg.workerID)
}

func processJob(ctx context.Context, client *redis.Client, durableStore durableJobStore, objectStore objectStore, cfg config, jobID string) {
	jobKey := fmt.Sprintf("job:%s", jobID)
	job, err := client.HGetAll(ctx, jobKey).Result()
	if err != nil {
		log.Printf("job_load_failed worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, err)
		return
	}
	if len(job) == 0 {
		log.Printf("job_missing worker_id=%s job_id=%s", cfg.workerID, jobID)
		removeFromProcessing(ctx, client, cfg.processingQueueKey, cfg.workerID, jobID)
		return
	}

	expectedRetryCount, err := parseOptionalInt64(job["retry_count"])
	if err != nil {
		log.Printf("job_retry_count_parse_failed worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, err)
		return
	}

	durableJob, err := durableStore.GetJob(ctx, jobID)
	if err != nil {
		log.Printf("durable_job_load_failed worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, err)
		finalizeProcessingFailure(ctx, client, durableStore, cfg, jobKey, jobID, expectedRetryCount, err)
		return
	}
	if durableJob.Status != statusQueued {
		err := fmt.Errorf("durable job status must be queued before processing, got %s", durableJob.Status)
		log.Printf("durable_job_invalid_status worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, err)
		finalizeProcessingFailure(ctx, client, durableStore, cfg, jobKey, jobID, expectedRetryCount, err)
		return
	}
	if durableJob.S3Key == "" {
		err := fmt.Errorf("durable job is missing s3_key")
		log.Printf("durable_job_missing_s3_key worker_id=%s job_id=%s", cfg.workerID, jobID)
		finalizeProcessingFailure(ctx, client, durableStore, cfg, jobKey, jobID, expectedRetryCount, err)
		return
	}

	processingStartedAt, err := markJobInProgress(ctx, client, jobKey, cfg, job)
	if err != nil {
		log.Printf("job_start_update_failed worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, err)
		return
	}
	if err := durableStore.MarkInProgress(ctx, durableJobUpdate{
		JobID:               jobID,
		StartedAt:           processingStartedAt,
		ProcessingStartedAt: processingStartedAt,
		WorkerID:            cfg.workerID,
		FailureType:         failureTypeNone,
	}); err != nil {
		log.Printf("durable_job_start_update_failed worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, err)
	}

	filePath, cleanup, err := resolveProcessingFile(ctx, objectStore, durableJob.S3Key)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		finalizeProcessingFailure(ctx, client, durableStore, cfg, jobKey, jobID, expectedRetryCount, err)
		return
	}

	durationSeconds, err := probeDurationSeconds(filePath)
	if err != nil {
		finalizeProcessingFailure(ctx, client, durableStore, cfg, jobKey, jobID, expectedRetryCount, err)
		return
	}

	simulatedSeconds := simulatedAnalysisSeconds(durationSeconds)
	log.Printf(
		"job_processing worker_id=%s job_id=%s retry_count=%d duration_seconds=%.3f simulated_analysis_seconds=%.2f",
		cfg.workerID,
		jobID,
		expectedRetryCount,
		durationSeconds,
		simulatedSeconds,
	)

	processingDuration := time.Duration(simulatedSeconds * float64(time.Second))
	if err := runProcessingWithHeartbeat(ctx, client, jobKey, cfg, jobID, expectedRetryCount, processingDuration); err != nil {
		if errors.Is(err, errAttemptLost) {
			log.Printf("job_attempt_lost worker_id=%s job_id=%s expected_retry_count=%d", cfg.workerID, jobID, expectedRetryCount)
			return
		}
		log.Printf("job_processing_aborted worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, err)
		return
	}

	resultJSON, err := json.Marshal(jobResult{
		DurationSeconds:          durationSeconds,
		SimulatedAnalysisSeconds: simulatedSeconds,
	})
	if err != nil {
		finalizeProcessingFailure(ctx, client, durableStore, cfg, jobKey, jobID, expectedRetryCount, err)
		return
	}

	completedAt := utcNow()
	if err := markJobCompleted(ctx, client, cfg, jobKey, jobID, expectedRetryCount, completedAt, string(resultJSON)); err != nil {
		if errors.Is(err, errAttemptLost) {
			log.Printf("job_complete_attempt_lost worker_id=%s job_id=%s expected_retry_count=%d", cfg.workerID, jobID, expectedRetryCount)
			return
		}
		log.Printf("job_complete_update_failed worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, err)
		return
	}
	if err := durableStore.MarkCompleted(ctx, durableJobUpdate{
		JobID:       jobID,
		CompletedAt: completedAt,
		FailureType: failureTypeNone,
		ResultJSON:  string(resultJSON),
	}); err != nil {
		log.Printf("durable_job_complete_update_failed worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, err)
	}

	log.Printf("job_completed worker_id=%s job_id=%s retry_count=%d", cfg.workerID, jobID, expectedRetryCount)
}

func markJobInProgress(ctx context.Context, client *redis.Client, jobKey string, cfg config, job map[string]string) (string, error) {
	now := utcNow()
	fields := map[string]any{
		"status":                statusInProgress,
		"processing_started_at": now,
		"last_heartbeat_at":     now,
		"worker_id":             cfg.workerID,
		"completed_at":          "",
		"failure_type":          failureTypeNone,
		"error":                 "",
		"result_json":           "",
	}
	if job["started_at"] == "" {
		fields["started_at"] = now
	}

	pipe := client.TxPipeline()
	pipe.HSet(ctx, jobKey, fields)
	pipe.SRem(ctx, cfg.retryableFailedSet, job["job_id"])
	_, err := pipe.Exec(ctx)
	if err != nil {
		return "", err
	}
	return now, nil
}

func resolveProcessingFile(ctx context.Context, objectStore objectStore, s3Key string) (string, func(), error) {
	if s3Key == "" {
		return "", nil, fmt.Errorf("job is missing s3_key")
	}

	tempPath, cleanup, err := objectStore.DownloadToTemp(ctx, s3Key)
	if err != nil {
		return "", nil, err
	}
	return tempPath, cleanup, nil
}

func runProcessingWithHeartbeat(
	ctx context.Context,
	client *redis.Client,
	jobKey string,
	cfg config,
	jobID string,
	expectedRetryCount int64,
	totalDuration time.Duration,
) error {
	if totalDuration <= 0 {
		return nil
	}

	timer := time.NewTimer(totalDuration)
	defer timer.Stop()

	ticker := time.NewTicker(cfg.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return nil
		case <-ticker.C:
			if err := refreshHeartbeat(ctx, client, jobKey, cfg.workerID, jobID, expectedRetryCount); err != nil {
				return err
			}
		}
	}
}

func refreshHeartbeat(
	ctx context.Context,
	client *redis.Client,
	jobKey string,
	workerID string,
	jobID string,
	expectedRetryCount int64,
) error {
	err := withActiveAttempt(ctx, client, jobKey, expectedRetryCount, func(_ map[string]string, pipe redis.Pipeliner) error {
		pipe.HSet(ctx, jobKey, map[string]any{
			"last_heartbeat_at": utcNow(),
		})
		return nil
	})
	if err == nil {
		log.Printf("job_heartbeat worker_id=%s job_id=%s expected_retry_count=%d", workerID, jobID, expectedRetryCount)
	}
	return err
}

func markJobCompleted(
	ctx context.Context,
	client *redis.Client,
	cfg config,
	jobKey string,
	jobID string,
	expectedRetryCount int64,
	completedAt string,
	resultJSON string,
) error {
	return withActiveAttempt(ctx, client, jobKey, expectedRetryCount, func(_ map[string]string, pipe redis.Pipeliner) error {
		pipe.HSet(ctx, jobKey, map[string]any{
			"status":            statusCompleted,
			"completed_at":      completedAt,
			"last_heartbeat_at": completedAt,
			"failure_type":      failureTypeNone,
			"error":             "",
			"result_json":       resultJSON,
		})
		pipe.LRem(ctx, cfg.processingQueueKey, 1, jobID)
		pipe.SRem(ctx, cfg.retryableFailedSet, jobID)
		pipe.Incr(ctx, statsJobsCompleted)
		return nil
	})
}

func finalizeProcessingFailure(
	ctx context.Context,
	client *redis.Client,
	durableStore durableJobStore,
	cfg config,
	jobKey string,
	jobID string,
	expectedRetryCount int64,
	cause error,
) {
	var durableUpdate durableJobUpdate
	err := withActiveAttempt(ctx, client, jobKey, expectedRetryCount, func(_ map[string]string, pipe redis.Pipeliner) error {
		now := utcNow()
		baseFields := map[string]any{
			"status":            statusFailed,
			"last_heartbeat_at": now,
			"result_json":       "",
		}
		durableUpdate = durableJobUpdate{
			JobID:       jobID,
			FailureType: failureTypeProcessingErr,
			Error:       cause.Error(),
		}

		pipe.LRem(ctx, cfg.processingQueueKey, 1, jobID)

		if expectedRetryCount >= cfg.maxRetries {
			baseFields["completed_at"] = now
			baseFields["failure_type"] = failureTypePoisonPill
			baseFields["error"] = fmt.Sprintf("retry limit exceeded after processing error: %v", cause)
			pipe.HSet(ctx, jobKey, baseFields)
			pipe.SRem(ctx, cfg.retryableFailedSet, jobID)
			pipe.Incr(ctx, statsJobsFailed)
			pipe.Incr(ctx, statsJobsPoisonPill)
			durableUpdate.CompletedAt = now
			durableUpdate.FailureType = failureTypePoisonPill
			durableUpdate.Error = baseFields["error"].(string)
			return nil
		}

		baseFields["completed_at"] = ""
		baseFields["failure_type"] = failureTypeProcessingErr
		baseFields["error"] = cause.Error()
		pipe.HSet(ctx, jobKey, baseFields)
		pipe.SAdd(ctx, cfg.retryableFailedSet, jobID)
		return nil
	})

	if err == nil {
		if durableErr := durableStore.MarkFailed(ctx, durableUpdate); durableErr != nil {
			log.Printf("durable_job_failed_update_failed worker_id=%s job_id=%s error=%q", cfg.workerID, jobID, durableErr)
		}
		log.Printf("job_failed worker_id=%s job_id=%s expected_retry_count=%d error=%q", cfg.workerID, jobID, expectedRetryCount, cause)
		return
	}
	if errors.Is(err, errAttemptLost) {
		log.Printf("job_failed_attempt_lost worker_id=%s job_id=%s expected_retry_count=%d error=%q", cfg.workerID, jobID, expectedRetryCount, cause)
		return
	}
	log.Printf("job_failed_update_failed worker_id=%s job_id=%s error=%q original_error=%q", cfg.workerID, jobID, err, cause)
}

func withActiveAttempt(
	ctx context.Context,
	client *redis.Client,
	jobKey string,
	expectedRetryCount int64,
	fn func(values map[string]string, pipe redis.Pipeliner) error,
) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.HGetAll(ctx, jobKey).Result()
			if err != nil {
				return err
			}
			if len(values) == 0 {
				return errAttemptLost
			}
			currentRetryCount, err := parseOptionalInt64(values["retry_count"])
			if err != nil {
				return fmt.Errorf("parse retry_count: %w", err)
			}
			if values["status"] != statusInProgress || currentRetryCount != expectedRetryCount {
				return errAttemptLost
			}

			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				return fn(values, pipe)
			})
			return err
		}, jobKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return err
	}

	return fmt.Errorf("attempt transaction conflicted repeatedly for %s", jobKey)
}

func removeFromProcessing(ctx context.Context, client *redis.Client, processingQueueKey string, workerID string, jobID string) bool {
	removed, err := client.LRem(ctx, processingQueueKey, 1, jobID).Result()
	if err != nil {
		log.Printf("processing_remove_failed worker_id=%s job_id=%s error=%q", workerID, jobID, err)
		return false
	}
	if removed == 0 {
		log.Printf("processing_remove_missing worker_id=%s job_id=%s", workerID, jobID)
		return false
	}
	return true
}

func probeDurationSeconds(filePath string) (float64, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v",
		"error",
		"-show_entries",
		"format=duration",
		"-of",
		"json",
		filePath,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("run ffprobe: %w", err)
	}

	var payload ffprobeOutput
	if err := json.Unmarshal(output, &payload); err != nil {
		return 0, fmt.Errorf("decode ffprobe output: %w", err)
	}
	if payload.Format.Duration == "" {
		return 0, fmt.Errorf("ffprobe did not return duration")
	}

	duration, err := strconv.ParseFloat(payload.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return round(duration, 3), nil
}

func simulatedAnalysisSeconds(durationSeconds float64) float64 {
	rawSeconds := durationSeconds * 0.2
	boundedSeconds := math.Min(math.Max(rawSeconds, 1.0), 20.0)
	return round(boundedSeconds, 2)
}

func parseOptionalInt64(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func round(value float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(value*scale) / scale
}

func utcNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func newToken(prefix string, size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(bytes), nil
}
