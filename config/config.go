package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Port        string
	DBHost      string
	DBPort      string
	DBUser      string
	DBPassword  string
	DBName      string
	JWTSecret   string
	Environment string
	// Supabase connection pooler mode: "transaction" or "session"
	DBPoolMode string
}

var AppConfig *Config
var DB *gorm.DB

func LoadConfig() (*Config, error) {
	config := &Config{
		Port:        getEnv("PORT", "8080"),
		DBHost:      getEnv("DB_HOST", ""),
		DBPort:      getEnv("DB_PORT", "5432"),
		DBUser:      getEnv("DB_USER", "postgres"),
		DBPassword:  getEnv("DB_PASSWORD", ""),
		DBName:      getEnv("DB_NAME", "postgres"),
		JWTSecret:   getEnv("JWT_SECRET", "default-secret"),
		Environment: getEnv("ENVIRONMENT", "production"),
		DBPoolMode:  getEnv("DB_POOL_MODE", ""),
	}

	log.Printf("Config: PORT=%s, DB_HOST=%s, DB_USER=%s, DB_NAME=%s, ENV=%s",
		config.Port, maskStr(config.DBHost), config.DBUser, config.DBName, config.Environment)

	// Log Supabase connection info for debugging
	if strings.Contains(config.DBHost, "supabase.co") {
		log.Println("Detected Supabase connection")
		if strings.Contains(config.DBHost, "pooler.supabase.com") {
			log.Println("Using Supabase Connection Pooler (pgbouncer)")
		}
	}

	AppConfig = config
	return config, nil
}

func InitDB() (*gorm.DB, error) {
	if AppConfig.DBHost == "" {
		log.Println("ERROR: DB_HOST is empty")
		return nil, fmt.Errorf("DB_HOST is empty - please configure database connection. Set DB_HOST environment variable to your Supabase database host (e.g., db.xxxxxxxxxxxxx.supabase.co)")
	}
	if AppConfig.DBPassword == "" {
		log.Println("ERROR: DB_PASSWORD is empty")
		return nil, fmt.Errorf("DB_PASSWORD is empty - please configure database password. Set DB_PASSWORD environment variable in Cloud Run or Cloud Build")
	}

	// URL encode password to handle special characters
	encodedPass := url.QueryEscape(AppConfig.DBPassword)

	// Build DSN with appropriate settings for Supabase
	// Use direct connection for transactions, pooler for session mode
	sslMode := "require"
	connectTimeout := 10

	// For Supabase pooler (pgbouncer), we need different settings
	isPooler := strings.Contains(AppConfig.DBHost, "pooler.supabase.com")
	if isPooler {
		log.Println("Configuring for Supabase Connection Pooler")
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s&connect_timeout=%d",
		AppConfig.DBUser,
		encodedPass,
		AppConfig.DBHost,
		AppConfig.DBPort,
		AppConfig.DBName,
		sslMode,
		connectTimeout,
	)

	log.Printf("Connecting to database at %s...", maskStr(AppConfig.DBHost))

	gormConfig := &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: false,
		// Skip default transaction for better performance
		SkipDefaultTransaction: true,
	}

	// For Supabase pooler (pgbouncer), disable prepared statements
	if isPooler {
		gormConfig.PrepareStmt = false
		log.Println("Prepared statements disabled for pgbouncer compatibility")
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		log.Printf("ERROR: Failed to connect to database: %v", err)
		return nil, fmt.Errorf("gorm.Open failed: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("ERROR: Failed to get underlying DB: %v", err)
		return nil, fmt.Errorf("db.DB() failed: %w", err)
	}

	// Optimized connection pool settings for cloud environments (Cloud Run + Supabase)
	// Cloud Run scales to zero, so we need smaller pool settings
	sqlDB.SetMaxIdleConns(2)                   // Fewer idle connections for serverless
	sqlDB.SetMaxOpenConns(5)                   // Limit concurrent connections for pooler
	sqlDB.SetConnMaxLifetime(10 * time.Minute) // Shorter lifetime for cloud environments
	sqlDB.SetConnMaxIdleTime(3 * time.Minute)  // Close idle connections quickly

	if err := sqlDB.Ping(); err != nil {
		log.Printf("ERROR: Database ping failed: %v", err)
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	log.Println("Database connected successfully!")
	DB = db
	return db, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func maskStr(s string) string {
	if len(s) < 8 {
		return "***"
	}
	return s[:4] + "***"
}
