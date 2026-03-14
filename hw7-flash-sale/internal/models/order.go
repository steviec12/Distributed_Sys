package models

import "time"

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
)

type Item struct {
	ProductID int `json:"product_id" binding:"required,min=1"`
	Quantity  int `json:"quantity" binding:"required,min=1"`
}

type CreateOrderRequest struct {
	CustomerID int    `json:"customer_id" binding:"required,min=1"`
	Items      []Item `json:"items" binding:"required,min=1,dive"`
}

type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"`
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type OrderCreatedEvent struct {
	EventType   string    `json:"event_type"`
	PublishedAt time.Time `json:"published_at"`
	Order       Order     `json:"order"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
