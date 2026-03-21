package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *App) handlePostShoppingCart(c *gin.Context) {
	var request CreateShoppingCartRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: err.Error(),
		})
		return
	}

	cart, err := a.carts.CreateCart(c.Request.Context(), request.CustomerID)
	if err != nil {
		log.Printf("create cart failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_SERVER_ERROR",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, CreateShoppingCartResponse{
		ShoppingCartID: cart.ShoppingCartID,
	})
}

func (a *App) handleGetShoppingCart(c *gin.Context) {
	cartID, ok := parsePositiveInt64PathParam(c, "shoppingCartId")
	if !ok {
		return
	}

	cart, err := a.carts.GetCart(c.Request.Context(), cartID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "NOT_FOUND",
				Message: "shopping cart not found",
			})
			return
		}

		log.Printf("get cart %d failed: %v", cartID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_SERVER_ERROR",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, NewShoppingCartResponse(cart))
}

func (a *App) handlePostShoppingCartItems(c *gin.Context) {
	cartID, ok := parsePositiveInt64PathParam(c, "shoppingCartId")
	if !ok {
		return
	}

	var request AddCartItemRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: err.Error(),
		})
		return
	}

	if _, exists := a.products.Get(request.ProductID); !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "product not found",
		})
		return
	}

	err := a.carts.AddItem(c.Request.Context(), cartID, request.ProductID, request.Quantity)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "NOT_FOUND",
				Message: "shopping cart not found",
			})
		case errors.Is(err, ErrCartClosed):
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "INVALID_INPUT",
				Message: "shopping cart is not active",
			})
		default:
			log.Printf("add item to cart %d failed: %v", cartID, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "INTERNAL_SERVER_ERROR",
				Message: err.Error(),
			})
		}
		return
	}

	c.Status(http.StatusNoContent)
}
