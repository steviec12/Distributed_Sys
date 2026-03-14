package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"

	"hw7-flash-sale/internal/config"
	"hw7-flash-sale/internal/messaging"
	"hw7-flash-sale/internal/payment"
	"hw7-flash-sale/internal/processor"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	consumer, err := messaging.NewConsumer(ctx, cfg, log.Default())
	if err != nil {
		log.Fatalf("create consumer: %v", err)
	}
	paymentSimulator := payment.NewSimulator(cfg.ProcessorWorkerCount, cfg.PaymentDelay)
	runner := processor.NewRunner(consumer, paymentSimulator, cfg.ProcessorWorkerCount, log.Default())

	log.Printf(
		"starting processor backend=%s events_dir=%s queue_url_set=%t workers=%d payment_delay=%s poll_interval=%s",
		cfg.MessagingBackend,
		cfg.EventsDir,
		cfg.SQSQueueURL != "",
		cfg.ProcessorWorkerCount,
		cfg.PaymentDelay,
		cfg.ProcessorPollInterval,
	)

	if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run processor: %v", err)
	}
}
