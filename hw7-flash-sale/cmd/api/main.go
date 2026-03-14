package main

import (
	"context"
	"log"

	"hw7-flash-sale/internal/config"
	"hw7-flash-sale/internal/httpapi"
	"hw7-flash-sale/internal/messaging"
	"hw7-flash-sale/internal/orders"
	"hw7-flash-sale/internal/payment"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	paymentSimulator := payment.NewSimulator(cfg.SyncPaymentSlots, cfg.PaymentDelay)
	publisher, err := messaging.NewPublisher(context.Background(), cfg, log.Default())
	if err != nil {
		log.Fatalf("create publisher: %v", err)
	}
	orderService := orders.NewService(paymentSimulator, publisher)
	router := httpapi.NewRouter(orderService, cfg)

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("run api server: %v", err)
	}
}
