package config

import (
	"fmt"
	"log"
	"os"

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
}

var AppConfig *Config
var DB *gorm.DB

// LoadConfig loads environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	config := &Config{
		Port:          getEnv("PORT", "8080"),
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnv("DB_PORT", "5432"),
		DBUser:        getEnv("DB_USER", "postgres"),
		DBPassword:    getEnv("DB_PASSWORD", ""),
		DBName:        getEnv("DB_NAME", "cpls_db"),
		JWTSecret:     getEnv("JWT_SECRET", "your-secret-key"),
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		Environment:   getEnv("ENVIRONMENT", "development"),
	}

	AppConfig = config
	return config, nil
}

// InitDB initializes database connection
func InitDB() (*gorm.DB, error) {
	// Log connection info (masked for security)
	log.Printf("Connecting to database: host=%s port=%s user=%s dbname=%s",
		maskHost(AppConfig.DBHost),
		AppConfig.DBPort,
		AppConfig.DBUser,
		AppConfig.DBName,
	)

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=require TimeZone=Asia/Ho_Chi_Minh",
		AppConfig.DBHost,
		AppConfig.DBUser,
		AppConfig.DBPassword,
		AppConfig.DBName,
		AppConfig.DBPort,
	)

	var logLevel logger.LogLevel
	if AppConfig.Environment == "production" {
		logLevel = logger.Error
	} else {
		logLevel = logger.Info
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})

	if err != nil {
		log.Printf("Database connection error: %v", err)
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Verify connection with ping
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("Failed to get underlying database: %v", err)
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

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
