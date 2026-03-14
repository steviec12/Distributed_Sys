package messaging

import (
	"context"
	"fmt"
	"log"

	appconfig "hw7-flash-sale/internal/config"
)

func NewPublisher(ctx context.Context, cfg appconfig.Config, logger *log.Logger) (Publisher, error) {
	switch cfg.MessagingBackend {
	case "file":
		return NewFilePublisher(cfg.EventsDir, logger), nil
	case "aws":
		return NewSNSPublisher(ctx, cfg, logger)
	default:
		return nil, fmt.Errorf("unsupported messaging backend: %s", cfg.MessagingBackend)
	}
}

func NewConsumer(ctx context.Context, cfg appconfig.Config, logger *log.Logger) (Consumer, error) {
	switch cfg.MessagingBackend {
	case "file":
		return NewFileConsumer(cfg.EventsDir, cfg.ProcessorPollInterval, logger), nil
	case "aws":
		return NewSQSConsumer(ctx, cfg, logger)
	default:
		return nil, fmt.Errorf("unsupported messaging backend: %s", cfg.MessagingBackend)
	}
}
