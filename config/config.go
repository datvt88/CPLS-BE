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
	Port        string
	DBHost      string
	DBPort      string
	DBUser      string
	DBPassword  string
	DBName      string
	JWTSecret   string
	RedisHost   string
	RedisPort   string
	Environment string
}

var AppConfig *Config
var DB *gorm.DB

// LoadConfig loads environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists (ignored in production)
	_ = godotenv.Load()

	config := &Config{
		Port:        getEnv("PORT", "8080"),
		DBHost:      getEnv("DB_HOST", ""),
		DBPort:      getEnv("DB_PORT", "5432"),
		DBUser:      getEnv("DB_USER", "postgres"),
		DBPassword:  getEnv("DB_PASSWORD", ""),
		DBName:      getEnv("DB_NAME", "postgres"),
		JWTSecret:   getEnv("JWT_SECRET", ""),
		RedisHost:   getEnv("REDIS_HOST", ""),
		RedisPort:   getEnv("REDIS_PORT", "6379"),
		Environment: getEnv("ENVIRONMENT", "development"),
	}

	// Log loaded config (masked)
	log.Printf("Config loaded: ENV=%s, DB_HOST=%s, DB_PORT=%s, DB_USER=%s, DB_NAME=%s",
		config.Environment,
		maskString(config.DBHost),
		config.DBPort,
		config.DBUser,
		config.DBName,
	)

	AppConfig = config
	return config, nil
}

// InitDB initializes database connection
func InitDB() (*gorm.DB, error) {
	// Validate required config
	if AppConfig.DBHost == "" {
		return nil, fmt.Errorf("DB_HOST is required")
	}
	if AppConfig.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD is required")
	}

	log.Printf("Connecting to database...")

	// Standard PostgreSQL DSN for Supabase
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		AppConfig.DBHost,
		AppConfig.DBPort,
		AppConfig.DBUser,
		AppConfig.DBPassword,
		AppConfig.DBName,
	)

	// GORM config optimized for serverless
	gormConfig := &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Error),
		PrepareStmt:            false,
		SkipDefaultTransaction: true,
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		log.Printf("Database connection failed: %v", err)
		return nil, err
	}

	// Configure connection pool for Cloud Run
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		log.Printf("Database ping failed: %v", err)
		return nil, err
	}

	log.Println("Database connected successfully!")
	DB = db
	return db, nil
}

// maskString masks sensitive strings for logging
func maskString(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

// getEnv gets environment variable or default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
