package mysql

type CaseRow struct {
	Code   string
	Title  string
	Status string
}

type SearchRow struct {
	ID     int64
	Code   string
	Title  string
	Status string
}
