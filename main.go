package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go_backend_project/config"
	"go_backend_project/models"
	"go_backend_project/routes"
	"go_backend_project/scheduler"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var globalDB *gorm.DB

func main() {
	log.Println("=== CPLS Backend Starting ===")

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("LoadConfig failed: %v", err)
	}

	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Create router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// Load templates
	router.SetFuncMap(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	})
	router.LoadHTMLGlob("admin/templates/*.html")

	// Health check - always available
	router.GET("/health", func(c *gin.Context) {
		dbStatus := "disconnected"
		if globalDB != nil {
			dbStatus = "connected"
		}
		c.JSON(200, gin.H{
			"status":    "ok",
			"db_status": dbStatus,
		})
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "CPLS Backend API",
		})
	})

	// Try to connect to database FIRST (with timeout)
	log.Println("Attempting database connection...")
	dbConnected := false

	// Create a channel to signal DB connection result
	dbChan := make(chan *gorm.DB, 1)
	go func() {
		db, err := config.InitDB()
		if err != nil {
			log.Printf("Database connection failed: %v", err)
			dbChan <- nil
		} else {
			dbChan <- db
		}
	}()

	// Wait for DB connection with timeout (max 8 seconds to leave time for server startup)
	select {
	case db := <-dbChan:
		if db != nil {
			globalDB = db
			dbConnected = true
			log.Println("Database connected successfully!")
		}
	case <-time.After(8 * time.Second):
		log.Println("Database connection timeout, starting without DB")
	}

	if dbConnected {
		// Run migrations
		log.Println("Running migrations...")
		runMigrations(globalDB)

		// Setup full routes
		routes.SetupRoutes(router, globalDB)

		// Start scheduler
		sched := scheduler.NewScheduler(globalDB)
		sched.Start()
		defer sched.Stop()
	} else {
		// Setup maintenance routes only
		setupMaintenanceRoutes(router)
	}

	// Start server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: router,
	}

	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	log.Println("=== Server is ready ===")

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func runMigrations(db *gorm.DB) {
	if err := models.MigrateStockModels(db); err != nil {
		log.Printf("MigrateStockModels: %v", err)
	}
	if err := models.MigrateTradingModels(db); err != nil {
		log.Printf("MigrateTradingModels: %v", err)
	}
	if err := models.MigrateUserModels(db); err != nil {
		log.Printf("MigrateUserModels: %v", err)
	}
	if err := models.MigrateSubscriptionModels(db); err != nil {
		log.Printf("MigrateSubscriptionModels: %v", err)
	}
	if err := models.MigrateAdminModels(db); err != nil {
		log.Printf("MigrateAdminModels: %v", err)
	}
	if err := models.SeedDefaultAdminUser(db); err != nil {
		log.Printf("SeedDefaultAdminUser: %v", err)
	}
	log.Println("Migrations completed")
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func setupMaintenanceRoutes(router *gin.Engine) {
	admin := router.Group("/admin")
	{
		admin.GET("", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/admin/login")
		})
		admin.GET("/login", func(c *gin.Context) {
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error": "Database connection failed. Please check DB_HOST, DB_USER, DB_PASSWORD, DB_NAME environment variables.",
			})
		})
		admin.POST("/login", func(c *gin.Context) {
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error": "Database connection failed. Cannot login at this time.",
			})
		})
		// Catch all other admin routes
		admin.GET("/*path", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/admin/login")
		})
	}
}
