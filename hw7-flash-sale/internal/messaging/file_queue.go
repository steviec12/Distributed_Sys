package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"hw7-flash-sale/internal/models"
)

type ReceivedOrderCreatedEvent struct {
	Event models.OrderCreatedEvent
	Ack   func() error
}

type Consumer interface {
	Receive(ctx context.Context) (ReceivedOrderCreatedEvent, error)
}

type FilePublisher struct {
	dir    string
	logger *log.Logger
	seq    atomic.Uint64
}

func NewFilePublisher(dir string, logger *log.Logger) *FilePublisher {
	if logger == nil {
		logger = log.Default()
	}

	return &FilePublisher{
		dir:    dir,
		logger: logger,
	}
}

func (p *FilePublisher) PublishOrderCreated(ctx context.Context, event models.OrderCreatedEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%d-%06d-%s.json", time.Now().UTC().UnixNano(), p.seq.Add(1), event.Order.OrderID)
	tmpPath := filepath.Join(p.dir, filename+".tmp")
	finalPath := filepath.Join(p.dir, filename)

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return err
	}

	p.logger.Printf("queued local order event file=%s order_id=%s", finalPath, event.Order.OrderID)
	return nil
}

type FileConsumer struct {
	dir          string
	pollInterval time.Duration
	logger       *log.Logger
}

func NewFileConsumer(dir string, pollInterval time.Duration, logger *log.Logger) *FileConsumer {
	if logger == nil {
		logger = log.Default()
	}

	return &FileConsumer{
		dir:          dir,
		pollInterval: pollInterval,
		logger:       logger,
	}
}

func (c *FileConsumer) Receive(ctx context.Context) (ReceivedOrderCreatedEvent, error) {
	for {
		claimedPath, err := c.claimNextFile()
		if err != nil {
			return ReceivedOrderCreatedEvent{}, err
		}

		if claimedPath != "" {
			payload, err := os.ReadFile(claimedPath)
			if err != nil {
				return ReceivedOrderCreatedEvent{}, err
			}

			var event models.OrderCreatedEvent
			if err := json.Unmarshal(payload, &event); err != nil {
				_ = os.Remove(claimedPath)
				return ReceivedOrderCreatedEvent{}, err
			}

			c.logger.Printf("claimed local order event file=%s order_id=%s", claimedPath, event.Order.OrderID)

			return ReceivedOrderCreatedEvent{
				Event: event,
				Ack: func() error {
					return os.Remove(claimedPath)
				},
			}, nil
		}

		timer := time.NewTimer(c.pollInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ReceivedOrderCreatedEvent{}, ctx.Err()
		}
	}
}

func (c *FileConsumer) claimNextFile() (string, error) {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return "", err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		originalPath := filepath.Join(c.dir, entry.Name())
		claimedPath := strings.TrimSuffix(originalPath, ".json") + ".processing"

		if err := os.Rename(originalPath, claimedPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", err
		}

		return claimedPath, nil
	}

	return "", nil
}
