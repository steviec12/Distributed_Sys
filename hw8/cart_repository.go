package main

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrCartClosed     = errors.New("cart closed")
	ErrNotImplemented = errors.New("not implemented")
)

type CartRepository interface {
	CreateCart(ctx context.Context, customerID int32) (*ShoppingCart, error)
	GetCart(ctx context.Context, cartID int64) (*ShoppingCart, error)
	AddItem(ctx context.Context, cartID int64, productID int32, quantity int32) error
	Close() error
}

func NewCartRepository(cfg Config) (CartRepository, error) {
	switch cfg.DataBackend {
	case "mysql":
		return NewMySQLCartRepository(cfg)
	case "dynamodb":
		return NewDynamoDBCartRepository(cfg)
	default:
		return nil, fmt.Errorf("unsupported DATA_BACKEND %q", cfg.DataBackend)
	}
}
