package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	statusQueued     = "queued"
	statusInProgress = "in_progress"
	statusFailed     = "failed"

	failureTypeStaleTimeout  = "stale_timeout"
	failureTypeProcessingErr = "processing_error"
	failureTypePoisonPill    = "poison_pill"

	statsJobsFailed          = "stats:jobs_failed"
	statsJobsRecoveredStale  = "stats:jobs_recovered_stale"
	statsJobsRecoveredFailed = "stats:jobs_recovered_failed"
	statsJobsPoisonPill      = "stats:jobs_poison_pill"

	timestampLayout = "2006-01-02T15:04:05.000Z"
)

type config struct {
	redisAddr          string
	redisPassword      string
	redisDB            int
	pendingQueueKey    string
	processingQueueKey string
	retryableFailedSet string
	reaperID           string
	maxRetries         int64
	reaperInterval     time.Duration
	staleTimeout       time.Duration
	dynamoEnabled      bool
	awsRegion          string
	dynamoEndpoint     string
	dynamoTableName    string
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

	ctx := context.Background()
	log.Printf(
		"reaper_started reaper_id=%s processing_queue=%s retryable_failed_set=%s reaper_interval=%s stale_timeout=%s max_retries=%d redis_addr=%s durable_store_enabled=%t",
		cfg.reaperID,
		cfg.processingQueueKey,
		cfg.retryableFailedSet,
		cfg.reaperInterval,
		cfg.staleTimeout,
		cfg.maxRetries,
		cfg.redisAddr,
		cfg.dynamoEnabled,
	)

	ticker := time.NewTicker(cfg.reaperInterval)
	defer ticker.Stop()

	for {
		runRecoveryCycle(ctx, client, durableStore, cfg)
		<-ticker.C
	}
}

func runRecoveryCycle(ctx context.Context, client *redis.Client, durableStore durableJobStore, cfg config) {
	recoverStaleProcessingJobs(ctx, client, durableStore, cfg)
	recoverRetryableFailedJobs(ctx, client, durableStore, cfg)
}

func recoverStaleProcessingJobs(ctx context.Context, client *redis.Client, durableStore durableJobStore, cfg config) {
	jobIDs, err := client.LRange(ctx, cfg.processingQueueKey, 0, -1).Result()
	if err != nil {
		log.Printf("stale_scan_failed reaper_id=%s error=%q", cfg.reaperID, err)
		return
	}

	for _, jobID := range jobIDs {
		if err := recoverProcessingJob(ctx, client, durableStore, cfg, jobID); err != nil {
			log.Printf("stale_recovery_failed reaper_id=%s job_id=%s error=%q", cfg.reaperID, jobID, err)
		}
	}
}

func recoverRetryableFailedJobs(ctx context.Context, client *redis.Client, durableStore durableJobStore, cfg config) {
	jobIDs, err := client.SMembers(ctx, cfg.retryableFailedSet).Result()
	if err != nil {
		log.Printf("failed_set_scan_failed reaper_id=%s error=%q", cfg.reaperID, err)
		return
	}

	for _, jobID := range jobIDs {
		if err := recoverFailedJob(ctx, client, durableStore, cfg, jobID); err != nil {
			log.Printf("failed_recovery_failed reaper_id=%s job_id=%s error=%q", cfg.reaperID, jobID, err)
		}
	}
}

func recoverProcessingJob(ctx context.Context, client *redis.Client, durableStore durableJobStore, cfg config, jobID string) error {
	jobKey := fmt.Sprintf("job:%s", jobID)

	for attempt := 0; attempt < 3; attempt++ {
		err := client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.HGetAll(ctx, jobKey).Result()
			if err != nil {
				return err
			}
			if len(values) == 0 {
				_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
					pipe.LRem(ctx, cfg.processingQueueKey, 1, jobID)
					return nil
				})
				return err
			}
			if values["status"] != statusInProgress {
				_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
					pipe.LRem(ctx, cfg.processingQueueKey, 1, jobID)
					return nil
				})
				return err
			}

			stale, err := isStale(values, cfg.staleTimeout)
			if err != nil {
				return err
			}
			if !stale {
				return nil
			}

			retryCount, err := parseOptionalInt64(values["retry_count"])
			if err != nil {
				return fmt.Errorf("parse retry_count: %w", err)
			}

			now := utcNow()
			var durableUpdate durableJobUpdate
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.LRem(ctx, cfg.processingQueueKey, 1, jobID)

				if retryCount >= cfg.maxRetries {
					pipe.HSet(ctx, jobKey, map[string]any{
						"status":       statusFailed,
						"completed_at": now,
						"failure_type": failureTypePoisonPill,
						"error":        "retry limit exceeded after stale timeout",
						"result_json":  "",
					})
					pipe.SRem(ctx, cfg.retryableFailedSet, jobID)
					pipe.Incr(ctx, statsJobsFailed)
					pipe.Incr(ctx, statsJobsPoisonPill)
					durableUpdate = durableJobUpdate{
						JobID:       jobID,
						Status:      statusFailed,
						RetryCount:  retryCount,
						CompletedAt: now,
						FailureType: failureTypePoisonPill,
						Error:       "retry limit exceeded after stale timeout",
					}
					return nil
				}

				pipe.HSet(ctx, jobKey, map[string]any{
					"status":                statusQueued,
					"completed_at":          "",
					"processing_started_at": "",
					"last_heartbeat_at":     "",
					"worker_id":             "",
					"retry_count":           retryCount + 1,
					"failure_type":          failureTypeStaleTimeout,
					"error":                 "requeued after stale timeout",
					"result_json":           "",
				})
				pipe.RPush(ctx, cfg.pendingQueueKey, jobID)
				pipe.Incr(ctx, statsJobsRecoveredStale)
				durableUpdate = durableJobUpdate{
					JobID:       jobID,
					Status:      statusQueued,
					RetryCount:  retryCount + 1,
					FailureType: failureTypeStaleTimeout,
					Error:       "requeued after stale timeout",
				}
				return nil
			})
			if err == nil {
				if durableErr := durableStore.MarkRecoveryOutcome(ctx, durableUpdate); durableErr != nil {
					log.Printf("durable_stale_recovery_update_failed reaper_id=%s job_id=%s error=%q", cfg.reaperID, jobID, durableErr)
				}
			}
			return err
		}, jobKey)

		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		if err == nil {
			return nil
		}
		return err
	}

	return fmt.Errorf("processing recovery conflicted repeatedly for %s", jobID)
}

func recoverFailedJob(ctx context.Context, client *redis.Client, durableStore durableJobStore, cfg config, jobID string) error {
	jobKey := fmt.Sprintf("job:%s", jobID)

	for attempt := 0; attempt < 3; attempt++ {
		err := client.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.HGetAll(ctx, jobKey).Result()
			if err != nil {
				return err
			}
			if len(values) == 0 {
				_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
					pipe.SRem(ctx, cfg.retryableFailedSet, jobID)
					return nil
				})
				return err
			}
			if values["status"] != statusFailed || values["failure_type"] != failureTypeProcessingErr {
				_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
					pipe.SRem(ctx, cfg.retryableFailedSet, jobID)
					return nil
				})
				return err
			}

			retryCount, err := parseOptionalInt64(values["retry_count"])
			if err != nil {
				return fmt.Errorf("parse retry_count: %w", err)
			}

			now := utcNow()
			var durableUpdate durableJobUpdate
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.SRem(ctx, cfg.retryableFailedSet, jobID)

				if retryCount >= cfg.maxRetries {
					pipe.HSet(ctx, jobKey, map[string]any{
						"status":       statusFailed,
						"completed_at": now,
						"failure_type": failureTypePoisonPill,
						"error":        "retry limit exceeded after processing error",
						"result_json":  "",
					})
					pipe.Incr(ctx, statsJobsFailed)
					pipe.Incr(ctx, statsJobsPoisonPill)
					durableUpdate = durableJobUpdate{
						JobID:       jobID,
						Status:      statusFailed,
						RetryCount:  retryCount,
						CompletedAt: now,
						FailureType: failureTypePoisonPill,
						Error:       "retry limit exceeded after processing error",
					}
					return nil
				}

				pipe.HSet(ctx, jobKey, map[string]any{
					"status":                statusQueued,
					"completed_at":          "",
					"processing_started_at": "",
					"last_heartbeat_at":     "",
					"worker_id":             "",
					"retry_count":           retryCount + 1,
					"failure_type":          failureTypeProcessingErr,
					"error":                 "requeued after processing error",
					"result_json":           "",
				})
				pipe.RPush(ctx, cfg.pendingQueueKey, jobID)
				pipe.Incr(ctx, statsJobsRecoveredFailed)
				durableUpdate = durableJobUpdate{
					JobID:       jobID,
					Status:      statusQueued,
					RetryCount:  retryCount + 1,
					FailureType: failureTypeProcessingErr,
					Error:       "requeued after processing error",
				}
				return nil
			})
			if err == nil {
				if durableErr := durableStore.MarkRecoveryOutcome(ctx, durableUpdate); durableErr != nil {
					log.Printf("durable_failed_recovery_update_failed reaper_id=%s job_id=%s error=%q", cfg.reaperID, jobID, durableErr)
				}
			}
			return err
		}, jobKey)

		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		if err == nil {
			return nil
		}
		return err
	}

	return fmt.Errorf("failed-job recovery conflicted repeatedly for %s", jobID)
}

func isStale(values map[string]string, staleTimeout time.Duration) (bool, error) {
	referenceTime := values["last_heartbeat_at"]
	if referenceTime == "" {
		referenceTime = values["processing_started_at"]
	}
	if referenceTime == "" {
		return true, nil
	}

	parsed, err := time.Parse(timestampLayout, referenceTime)
	if err != nil {
		return false, fmt.Errorf("parse heartbeat timestamp: %w", err)
	}
	return time.Since(parsed) > staleTimeout, nil
}

func parseOptionalInt64(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func utcNow() string {
	return time.Now().UTC().Format(timestampLayout)
}

func newToken(prefix string, size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(bytes), nil
}
