package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL          string
	HTTPAddr             string
	WorkerHTTPAddr       string
	WorkerPollInterval   time.Duration
	BotToken             string
	APIBaseURL           string
	OpenAIAPIKey         string
	OpenAIModel          string
	EmbeddingModel       string
	EmbeddingDimensions  int
	AIMock               bool
	RetrievalMode        string
	RetrievalTopK        int
	DemoMode             bool
	DemoFictionalPrefix  string
	DemoSeedFile         string
	LogLevel             string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		DatabaseURL:         env("DATABASE_URL", "postgres://dating:dating@localhost:5432/dating_bot?sslmode=disable"),
		HTTPAddr:            env("HTTP_ADDR", ":8080"),
		WorkerHTTPAddr:      env("WORKER_HTTP_ADDR", ":8081"),
		WorkerPollInterval:  envDuration("WORKER_POLL_INTERVAL", 2*time.Second),
		BotToken:            env("BOT_TOKEN", ""),
		APIBaseURL:          env("API_BASE_URL", "http://localhost:8080/v1"),
		OpenAIAPIKey:        env("OPENAI_API_KEY", ""),
		OpenAIModel:         env("OPENAI_MODEL", "gpt-4o-mini"),
		EmbeddingModel:      env("EMBEDDING_MODEL", "text-embedding-3-small"),
		EmbeddingDimensions: envInt("EMBEDDING_DIMENSIONS", 1536),
		AIMock:              envBool("AI_MOCK", true),
		RetrievalMode:       env("RETRIEVAL_MODE", "ann"),
		RetrievalTopK:       envInt("RETRIEVAL_TOP_K", 50),
		DemoMode:            envBool("DEMO_MODE", true),
		DemoFictionalPrefix: env("DEMO_FICTIONAL_PREFIX", "вымышленный_"),
		DemoSeedFile:        env("DEMO_SEED_FILE", "seed/fictional_profiles.json"),
		LogLevel:            env("LOG_LEVEL", "info"),
	}

	if !cfg.AIMock && cfg.OpenAIAPIKey == "" {
		return cfg, fmt.Errorf("AI_MOCK=false but OPENAI_API_KEY is empty")
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
