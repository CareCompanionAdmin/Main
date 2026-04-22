package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App         AppConfig
	Database    DatabaseConfig
	Redis       RedisConfig
	JWT         JWTConfig
	Correlation CorrelationConfig
	Storage     StorageConfig
	SMTP        SMTPConfig
	FCM         FCMConfig
	Claude      ClaudeConfig
}

type StorageConfig struct {
	UploadDir   string
	MaxFileSize int64
}

type AppConfig struct {
	Env   string
	Debug bool
	Port  string
	Host  string
	URL   string
}

type DatabaseConfig struct {
	Host            string
	Port            string
	Name            string
	User            string
	Password        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type JWTConfig struct {
	Secret        string
	AccessExpiry  time.Duration
	RefreshExpiry time.Duration
}

type CorrelationConfig struct {
	MinDataPoints       int
	ConfidenceThreshold float64
	BatchSize           int
}

type SMTPConfig struct {
	Enabled     bool
	Host        string
	Port        string
	Username    string
	Password    string
	FromAddress string
	FromName    string
}

type FCMConfig struct {
	ServerKey string
}

type ClaudeConfig struct {
	APIKey         string
	Model          string
	MaxTokens      int
	DailyRunHour   int
	MaxInsights    int
	LookbackDays   int
	Enabled        bool
}

func Load() (*Config, error) {
	godotenv.Load()

	cfg := &Config{
		App: AppConfig{
			Env:   getEnv("APP_ENV", "development"),
			Debug: getEnvBool("APP_DEBUG", true),
			Port:  getEnv("APP_PORT", "8080"),
			Host:  getEnv("APP_HOST", "0.0.0.0"),
			URL:   getEnv("APP_URL", "http://localhost:8080"),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "172.28.0.10"),
			Port:            getEnv("DB_PORT", "5432"),
			Name:            getEnv("DB_NAME", "carecompanion"),
			User:            getEnv("DB_USER", "carecomp_app"),
			Password:        getEnv("DB_PASSWORD", ""),
			SSLMode:         getEnv("DB_SSLMODE", "disable"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "172.28.0.30"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret:        getEnv("JWT_SECRET", ""),
			AccessExpiry:  getEnvDuration("JWT_ACCESS_EXPIRY", 15*time.Minute),
			RefreshExpiry: getEnvDuration("JWT_REFRESH_EXPIRY", 7*24*time.Hour),
		},
		Correlation: CorrelationConfig{
			MinDataPoints:       getEnvInt("CORRELATION_MIN_DATA_POINTS", 7),
			ConfidenceThreshold: getEnvFloat("CORRELATION_CONFIDENCE_THRESHOLD", 0.6),
			BatchSize:           getEnvInt("CORRELATION_BATCH_SIZE", 100),
		},
		Storage: StorageConfig{
			UploadDir:   getEnv("STORAGE_UPLOAD_DIR", "./uploads"),
			MaxFileSize: int64(getEnvInt("STORAGE_MAX_FILE_SIZE", 10*1024*1024)), // 10MB default
		},
		FCM: FCMConfig{
			ServerKey: getEnv("FCM_SERVER_KEY", ""),
		},
		SMTP: SMTPConfig{
			Enabled:     getEnvBool("SMTP_ENABLED", false),
			Host:        getEnv("SMTP_HOST", "smtp.office365.com"),
			Port:        getEnv("SMTP_PORT", "587"),
			Username:    getEnv("SMTP_USERNAME", ""),
			Password:    getEnv("SMTP_PASSWORD", ""),
			FromAddress: getEnv("SMTP_FROM_ADDRESS", "notifications@mycarecompanion.net"),
			FromName:    getEnv("SMTP_FROM_NAME", "MyCareCompanion"),
		},
		Claude: ClaudeConfig{
			APIKey:       getEnv("CLAUDE_API_KEY", ""),
			Model:        getEnv("CLAUDE_MODEL", "claude-sonnet-4-5-20241022"),
			MaxTokens:    getEnvInt("CLAUDE_MAX_TOKENS", 4096),
			DailyRunHour: getEnvInt("CLAUDE_DAILY_RUN_HOUR", 6),
			MaxInsights:  getEnvInt("CLAUDE_MAX_INSIGHTS", 5),
			LookbackDays: getEnvInt("CLAUDE_LOOKBACK_DAYS", 7),
			Enabled:      getEnvBool("CLAUDE_ENABLED", false),
		},
	}

	return cfg, nil
}

func (c *DatabaseConfig) DSN() string {
	return "host=" + c.Host +
		" port=" + c.Port +
		" user=" + c.User +
		" password=" + c.Password +
		" dbname=" + c.Name +
		" sslmode=" + c.SSLMode
}

func (c *RedisConfig) Addr() string {
	return c.Host + ":" + c.Port
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
