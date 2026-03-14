package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"hw7-flash-sale/internal/models"
	"hw7-flash-sale/internal/payment"
)

var coldStart atomic.Bool

type handler struct {
	logger  *log.Logger
	payment *payment.Simulator
}

func newHandler() (*handler, error) {
	delaySeconds, err := getEnvInt("PAYMENT_DELAY_SECONDS", 3)
	if err != nil {
		return nil, fmt.Errorf("parse PAYMENT_DELAY_SECONDS: %w", err)
	}
	if delaySeconds < 1 {
		return nil, fmt.Errorf("PAYMENT_DELAY_SECONDS must be at least 1")
	}

	return &handler{
		logger:  log.Default(),
		payment: payment.NewSimulator(1, time.Duration(delaySeconds)*time.Second),
	}, nil
}

func (h *handler) Handle(ctx context.Context, event events.SNSEvent) error {
	isColdStart := coldStart.Swap(false)
	h.logger.Printf("lambda invocation cold_start=%t records=%d", isColdStart, len(event.Records))

	for _, record := range event.Records {
		var orderEvent models.OrderCreatedEvent
		if err := json.Unmarshal([]byte(record.SNS.Message), &orderEvent); err != nil {
			return fmt.Errorf("unmarshal sns message_id=%s: %w", record.SNS.MessageID, err)
		}

		orderID := orderEvent.Order.OrderID
		h.logger.Printf("lambda processing order_id=%s message_id=%s", orderID, record.SNS.MessageID)

		if err := h.payment.Verify(ctx); err != nil {
			return fmt.Errorf("verify order_id=%s: %w", orderID, err)
		}

		h.logger.Printf("lambda completed order_id=%s", orderID)
	}

	return nil
}

func getEnvInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	return strconv.Atoi(value)
}

func main() {
	coldStart.Store(true)

	h, err := newHandler()
	if err != nil {
		log.Fatalf("create lambda handler: %v", err)
	}

	lambda.Start(h.Handle)
}
