package main

import (
	"context"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go_backend_project/admin"
	"go_backend_project/admin/templates"
	"go_backend_project/config"
	"go_backend_project/models"
	"go_backend_project/routes"
	"go_backend_project/scheduler"
	"go_backend_project/services"

	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("==============================================")
	log.Println("  CPLS Backend API - Starting...")
	log.Println("==============================================")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Warning: Config load issue: %v", err)
	}

	// Set Gin mode based on environment
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize database connection
	db, err := config.InitDB()
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		log.Println("Starting in limited mode (health check only)...")
		startLimitedServer(cfg.Port)
		return
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

	// Initialize global services
	initializeGlobalServices()

	// Create Gin router
	router := gin.New()

	// Add middlewares
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(requestLogger())

	// Load HTML templates from embedded filesystem
	if err := loadTemplates(router); err != nil {
		log.Printf("Warning: Could not load templates: %v", err)
	}

	// Setup health check and root endpoints
	setupHealthEndpoints(router, db != nil)

	// Setup all API routes
	routes.SetupRoutes(router, db)

	// Setup initial admin routes (login page available without auth)
	setupInitialAdminRoutes(router, db)

	// Start background scheduler
	jobScheduler := scheduler.NewScheduler(db)
	go jobScheduler.Start()

	// Create HTTP server with timeouts optimized for Cloud Run
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on port %s", cfg.Port)
		log.Println("==============================================")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
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

// loadTemplates loads HTML templates from embedded filesystem
func loadTemplates(router *gin.Engine) error {
	// Get embedded templates
	tmplFS := templates.TemplateFS

	// Parse templates from embedded filesystem
	tmpl := template.New("")

	// Walk through embedded files and parse them
	err := fs.WalkDir(tmplFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path == "embed.go" {
			return nil
		}

		// Read and parse template
		content, err := fs.ReadFile(tmplFS, path)
		if err != nil {
			return err
		}

		_, err = tmpl.New(path).Parse(string(content))
		return err
	})

	if err != nil {
		return err
	}

	router.SetHTMLTemplate(tmpl)
	log.Println("HTML templates loaded successfully")
	return nil
}

// setupHealthEndpoints sets up health check endpoints for Cloud Run
func setupHealthEndpoints(router *gin.Engine, dbConnected bool) {
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
		if !dbConnected {
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

// setupInitialAdminRoutes sets up admin login routes that are available before full route setup
func setupInitialAdminRoutes(router *gin.Engine, db interface{}) {
	if db == nil {
		return
	}

	// Create auth controller for login
	gormDB := config.DB
	authController := admin.NewAuthController(gormDB)

	// Admin login routes (public)
	adminGroup := router.Group("/admin")
	{
		adminGroup.GET("/login", authController.LoginPage)
		adminGroup.POST("/login", authController.Login)
	}
}

// corsMiddleware returns a CORS middleware handler
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

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

	// Close database connection
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
