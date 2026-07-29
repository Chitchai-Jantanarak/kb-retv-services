package config

type MySQL struct {
	Enabled      bool
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
}

type Redis struct {
	URL string
}

type Qdrant struct {
	Enabled          bool
	URL              string
	APIKey           string
	CollectionPrefix string
}

type Memgraph struct {
	Enabled  bool
	URI      string
	URL      string
	Username string
	Password string
}
