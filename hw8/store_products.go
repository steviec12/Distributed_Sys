package main

import "sync"

type ProductStore struct {
	products sync.Map
}

func NewProductStore() *ProductStore {
	return &ProductStore{}
}

func (s *ProductStore) Get(id int32) (Product, bool) {
	value, exists := s.products.Load(id)
	if !exists {
		return Product{}, false
	}

	product, ok := value.(Product)
	if !ok {
		return Product{}, false
	}

	return product, true
}

func (s *ProductStore) Save(product Product) {
	s.products.Store(product.ProductID, product)
}
