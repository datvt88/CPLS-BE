package config

import (
	"fmt"
	"log"
	"os"
	"strings"

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
	DBSSLMode     string
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
		DBSSLMode:     getEnv("DB_SSLMODE", "require"),
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
	// Log database configuration (without password)
	log.Printf("Database config: host=%s, port=%s, user=%s, dbname=%s, sslmode=%s",
		AppConfig.DBHost, AppConfig.DBPort, AppConfig.DBUser, AppConfig.DBName, AppConfig.DBSSLMode)

	// Check for required configuration in production
	if AppConfig.Environment == "production" {
		if AppConfig.DBHost == "" || AppConfig.DBHost == "localhost" {
			log.Println("Warning: DB_HOST is not configured or using default 'localhost'")
		}
		if AppConfig.DBPassword == "" {
			log.Println("Warning: DB_PASSWORD is not configured")
		}
	}

	var dsn string

	// Check if DB_HOST is a Cloud SQL Unix socket path (starts with /)
	if strings.HasPrefix(AppConfig.DBHost, "/") {
		// Cloud SQL Unix socket connection (e.g., /cloudsql/project:region:instance)
		dsn = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s TimeZone=Asia/Ho_Chi_Minh",
			AppConfig.DBHost,
			AppConfig.DBUser,
			AppConfig.DBPassword,
			AppConfig.DBName,
		)
		log.Println("Using Cloud SQL Unix socket connection")
	} else {
		// Standard TCP connection (Supabase, Cloud SQL with IP, etc.)
		dsn = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Ho_Chi_Minh",
			AppConfig.DBHost,
			AppConfig.DBUser,
			AppConfig.DBPassword,
			AppConfig.DBName,
			AppConfig.DBPort,
			AppConfig.DBSSLMode,
		)
		log.Println("Using TCP connection with SSL mode:", AppConfig.DBSSLMode)
	}

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
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	DB = db
	return db, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
