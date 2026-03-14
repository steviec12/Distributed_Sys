package messaging

import (
	"context"
	"encoding/json"
	"log"

	"hw7-flash-sale/internal/models"
)

type Publisher interface {
	PublishOrderCreated(ctx context.Context, event models.OrderCreatedEvent) error
}

type LogPublisher struct {
	logger *log.Logger
}

func NewLogPublisher(logger *log.Logger) *LogPublisher {
	if logger == nil {
		logger = log.Default()
	}

	return &LogPublisher{logger: logger}
}

func (p *LogPublisher) PublishOrderCreated(_ context.Context, event models.OrderCreatedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	p.logger.Printf("local async publish: %s", payload)
	return nil
}
