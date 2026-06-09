package kb

type Article struct {
	ID        string
	CompanyID int64
	Title     string
	Summary   string
	Content   string
}

type Chunk struct {
	ID        string
	ArticleID string
	CompanyID int64
	Title     string
	Content   string
}

type ChunkQuery struct {
	IDs       []string
	CompanyID int64
}

type ChunkContext struct {
	ChildID   string
	ParentID  string
	ArticleID string
	CompanyID int64
	Title     string
	Content   string
}
