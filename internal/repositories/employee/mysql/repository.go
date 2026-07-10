package mysql

import (
	"github.com/my/app/internal/infra/tenant"
)

type CaseRow struct {
	Code   string
	Title  string
	Status string
}

type WorkloadRow struct {
	Name   string
	Open   int64
	Oldest string
}

type Repository struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Repository {
	return &Repository{db: db}
}
