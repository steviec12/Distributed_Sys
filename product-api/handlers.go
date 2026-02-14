package main

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func handleGetProduct(c *gin.Context) {
	//get productid from url

	idStr := c.Param("productId")

	id, err := strconv.Atoi(idStr)

	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid Input",
			Message: "invalid format of product_id",
		})
		return
	}

	if id < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid Input",
			Message: "product_id must be positive",
		})
		return
	}

	product, exits := getProduct(int32(id))
	if !exits {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "Invalid Input",
			Message: "product not found",
		})
		return
	}

	c.JSON(http.StatusOK, product)
}

func handlePostProductDetails(c *gin.Context) {
	// get product_id

	idStr := c.Param("productId")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid Input",
			Message: "invalid product id",
		})
		return
	}

	if id < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid Input",
			Message: "product_id must be positive",
		})
		return
	}
	var product Product
	if err := c.ShouldBindJSON(&product); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid Input",
			Message: err.Error(),
		})
		return
	}

	if int32(id) != product.ProductID {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "URL product ID does not match body product_id",
		})
		return
	}

	saveProduct(product)

	c.Status(http.StatusNoContent)

}
