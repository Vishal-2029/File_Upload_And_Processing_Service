package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr string

	JWTSecret      string
	JWTExpiryHours int

	PostgresDSN string

	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioUseSSL    bool

	SMTPHost string
	SMTPPort int
	SMTPFrom string

	TmpDir         string
	ProcessedDir   string
	MaxFileSizeMB  int64

	WorkerCount  int
	JobQueueSize int

	RateLimitRPS   float64
	RateLimitBurst int
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		HTTPAddr: getEnv("HTTP_ADDR", ":3000"),

		JWTSecret:      getEnv("JWT_SECRET", "change-me"),
		JWTExpiryHours: getEnvInt("JWT_EXPIRY_HOURS", 72),

		PostgresDSN: getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/fileservice?sslmode=disable"),

		MinioEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinioAccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinioBucket:    getEnv("MINIO_BUCKET", "uploads"),
		MinioUseSSL:    getEnvBool("MINIO_USE_SSL", false),

		SMTPHost: getEnv("SMTP_HOST", "localhost"),
		SMTPPort: getEnvInt("SMTP_PORT", 1025),
		SMTPFrom: getEnv("SMTP_FROM", "noreply@fileservice.dev"),

		TmpDir:        getEnv("TMP_DIR", "/tmp/fileservice"),
		ProcessedDir:  getEnv("PROCESSED_DIR", "/tmp/fileservice/processed"),
		MaxFileSizeMB: getEnvInt64("MAX_FILE_SIZE_MB", 50),

		WorkerCount:  getEnvInt("WORKER_COUNT", 10),
		JobQueueSize: getEnvInt("JOB_QUEUE_SIZE", 1000),

		RateLimitRPS:   getEnvFloat("RATE_LIMIT_RPS", 1.67),
		RateLimitBurst: getEnvInt("RATE_LIMIT_BURST", 10),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
