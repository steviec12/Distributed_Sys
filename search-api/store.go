package main

import (
	"fmt"
	"strings"
	"sync"
)

var products sync.Map

var brands = []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta", "Eta", "Theta"}
var categories = []string{"Electronics", "Books", "Home", "Garden", "Sports", "Toys", "Food", "Clothing"}
var descriptions = []string{
	"A high-quality product for everyday use",
	"Premium grade item with warranty",
	"Budget-friendly option with great reviews",
	"Professional grade equipment",
	"Best seller in its category",
	"New arrival with innovative features",
	"Classic design with modern functionality",
	"Eco-friendly and sustainable choice",
}

const totalProducts = 100_000

// generateProducts creates 100,000 products and stores them in the sync.Map.
func generateProducts() {
	for i := 1; i <= totalProducts; i++ {
		brand := brands[i%len(brands)]
		p := Product{
			ProductID:   int32(i),
			Name:        fmt.Sprintf("Product %s %d", brand, i),
			Category:    categories[i%len(categories)],
			Description: descriptions[i%len(descriptions)],
			Brand:       brand,
		}
		products.Store(int32(i), p)
	}
}

const maxCheck = 100
const maxResults = 20

// searchProducts checks exactly 100 products and returns up to 20 case-insensitive matches.
func searchProducts(query string) ([]Product, int) {
	var results []Product
	checked := 0
	found := 0
	lowerQuery := strings.ToLower(query)

	products.Range(func(key, value any) bool {
		checked++
		p := value.(Product)

		lowerName := strings.ToLower(p.Name)
		lowerCat := strings.ToLower(p.Category)

		if strings.Contains(lowerName, lowerQuery) || strings.Contains(lowerCat, lowerQuery) {
			found++
			if len(results) < maxResults {
				results = append(results, p)
			}
		}

		return checked < maxCheck
	})

	return results, found
}
