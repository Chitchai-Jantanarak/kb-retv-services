package retrieval

// Query carries company-scoped retrieval parameters.
type Query struct {
	CompanyID int64
	Text      string
	Limit     int
}
