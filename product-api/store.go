package main

import "sync"

// thread safe
var products sync.Map

func getProduct(id int32) (Product, bool) {
	value, exists := products.Load(id)
	if !exists {
		return Product{}, false
	}

	return value.(Product), true
}

func saveProduct(product Product) {
	products.Store(product.ProductID, product)
}
