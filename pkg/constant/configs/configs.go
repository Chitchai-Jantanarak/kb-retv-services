package configs

import (
	"os"
	"strconv"
)

var (
	SERVER_PORT = getEnv("SERVER_PORT", "8080")

	DATABASE_WEBSTORE_IP       = getEnv("DATABASE_WEBSTORE_IP", "localhost")
	DATABASE_WEBSTORE_PORT     = getEnv("DATABASE_WEBSTORE_PORT", "5432")
	DATABASE_WEBSTORE_USER     = getEnv("DATABASE_WEBSTORE_USER", "user")
	DATABASE_WEBSTORE_PASSWORD = getEnv("DATABASE_WEBSTORE_PASSWORD", "password")
	DATABASE_WEBSTORE_NAME     = getEnv("DATABASE_WEBSTORE_NAME", "webstore")

	REDIS_HOST     = getEnv("REDIS_HOST", "localhost")
	REDIS_PORT     = getEnv("REDIS_PORT", "6379")
	REDIS_PASSWORD = getEnv("REDIS_PASSWORD", "")
	REDIS_DB       = getEnvInt("REDIS_DB", 0)

	AES_KEY = getEnv("AES_KEY", "1234567890123456")

	X_KEY_API     = getEnv("X_KEY_API", "default_api_key")
	API_PANEL_URL = getEnv("API_PANEL_URL", "http://localhost:5050")
)

func getEnv(name, value string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return value
}

func getEnvInt(name string, value int) int {
	if v := os.Getenv(name); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return value
}
