package mysql

import (
	"github.com/my/app/internal/infra/tenant"
)

type Profile struct {
	Company string
	Status  string
	Address string
	Website string
	Found   bool
}

type Repository struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Repository {
	return &Repository{db: db}
}
