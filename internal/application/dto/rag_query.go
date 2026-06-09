package dto

type RAGQueryRequest struct {
	Query    string            `json:"query" validate:"required"`
	Limit    int               `json:"limit,omitempty" validate:"omitempty,gte=1,lte=50"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (r *RAGQueryRequest) Validate() error {
	r.Normalize()
	if r.Limit == 0 {
		r.Limit = 8
	}
	if r.Limit > 50 {
		r.Limit = 50
	}
	return validateStruct(r)
}

func (r *RAGQueryRequest) Normalize() {
	r.Query = normalizeText(r.Query)
}
