package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
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
	}

	log.Printf("Config: PORT=%s, DB_HOST=%s, DB_USER=%s, DB_NAME=%s",
		config.Port, maskStr(config.DBHost), config.DBUser, config.DBName)

	AppConfig = config
	return config, nil
}

func InitDB() (*gorm.DB, error) {
	if AppConfig.DBHost == "" {
		return nil, fmt.Errorf("DB_HOST is empty - please configure database connection")
	}
	if AppConfig.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD is empty - please configure database password")
	}

	// URL encode password to handle special characters
	encodedPass := url.QueryEscape(AppConfig.DBPassword)

	// Add connection timeout parameters for faster failure detection
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=require&connect_timeout=10",
		AppConfig.DBUser,
		encodedPass,
		AppConfig.DBHost,
		AppConfig.DBPort,
		AppConfig.DBName,
	)

	log.Println("Connecting to database...")

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: false,
		// Skip default transaction for better performance
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm.Open failed: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("db.DB() failed: %w", err)
	}

	// Optimized connection pool settings for cloud environments
	sqlDB.SetMaxIdleConns(5)                   // Increase idle connections for faster reuse
	sqlDB.SetMaxOpenConns(10)                  // Allow more concurrent connections
	sqlDB.SetConnMaxLifetime(15 * time.Minute) // Shorter lifetime for cloud environments
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)  // Close idle connections after 5 minutes

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	log.Println("Database connected!")
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
