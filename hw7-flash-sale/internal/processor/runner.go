package processor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"hw7-flash-sale/internal/messaging"
	"hw7-flash-sale/internal/payment"
)

type Runner struct {
	consumer messaging.Consumer
	payment  *payment.Simulator
	workers  int
	logger   *log.Logger
}

func NewRunner(consumer messaging.Consumer, paymentSimulator *payment.Simulator, workers int, logger *log.Logger) *Runner {
	if logger == nil {
		logger = log.Default()
	}

	return &Runner{
		consumer: consumer,
		payment:  paymentSimulator,
		workers:  workers,
		logger:   logger,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, r.workers)

	for workerID := 1; workerID <= r.workers; workerID++ {
		wg.Add(1)

		go func(id int) {
			defer wg.Done()

			if err := r.runWorker(ctx, id); err != nil && !errors.Is(err, context.Canceled) {
				select {
				case errCh <- err:
				default:
				}
			}
		}(workerID)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		<-done
		return ctx.Err()
	}
}

func (r *Runner) runWorker(ctx context.Context, workerID int) error {
	for {
		msg, err := r.consumer.Receive(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			return err
		}

		orderID := msg.Event.Order.OrderID
		r.logger.Printf("worker=%d processing order_id=%s", workerID, orderID)

		if err := r.payment.Verify(ctx); err != nil {
			return fmt.Errorf("worker=%d verify order_id=%s: %w", workerID, orderID, err)
		}

		r.logger.Printf("worker=%d completed order_id=%s", workerID, orderID)

		if err := msg.Ack(); err != nil {
			return fmt.Errorf("worker=%d ack order_id=%s: %w", workerID, orderID, err)
		}
	}
}
