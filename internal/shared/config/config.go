package config

type Config struct {
	App       App
	Server    Server
	MySQL     MySQL
	Redis     Redis
	Qdrant    Qdrant
	Memgraph  Memgraph
	LLM       LLM
	APIKeys   APIKeys
	Logger    Logger
	Swagger   Swagger
	Laravel   Laravel
	Line      Line
	Budget    Budget
	Chat      Chat
	Embedding Embedding
	Reply     Reply
}

func Load() (Config, error) {
	return LoadFrom("config.yaml")
}
