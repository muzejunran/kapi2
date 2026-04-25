package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ServerPort     string
	AuthPort       string
	LLMServicePort string
	SessionPort    string
	MonitoringPort string
	JWTSecret      string
	RedisAddr      string
	MySQLDSN       string
	LLMApiKey      string
	LLMEndpoint    string
	RateLimit      int
	TokenBudget    int
	SkillTimeout   time.Duration
	SkillServerURL string // skill-server 地址
	ModelName      string
	MaxTokens      int
	Temperature    float64
}

func Load() (*Config, error) {
	return &Config{
		ServerPort:     getEnv("SERVER_PORT", "8080"),
		AuthPort:       getEnv("AUTH_PORT", "8081"),
		LLMServicePort: getEnv("LLM_PORT", "8082"),
		SessionPort:    getEnv("SESSION_PORT", "8083"),
		MonitoringPort: getEnv("MONITORING_PORT", "9090"),
		JWTSecret:      getEnv("JWT_SECRET", "qwert"),
		RedisAddr:      getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		MySQLDSN:       getEnv("MYSQL_DSN", "root:11111111@tcp(127.0.0.1:3306)/kapi?charset=utf8mb4&parseTime=true"),
		LLMApiKey:      getEnv("LLM_API_KEY", "test"),
		LLMEndpoint:    getEnv("LLM_ENDPOINT", "https://open/xxxx/completions"),
		RateLimit:      getEnvInt("RATE_LIMIT", 100),
		TokenBudget:    getEnvInt("TOKEN_BUDGET", 1500),
		SkillTimeout:   getEnvDuration("SKILL_TIMEOUT", 60*time.Second),
		SkillServerURL: getEnv("SKILL_SERVER_URL", "http://localhost:8090"),
		ModelName:      getEnv("MODEL_NAME", "test"),
		MaxTokens:      getEnvInt("MAX_TOKENS", 10000),
		Temperature:    getEnvFloat("TEMPERATURE", 0.7),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}


func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := parseInt(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := parseFloat(value); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
