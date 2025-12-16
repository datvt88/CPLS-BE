package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go_backend_project/config"
	"go_backend_project/models"
	"go_backend_project/routes"
	"go_backend_project/scheduler"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting CPLS Backend API in %s mode...", cfg.Environment)

	// Set Gin mode
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize Gin router
	router := gin.Default()

	// CORS middleware
	router.Use(corsMiddleware())

	// Load HTML templates with custom functions (available even without database for maintenance pages)
	router.SetFuncMap(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	})
	router.LoadHTMLGlob("admin/templates/*.html")

	// Health check endpoint (available even without database)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":      "ok",
			"message":     "CPLS Backend API is running",
			"version":     "2.0.0",
			"environment": cfg.Environment,
			"db_host":     maskString(cfg.DBHost),
		})
	})

	// Root path - API info endpoint (available even without database)
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "CPLS Backend API is running",
			"version": "2.0.0",
			"endpoints": gin.H{
				"health": "/health",
				"api":    "/api/v1",
				"admin":  "/admin",
			},
		})
	})

	// Initialize database
	db, err := config.InitDB()
	if err != nil {
		log.Printf("Warning: Failed to connect to database: %v", err)
		log.Println("Server will start but database features will be unavailable")

		// Set up maintenance mode routes for admin panel
		setupMaintenanceRoutes(router)

		// Start server without database features
		startServer(router, cfg.Port)
		return
	}

	log.Println("Database connected successfully")

	// Run migrations
	log.Println("Running database migrations...")
	if err := models.MigrateStockModels(db); err != nil {
		log.Fatalf("Failed to migrate stock models: %v", err)
	}
	if err := models.MigrateTradingModels(db); err != nil {
		log.Fatalf("Failed to migrate trading models: %v", err)
	}
	if err := models.MigrateUserModels(db); err != nil {
		log.Fatalf("Failed to migrate user models: %v", err)
	}
	if err := models.MigrateSubscriptionModels(db); err != nil {
		log.Fatalf("Failed to migrate subscription models: %v", err)
	}
	if err := models.MigrateAdminModels(db); err != nil {
		log.Fatalf("Failed to migrate admin models: %v", err)
	}
	log.Println("Migrations completed successfully")

	// Seed default admin user
	if err := models.SeedDefaultAdminUser(db); err != nil {
		log.Printf("Warning: Failed to seed default admin user: %v", err)
	}

	// Setup routes
	routes.SetupRoutes(router, db)

	// Initialize and start scheduler
	sched := scheduler.NewScheduler(db)
	sched.Start()

	// Graceful shutdown handler
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("Shutting down gracefully...")
		sched.Stop()
		os.Exit(0)
	}()

	// Start server
	startServer(router, cfg.Port)
}

// startServer starts the HTTP server on the given port
func startServer(router *gin.Engine, port string) {
	log.Printf("Server starting on port %s", port)
	log.Printf("API documentation available at http://localhost:%s/health", port)

	if err := router.Run(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// corsMiddleware adds CORS headers
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// maintenanceErrorMessage is the error message shown when database is unavailable
const maintenanceErrorMessage = "Service temporarily unavailable. Database connection failed. Please try again later."

// setupMaintenanceRoutes sets up routes that display maintenance messages when database is unavailable
func setupMaintenanceRoutes(router *gin.Engine) {
	// Admin routes - show login page with maintenance error
	adminRoutes := router.Group("/admin")
	{
		// Admin root - redirect to login
		adminRoutes.GET("", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/admin/login")
		})
		adminRoutes.GET("/login", func(c *gin.Context) {
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error": maintenanceErrorMessage,
			})
		})
		adminRoutes.POST("/login", func(c *gin.Context) {
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error": maintenanceErrorMessage,
			})
		})
	}
}

// maskString masks a string for logging, showing only first 5 and last 3 characters
func maskString(s string) string {
	if len(s) <= 10 {
		return "***"
	}
	return s[:5] + "***" + s[len(s)-3:]
}