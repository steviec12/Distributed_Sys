package orders

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"hw7-flash-sale/internal/messaging"
	"hw7-flash-sale/internal/models"
	"hw7-flash-sale/internal/payment"
)

type Service struct {
	payment *payment.Simulator
	pub     messaging.Publisher
	seq     atomic.Uint64
	orders  sync.Map
}

func NewService(paymentSimulator *payment.Simulator, publisher messaging.Publisher) *Service {
	return &Service{
		payment: paymentSimulator,
		pub:     publisher,
	}
}

func (s *Service) ProcessSync(ctx context.Context, req models.CreateOrderRequest) (models.Order, error) {
	order := s.newOrder(req)
	s.save(order)

	order.Status = models.StatusProcessing
	order.UpdatedAt = time.Now().UTC()
	s.save(order)

	if err := s.payment.Verify(ctx); err != nil {
		return models.Order{}, err
	}

	order.Status = models.StatusCompleted
	order.UpdatedAt = time.Now().UTC()
	s.save(order)

	return order, nil
}

func (s *Service) AcceptAsync(ctx context.Context, req models.CreateOrderRequest) (models.Order, error) {
	order := s.newOrder(req)

	event := models.OrderCreatedEvent{
		EventType:   "order.created",
		PublishedAt: time.Now().UTC(),
		Order:       order,
	}

	if err := s.pub.PublishOrderCreated(ctx, event); err != nil {
		return models.Order{}, err
	}

	s.save(order)
	return order, nil
}

func (s *Service) newOrder(req models.CreateOrderRequest) models.Order {
	now := time.Now().UTC()

	return models.Order{
		OrderID:    fmt.Sprintf("ord-%06d", s.seq.Add(1)),
		CustomerID: req.CustomerID,
		Status:     models.StatusPending,
		Items:      append([]models.Item(nil), req.Items...),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func (s *Service) save(order models.Order) {
	s.orders.Store(order.OrderID, order)
}
