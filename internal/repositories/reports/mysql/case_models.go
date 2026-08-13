package mysql

import "database/sql"

type CaseRow struct {
	Code       string
	Title      string
	Status     string
	CustomerID sql.NullInt64
	SiteID     sql.NullInt64
}

type SearchRow struct {
	ID     int64
	Code   string
	Title  string
	Status string
}
