package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App              AppConfig
	Database         DatabaseConfig
	Redis            RedisConfig
	JWT              JWTConfig
	Correlation      CorrelationConfig
	Storage          StorageConfig
	SMTP             SMTPConfig
	FCM              FCMConfig
	Claude           ClaudeConfig
	AppStoreConnect  AppStoreConnectConfig
	Stripe           StripeConfig
}

// StripeConfig holds the test/live API keys + webhook signing secret.
// Loaded from /home/carecomp/secrets/stripe.env (mode 600) on dev via the
// systemd EnvironmentFile directive; from AWS Secrets Manager on prod.
// SecretKey is the only field required to call the Stripe API; the others
// are needed for the front-end (PublishableKey) and the webhook receiver
// (WebhookSecret) once those land.
type StripeConfig struct {
	SecretKey      string // sk_test_... or sk_live_... (server-side; never exposed to client)
	PublishableKey string // pk_test_... or pk_live_... (safe to embed in HTML)
	WebhookSecret  string // whsec_... — verifies signatures on POST /webhooks/stripe
}

// Enabled returns true when the Stripe SDK can be invoked. SecretKey alone
// is enough for Checkout sessions and product/price creation; the webhook
// secret is only required when handling webhook callbacks.
func (s StripeConfig) Enabled() bool {
	return s.SecretKey != ""
}

// AppStoreConnectConfig holds the team-level API key Apple issues from
// App Store Connect → Users and Access → Integrations → Team Keys.
// All four fields must be set for the beta-invite auto-add flow to work;
// when any are blank the BetaService falls back to manual-add (logs only).
type AppStoreConnectConfig struct {
	IssuerID      string
	KeyID         string
	KeyPath       string // absolute path to AuthKey_*.p8 (mode 600, outside repo)
	BetaGroupName string // e.g. "External Beta Testers"
}

type StorageConfig struct {
	UploadDir   string
	MaxFileSize int64
	// Ticket attachments — separate ceiling so reports / other uploads
	// can keep their own MaxFileSize.
	AttachmentMaxBytes   int64
	AttachmentMaxPerTkt  int
	// S3 driver. If S3Bucket is empty the localfs driver is used.
	S3Bucket       string
	S3Region       string
	S3Prefix       string // ticket attachments
	ReportS3Prefix string // reports
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

	// SupportDSN, when non-empty, overrides where the support-ticket repos
	// (admin / user-support / ticket-attachment) connect for support_tickets,
	// ticket_messages, and ticket_attachments. The main DB is still used for
	// every other table including users-lookup-for-denorm. When empty, the
	// main DB is used for support too. Set on dev to share prod's tickets.
	SupportDSN string

	// SessionsProdDSN, when non-empty, opens a SECOND read-only connection to
	// the prod sessions table so the dev Live Sessions admin page can show
	// prod sessions alongside dev. Empty in prod (cross-env display is a
	// dev-side affordance only).
	SessionsProdDSN string

	// AdminMirrorDSN, when non-empty, opens a connection to the OTHER env's
	// database for bidirectional admin_users replication. Every admin CRUD
	// operation dual-writes to both the local DB and this mirror; failure to
	// commit on either side rolls back the local write. Set on BOTH envs:
	// dev points at prod RDS, prod points at the dev docker-postgres on the
	// admin EC2 (via private-IP SG ingress). When unset, replication is
	// disabled and admin CRUD is local-only.
	AdminMirrorDSN string
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
	ServerKey              string
	ServiceAccountKeyFile  string
}

type ClaudeConfig struct {
	APIKey         string
	Model          string
	MaxTokens      int
	DailyRunHour   int
	MaxInsights    int
	LookbackDays   int
	Enabled        bool

	// NarrativeOptInAvailable gates whether the AI Narrative Analysis
	// opt-in toggle is shown in the user-facing Settings page and
	// whether the consent service will honor a "true" consent value.
	// Stays false through Phases 3-4 even as the consent code ships;
	// flipped to true in Phase 5 once AWS Bedrock + BAA are in place.
	// See docs/superpowers/specs/2026-05-11-ai-phi-stripping-and-internal-expansion.md
	NarrativeOptInAvailable bool
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
			SupportDSN:      getEnv("SUPPORT_DB_DSN", ""),
			SessionsProdDSN: getEnv("SESSIONS_PROD_DB_DSN", ""),
			AdminMirrorDSN:  getEnv("ADMIN_MIRROR_DB_DSN", ""),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "172.28.0.30"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", ""),
			// Access token TTL: long-ish so the silent-refresh flow in
			// session_guard.js rarely has to fire. Refresh tokens (7d)
			// keep "remember me" behavior. Bumped from 15m → 8h on
			// 2026-05-07 to fix Joe Steinmetz's mid-input logout.
			AccessExpiry:  getEnvDuration("JWT_ACCESS_EXPIRY", 8*time.Hour),
			RefreshExpiry: getEnvDuration("JWT_REFRESH_EXPIRY", 7*24*time.Hour),
		},
		Correlation: CorrelationConfig{
			MinDataPoints:       getEnvInt("CORRELATION_MIN_DATA_POINTS", 7),
			ConfidenceThreshold: getEnvFloat("CORRELATION_CONFIDENCE_THRESHOLD", 0.6),
			BatchSize:           getEnvInt("CORRELATION_BATCH_SIZE", 100),
		},
		Storage: StorageConfig{
			UploadDir:           getEnv("STORAGE_UPLOAD_DIR", "./uploads"),
			MaxFileSize:         int64(getEnvInt("STORAGE_MAX_FILE_SIZE", 10*1024*1024)), // 10MB default
			AttachmentMaxBytes:  int64(getEnvInt("ATTACHMENT_MAX_BYTES", 25*1024*1024)),  // 25MB per ticket attachment
			AttachmentMaxPerTkt: getEnvInt("ATTACHMENT_MAX_PER_TICKET", 5),
			S3Bucket:            getEnv("ATTACHMENT_S3_BUCKET", ""),
			S3Region:            getEnv("ATTACHMENT_S3_REGION", "us-east-1"),
			S3Prefix:            getEnv("ATTACHMENT_S3_PREFIX", "ticket-attachments/"),
			ReportS3Prefix:      getEnv("REPORT_S3_PREFIX", "reports/"),
		},
		FCM: FCMConfig{
			ServerKey:             getEnv("FCM_SERVER_KEY", ""),
			ServiceAccountKeyFile: getEnv("FIREBASE_SERVICE_ACCOUNT_KEY", ""),
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
		AppStoreConnect: AppStoreConnectConfig{
			IssuerID:      getEnv("ASC_ISSUER_ID", ""),
			KeyID:         getEnv("ASC_KEY_ID", ""),
			KeyPath:       getEnv("ASC_KEY_PATH", ""),
			BetaGroupName: getEnv("ASC_BETA_GROUP_NAME", "External Beta Testers"),
		},
		Claude: ClaudeConfig{
			APIKey:                  getEnv("CLAUDE_API_KEY", ""),
			Model:                   getEnv("CLAUDE_MODEL", "claude-sonnet-4-5-20241022"),
			MaxTokens:               getEnvInt("CLAUDE_MAX_TOKENS", 4096),
			DailyRunHour:            getEnvInt("CLAUDE_DAILY_RUN_HOUR", 6),
			MaxInsights:             getEnvInt("CLAUDE_MAX_INSIGHTS", 5),
			LookbackDays:            getEnvInt("CLAUDE_LOOKBACK_DAYS", 7),
			Enabled:                 getEnvBool("CLAUDE_ENABLED", false),
			NarrativeOptInAvailable: getEnvBool("AI_NARRATIVE_OPT_IN_AVAILABLE", false),
		},
		Stripe: StripeConfig{
			SecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
			PublishableKey: getEnv("STRIPE_PUBLISHABLE_KEY", ""),
			WebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
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
