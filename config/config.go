package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Port          string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	JWTSecret     string
	RedisHost     string
	RedisPort     string
	Environment   string
	// Supabase specific
	SupabaseProjectRef string
	UsePooler          bool
}

var AppConfig *Config
var DB *gorm.DB

// LoadConfig loads environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Check if using Supabase pooler
	usePooler := getEnv("USE_POOLER", "true") == "true"
	supabaseRef := getEnv("SUPABASE_PROJECT_REF", "")

	config := &Config{
		Port:               getEnv("PORT", "8080"),
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnv("DB_PORT", "5432"),
		DBUser:             getEnv("DB_USER", "postgres"),
		DBPassword:         getEnv("DB_PASSWORD", ""),
		DBName:             getEnv("DB_NAME", "postgres"),
		JWTSecret:          getEnv("JWT_SECRET", "your-secret-key"),
		RedisHost:          getEnv("REDIS_HOST", "localhost"),
		RedisPort:          getEnv("REDIS_PORT", "6379"),
		Environment:        getEnv("ENVIRONMENT", "development"),
		SupabaseProjectRef: supabaseRef,
		UsePooler:          usePooler,
	}

	AppConfig = config
	return config, nil
}

// InitDB initializes database connection
func InitDB() (*gorm.DB, error) {
	// Validate required configuration
	if AppConfig.DBHost == "" || AppConfig.DBHost == "localhost" {
		log.Printf("WARNING: DB_HOST is '%s'. For production, set DB_HOST environment variable to your Supabase/Cloud SQL host", AppConfig.DBHost)
	}
	if AppConfig.DBPassword == "" {
		log.Printf("ERROR: DB_PASSWORD is empty. Database connection will fail.")
		return nil, fmt.Errorf("DB_PASSWORD environment variable is required")
	}

	// Log connection info (masked for security)
	log.Printf("Connecting to database: host=%s port=%s user=%s dbname=%s pooler=%v",
		maskHost(AppConfig.DBHost),
		AppConfig.DBPort,
		AppConfig.DBUser,
		AppConfig.DBName,
		AppConfig.UsePooler,
	)

	// Build DSN with proper settings for Supabase
	// Using connection parameters optimized for serverless environments
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=require TimeZone=Asia/Ho_Chi_Minh connect_timeout=10",
		AppConfig.DBHost,
		AppConfig.DBUser,
		AppConfig.DBPassword,
		AppConfig.DBName,
		AppConfig.DBPort,
	)

	// Add prepared statements disable for transaction pooler mode
	if AppConfig.UsePooler {
		dsn += " statement_cache_mode=describe"
	}

	log.Printf("DSN configured (password hidden)")

	var logLevel logger.LogLevel
	if AppConfig.Environment == "production" {
		logLevel = logger.Error
	} else {
		logLevel = logger.Info
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logLevel),
		PrepareStmt:            false, // Disable prepared statements for Supabase pooler
		SkipDefaultTransaction: true,  // Improve performance
	})

	if err != nil {
		log.Printf("Database connection error: %v", err)
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool for serverless environment
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("Failed to get underlying database: %v", err)
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	// Set connection pool settings (important for Cloud Run)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	// Verify connection with ping
	if err := sqlDB.Ping(); err != nil {
		log.Printf("Database ping failed: %v", err)
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	log.Printf("Database connection verified successfully")
	DB = db
	return db, nil
}

// maskHost masks host for logging, preserving domain structure
func maskHost(host string) string {
	if len(host) <= 3 {
		return "***"
	}
	if len(host) <= 15 {
		return host[:3] + "***"
	}
	return host[:8] + "***" + host[len(host)-10:]
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
