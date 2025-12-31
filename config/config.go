package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
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

	// Try to parse DATABASE_URL if DB_HOST is not set
	if config.DBHost == "" {
		if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
			log.Println("Parsing DATABASE_URL...")
			if err := parseDBURL(dbURL, config); err != nil {
				log.Printf("Warning: Failed to parse DATABASE_URL: %v", err)
			}
		}
	}

	// Try to derive DB connection from SUPABASE_URL if still not set
	if config.DBHost == "" {
		if supabaseURL := os.Getenv("SUPABASE_URL"); supabaseURL != "" {
			log.Println("Deriving DB connection from SUPABASE_URL...")
			deriveFromSupabaseURL(supabaseURL, config)
		}
	}

	// Try SUPABASE_DB_PASSWORD as fallback for DB_PASSWORD
	if config.DBPassword == "" {
		if supabaseDBPass := os.Getenv("SUPABASE_DB_PASSWORD"); supabaseDBPass != "" {
			config.DBPassword = supabaseDBPass
			log.Println("Using SUPABASE_DB_PASSWORD")
		}
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

// parseDBURL parses a PostgreSQL DATABASE_URL and populates config
func parseDBURL(dbURL string, config *Config) error {
	u, err := url.Parse(dbURL)
	if err != nil {
		return err
	}

	config.DBHost = u.Hostname()
	if port := u.Port(); port != "" {
		config.DBPort = port
	}
	if u.User != nil {
		config.DBUser = u.User.Username()
		if pass, ok := u.User.Password(); ok {
			config.DBPassword = pass
		}
	}
	if len(u.Path) > 1 {
		config.DBName = u.Path[1:] // Remove leading /
	}

	log.Printf("Parsed DATABASE_URL: host=%s, port=%s, user=%s, db=%s",
		maskStr(config.DBHost), config.DBPort, config.DBUser, config.DBName)
	return nil
}

// deriveFromSupabaseURL extracts project ID from SUPABASE_URL and builds DB host
func deriveFromSupabaseURL(supabaseURL string, config *Config) {
	// SUPABASE_URL format: https://xxxxxxxxxxxxx.supabase.co
	// DB host format: db.xxxxxxxxxxxxx.supabase.co (direct) or
	//                 aws-0-ap-southeast-1.pooler.supabase.com (pooler)

	// Extract project ID from URL
	re := regexp.MustCompile(`https?://([a-z0-9]+)\.supabase\.co`)
	matches := re.FindStringSubmatch(supabaseURL)
	if len(matches) >= 2 {
		projectID := matches[1]
		// Use direct connection by default (more reliable for GORM)
		config.DBHost = fmt.Sprintf("db.%s.supabase.co", projectID)
		config.DBPort = "5432"
		config.DBName = "postgres"
		log.Printf("Derived DB host from SUPABASE_URL: %s", maskStr(config.DBHost))
	}
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
