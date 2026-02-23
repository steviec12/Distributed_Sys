package main

// Product represents a single item in our store.
// Fields: ID, Name (searchable), Category (searchable), Description, Brand.
type Product struct {
	ProductID   int32  `json:"product_id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

// SearchResponse is what the /products/search endpoint returns.
// Products: array of matches (max 20), TotalFound: count of all matches, SearchTime: optional duration.
type SearchResponse struct {
	Products   []Product `json:"products"`
	TotalFound int       `json:"total_found"`
	SearchTime string    `json:"search_time,omitempty"`
}

// ErrorResponse is the standard error format (same pattern as HW5).
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
