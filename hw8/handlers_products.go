package main

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (a *App) handleGetProduct(c *gin.Context) {
	productID, ok := parsePositiveInt32PathParam(c, "productId")
	if !ok {
		return
	}

	product, exists := a.products.Get(productID)
	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "product not found",
		})
		return
	}

	c.JSON(http.StatusOK, product)
}

func (a *App) handlePostProductDetails(c *gin.Context) {
	productID, ok := parsePositiveInt32PathParam(c, "productId")
	if !ok {
		return
	}

	var product Product
	if err := c.ShouldBindJSON(&product); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: err.Error(),
		})
		return
	}

	if product.ProductID != productID {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "URL product ID does not match body product_id",
		})
		return
	}

	a.products.Save(product)
	c.Status(http.StatusNoContent)
}

func parsePositiveInt32PathParam(c *gin.Context, name string) (int32, bool) {
	value := c.Param(name)
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: name + " must be a positive integer",
		})
		return 0, false
	}

	return int32(id), true
}

func parsePositiveInt64PathParam(c *gin.Context, name string) (int64, bool) {
	value := c.Param(name)
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: name + " must be a positive integer",
		})
		return 0, false
	}

	return id, true
}
