package main

import "time"

type App struct {
	products *ProductStore
	carts    CartRepository
}

type Product struct {
	ProductID    int32  `json:"product_id" binding:"required,min=1"`
	SKU          string `json:"sku" binding:"required,min=1,max=100"`
	Manufacturer string `json:"manufacturer" binding:"required,min=1,max=200"`
	CategoryID   int32  `json:"category_id" binding:"required,min=1"`
	Weight       int32  `json:"weight" binding:"min=0"`
	SomeOtherID  int32  `json:"some_other_id" binding:"required,min=1"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type ShoppingCartStatus string

const (
	CartStatusActive      ShoppingCartStatus = "ACTIVE"
	CartStatusCheckedOut  ShoppingCartStatus = "CHECKED_OUT"
)

type CreateShoppingCartRequest struct {
	CustomerID int32 `json:"customer_id" binding:"required,min=1"`
}

type CreateShoppingCartResponse struct {
	ShoppingCartID int64 `json:"shopping_cart_id"`
}

type AddCartItemRequest struct {
	ProductID int32 `json:"product_id" binding:"required,min=1"`
	Quantity  int32 `json:"quantity" binding:"required,min=1"`
}

type ShoppingCart struct {
	ShoppingCartID int64              `json:"shopping_cart_id"`
	CustomerID     int32              `json:"customer_id"`
	Status         ShoppingCartStatus `json:"status"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Items          []ShoppingCartItem `json:"items"`
}

type ShoppingCartItem struct {
	ShoppingCartID int64     `json:"shopping_cart_id"`
	ProductID      int32     `json:"product_id"`
	Quantity       int32     `json:"quantity"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ShoppingCartResponse struct {
	ShoppingCartID int64                   `json:"shopping_cart_id"`
	CustomerID     int32                   `json:"customer_id"`
	Status         ShoppingCartStatus      `json:"status"`
	UpdatedAt      time.Time               `json:"updated_at"`
	Items          []ShoppingCartItemEntry `json:"items"`
}

type ShoppingCartItemEntry struct {
	ProductID int32 `json:"product_id"`
	Quantity  int32 `json:"quantity"`
}

func NewShoppingCartResponse(cart *ShoppingCart) ShoppingCartResponse {
	items := make([]ShoppingCartItemEntry, 0, len(cart.Items))
	for _, item := range cart.Items {
		items = append(items, ShoppingCartItemEntry{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		})
	}

	return ShoppingCartResponse{
		ShoppingCartID: cart.ShoppingCartID,
		CustomerID:     cart.CustomerID,
		Status:         cart.Status,
		UpdatedAt:      cart.UpdatedAt,
		Items:          items,
	}
}
