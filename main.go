package main

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go_backend_project/admin/templates"
	"go_backend_project/config"
	"go_backend_project/models"
	"go_backend_project/routes"
	"go_backend_project/scheduler"
	"go_backend_project/services"

	"github.com/gin-gonic/gin"
)

// dbInitialized tracks whether database has been successfully initialized
// This global variable is used for thread-safe access across goroutines to allow
// the /ready health endpoint to dynamically check database status
var dbInitialized bool
var dbInitMutex sync.RWMutex

func main() {
	log.Println("==============================================")
	log.Println("  CPLS Backend API - Starting...")
	log.Println("==============================================")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Warning: Config load issue: %v", err)
	}

	// Initialize CORS configuration at startup
	initCORSConfig()

	// Set Gin mode based on environment
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.New()

	// Configure trusted proxies for Cloud Run
	// Set to nil to trust all proxies (Cloud Run Load Balancer)
	// This is required for proper HTTPS detection behind Cloud Run proxy
	router.SetTrustedProxies(nil)

	// Add middlewares
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(requestLogger())

	// Load HTML templates from embedded filesystem
	if err := loadTemplates(router); err != nil {
		log.Printf("Warning: Could not load templates: %v", err)
	}

	// Setup health check endpoints FIRST so Cloud Run can detect the service is up
	// Database will be initialized in background
	setupHealthEndpoints(router)

	// Setup admin routes early (before database init) so login is always accessible
	// Admin routes will use Supabase auth if configured, or show error page if DB not ready
	routes.SetupAdminRoutes(router, nil)

	// Setup protected admin routes early if Supabase auth is available
	// If Supabase is configured, dashboard will be accessible immediately after login
	// If not, protected routes will be set up after database initialization
	routes.SetupAdminProtectedRoutesEarly(router)

	// Create HTTP server with timeouts optimized for Cloud Run
	// Bind to 0.0.0.0 explicitly for container networking
	server := &http.Server{
		Addr:              "0.0.0.0:" + cfg.Port,
		Handler:           router,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	// Start server IMMEDIATELY so Cloud Run knows we're listening
	go func() {
		log.Printf("Server listening on 0.0.0.0:%s", cfg.Port)
		log.Println("==============================================")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Initialize database and setup routes in background
	var jobScheduler *scheduler.Scheduler
	go func() {
		// Initialize database connection
		db, err := config.InitDB()
		if err != nil {
			log.Printf("ERROR: Database connection failed: %v", err)
			log.Println("Service will continue in limited mode (health check only)")
			return
		}

		// Initialize databases (PostgreSQL + MongoDB)
		dbConfig, err := config.InitDatabases()
		if err != nil {
			log.Printf("Warning: Failed to initialize all databases: %v", err)
		} else {
			log.Println("Database configuration initialized successfully")
		}

		// Run database migrations
		log.Println("Running database migrations...")
		if err := runMigrations(); err != nil {
			log.Printf("ERROR: Migration failed: %v", err)
		} else {
			log.Println("Database migrations completed successfully")
		}

		// Seed default admin user
		if err := models.SeedDefaultAdminUser(config.DB); err != nil {
			log.Printf("Warning: Could not seed admin user: %v", err)
		}

		// Initialize global services (including MongoDB client)
		initializeGlobalServices()

		// Create MongoDB indexes if MongoDB is available
		if dbConfig != nil && dbConfig.IsMongoDBAvailable() {
			crawler := services.NewCrawlerService(dbConfig.MongoDB, dbConfig.MongoDBName)
			if err := crawler.CreateIndexes(); err != nil {
				log.Printf("Warning: Failed to create MongoDB indexes: %v", err)
			}
		}

		// Mark database as ready
		dbInitMutex.Lock()
		dbInitialized = true
		dbInitMutex.Unlock()

		// Setup all API routes (includes admin routes with login)
		routes.SetupRoutes(router, db)

		// Start background scheduler
		jobScheduler = scheduler.NewScheduler(db)
		go jobScheduler.Start()

		log.Println("Application fully initialized with database")
	}()

	// Graceful shutdown
	gracefulShutdown(server, jobScheduler)
}

// runMigrations runs all database migrations
func runMigrations() error {
	db := config.DB

	// Migrate stock models
	if err := models.MigrateStockModels(db); err != nil {
		return err
	}

	// Migrate user models
	if err := models.MigrateUserModels(db); err != nil {
		return err
	}

	// Migrate trading models
	if err := models.MigrateTradingModels(db); err != nil {
		return err
	}

	// Migrate subscription models
	if err := models.MigrateSubscriptionModels(db); err != nil {
		return err
	}

	// Migrate signal condition models (includes seeding templates)
	if err := models.MigrateSignalConditionModels(db); err != nil {
		return err
	}

	// Migrate admin models
	if err := models.MigrateAdminModels(db); err != nil {
		return err
	}

	return nil
}

// initializeGlobalServices initializes global service instances
func initializeGlobalServices() {
	// Initialize price service first (indicator service depends on it)
	if err := services.InitPriceService(); err != nil {
		log.Printf("Warning: Failed to initialize price service: %v", err)
	}

	// Initialize indicator service
	if err := services.InitIndicatorService(); err != nil {
		log.Printf("Warning: Failed to initialize indicator service: %v", err)
	}

	// Initialize MongoDB client if configured
	if err := services.InitMongoDBClient(); err != nil {
		log.Printf("MongoDB not configured or failed to connect: %v", err)
	}

	log.Println("Global services initialized")
}

// templateFuncs returns custom template functions
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"subtract": func(a, b int) int {
			return a - b
		},
		"mul": func(a, b int) int {
			return a * b
		},
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"iterate": func(n int) []int {
			result := make([]int, n)
			for i := 0; i < n; i++ {
				result[i] = i
			}
			return result
		},
	}
}

// loadTemplates loads HTML templates from embedded filesystem
func loadTemplates(router *gin.Engine) error {
	// Get embedded templates
	tmplFS := templates.TemplateFS

	// Read layout template first
	layoutContent, err := fs.ReadFile(tmplFS, "layout.html")
	if err != nil {
		return fmt.Errorf("failed to read layout.html: %w", err)
	}

	// Create master template with custom functions
	masterTmpl := template.New("master").Funcs(templateFuncs())

	// Walk through embedded files and parse them
	var templateFiles []string
	err = fs.WalkDir(tmplFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path == "embed.go" {
			return nil
		}
		templateFiles = append(templateFiles, path)
		return nil
	})
	if err != nil {
		return err
	}

	// Parse each template file
	for _, path := range templateFiles {
		content, err := fs.ReadFile(tmplFS, path)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", path, err)
		}

		// Skip layout.html as it's embedded in each page template
		if path == "layout.html" {
			continue
		}

		// For login.html, parse it standalone (it's a complete page)
		if path == "login.html" {
			_, err = masterTmpl.New(path).Parse(string(content))
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}
			continue
		}

		// For content templates that define "content" and "scripts",
		// each page gets its own complete template with layout embedded.
		// We rename the "content" and "scripts" definitions to be unique per page
		// to avoid conflicts between templates in the same namespace.
		if strings.Contains(string(content), `{{ define "content" }}`) {
			// Create unique template names for content and scripts blocks
			// Replace generic "content" and "scripts" with page-specific names
			baseName := strings.TrimSuffix(path, ".html")
			pageContent := strings.ReplaceAll(string(content), `{{ define "content" }}`, fmt.Sprintf(`{{ define "%s_content" }}`, baseName))
			pageContent = strings.ReplaceAll(pageContent, `{{ define "scripts" }}`, fmt.Sprintf(`{{ define "%s_scripts" }}`, baseName))
			
			// Modify layout to use page-specific template names
			pageLayout := strings.ReplaceAll(string(layoutContent), `{{ template "content" . }}`, fmt.Sprintf(`{{ template "%s_content" . }}`, baseName))
			pageLayout = strings.ReplaceAll(pageLayout, `{{ template "scripts" . }}`, fmt.Sprintf(`{{ template "%s_scripts" . }}`, baseName))
			
			// Combine layout with page-specific content
			combinedContent := pageLayout + "\n" + pageContent
			_, err = masterTmpl.New(path).Parse(combinedContent)
			if err != nil {
				return fmt.Errorf("failed to parse combined template %s: %w", path, err)
			}
		} else {
			// Parse as standalone template
			_, err = masterTmpl.New(path).Parse(string(content))
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}
		}
	}

	router.SetHTMLTemplate(masterTmpl)
	log.Println("HTML templates loaded successfully")
	return nil
}

// setupHealthEndpoints sets up health check endpoints for Cloud Run
func setupHealthEndpoints(router *gin.Engine) {
	// Root endpoint
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "CPLS Backend API",
			"version": "1.0.0",
		})
	})

	// Liveness probe - always returns OK if server is running
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// Readiness probe - checks if service is ready to receive traffic
	router.GET("/ready", func(c *gin.Context) {
		dbInitMutex.RLock()
		isDBReady := dbInitialized
		dbInitMutex.RUnlock()

		if !isDBReady {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "not_ready",
				"message": "Database not connected",
			})
			return
		}

		// Check database connection
		sqlDB, err := config.DB.DB()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "not_ready",
				"message": "Database connection error",
			})
			return
		}

		if err := sqlDB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "not_ready",
				"message": "Database ping failed",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
		})
	})

	// Startup probe - can be used for initial health check
	router.GET("/startup", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "started",
		})
	})
}

// allowedOrigins is initialized at startup with parsed CORS origins
// These variables are written once in initCORSConfig() before any concurrent access,
// then read-only during request handling. This pattern is safe without mutex protection.
var allowedOrigins []string
var allowedDomainSuffixes []string

// initCORSConfig initializes CORS configuration at application startup
// MUST be called before starting the HTTP server to ensure thread-safe access
func initCORSConfig() {
	// Get allowed origins from environment variable (comma-separated)
	allowedOriginsEnv := os.Getenv("CORS_ALLOWED_ORIGINS")
	
	// Default allowed origins for development
	defaultAllowedOrigins := []string{
		"http://localhost:3000",     // Local frontend development
		"http://localhost:8080",     // Local backend
		"https://localhost:3000",    // Local HTTPS frontend
	}
	
	// Parse allowed origins from environment
	if allowedOriginsEnv != "" {
		origins := strings.Split(allowedOriginsEnv, ",")
		for _, origin := range origins {
			trimmed := strings.TrimSpace(origin)
			if trimmed != "" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	} else {
		allowedOrigins = defaultAllowedOrigins
	}
	
	// Parse allowed domain suffixes from environment (for trusted platforms)
	// Format: comma-separated list of domain suffixes (e.g., ".run.app,.supabase.co")
	allowedSuffixesEnv := os.Getenv("CORS_ALLOWED_DOMAIN_SUFFIXES")
	if allowedSuffixesEnv != "" {
		suffixes := strings.Split(allowedSuffixesEnv, ",")
		for _, suffix := range suffixes {
			trimmed := strings.TrimSpace(suffix)
			if trimmed != "" {
				allowedDomainSuffixes = append(allowedDomainSuffixes, trimmed)
			}
		}
	} else {
		// Default: Allow Cloud Run and Supabase domains
		// For production, set CORS_ALLOWED_DOMAIN_SUFFIXES to restrict to specific domains
		allowedDomainSuffixes = []string{
			".run.app",     // Google Cloud Run
			".supabase.co", // Supabase
		}
	}
	
	log.Printf("CORS: Allowed %d specific origins and %d domain suffixes", len(allowedOrigins), len(allowedDomainSuffixes))
}

// isAllowedOrigin checks if the origin is in the allowlist
func isAllowedOrigin(origin string) bool {
	// Check exact matches first (most secure)
	for _, allowed := range allowedOrigins {
		if origin == allowed {
			return true
		}
	}
	
	// Check domain suffixes (for trusted platforms)
	// Note: Only allows if the origin is HTTPS (production) to prevent subdomain attacks on HTTP
	if strings.HasPrefix(origin, "https://") {
		for _, suffix := range allowedDomainSuffixes {
			if strings.HasSuffix(origin, suffix) {
				return true
			}
		}
	}
	
	return false
}

// corsMiddleware returns a CORS middleware handler
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		
		// IMPORTANT: When Access-Control-Allow-Credentials is true,
		// Access-Control-Allow-Origin CANNOT be "*" - it must be a specific origin.
		// For same-origin requests (no Origin header), we don't need to set CORS headers
		// since the browser will allow the request by default.
		if origin != "" && isAllowedOrigin(origin) {
			// Cross-origin request from allowed origin - set all CORS headers
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// requestLogger returns a request logging middleware
func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip logging for health checks to reduce noise
		path := c.Request.URL.Path
		if path == "/health" || path == "/ready" || path == "/startup" {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		// Only log errors or slow requests in production
		if c.Writer.Status() >= 400 || duration > 1*time.Second {
			log.Printf("%s %s %d %v", c.Request.Method, path, c.Writer.Status(), duration)
		}
	}
}

// gracefulShutdown handles graceful shutdown of the server
func gracefulShutdown(server *http.Server, jobScheduler *scheduler.Scheduler) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-quit
	log.Printf("Received signal %v, shutting down gracefully...", sig)

	// Stop scheduler first
	if jobScheduler != nil {
		jobScheduler.Stop()
	}

	// Create context with timeout for shutdown
	// Cloud Run gives 10 seconds for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Close MongoDB connection if initialized
	if config.GlobalDBConfig != nil {
		config.GlobalDBConfig.CloseConnections()
	}

	// Close PostgreSQL connection
	if config.DB != nil {
		sqlDB, err := config.DB.DB()
		if err == nil {
			sqlDB.Close()
			log.Println("Database connection closed")
		}
	}

	log.Println("Server shutdown completed")
}

// startLimitedServer starts a minimal server when database is not available
func startLimitedServer(port string) {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// Load HTML templates for limited mode too
	if err := loadTemplates(router); err != nil {
		log.Printf("Warning: Could not load templates in limited mode: %v", err)
	}

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "limited",
			"message": "CPLS Backend API - Database not connected",
		})
	})

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	router.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "not_ready",
			"message": "Database not connected",
		})
	})

	// Admin routes in limited mode - show error page
	adminRoutes := router.Group("/admin")
	{
		adminRoutes.GET("/login", func(c *gin.Context) {
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error":           "Database not connected. Please check server configuration.",
				"supabaseEnabled": false,
			})
		})
		adminRoutes.POST("/login", func(c *gin.Context) {
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error":           "Database not connected. Cannot authenticate.",
				"supabaseEnabled": false,
			})
		})
		adminRoutes.GET("", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/admin/login")
		})
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Limited server listening on port %s", port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down limited server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
