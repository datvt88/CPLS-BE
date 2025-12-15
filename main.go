package main

import (
	"fmt"
	"log"
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

	// Initialize database
	db, err := config.InitDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
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
	log.Println("Migrations completed successfully")

	// Set Gin mode
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize Gin router
	router := gin.Default()

	// CORS middleware
	router.Use(corsMiddleware())

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
	port := cfg.Port
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