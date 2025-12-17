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

var db *gorm.DB
var dbReady = false

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
		c.JSON(200, gin.H{
			"status":   "ok",
			"db_ready": dbReady,
		})
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "CPLS Backend API",
		})
	})

	// Setup maintenance routes first
	setupMaintenanceRoutes(router)

	// Start server immediately in goroutine
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

	// Give server time to start
	time.Sleep(100 * time.Millisecond)
	log.Println("Server is now listening")

	// Now try database connection (non-blocking for server)
	go initDatabaseAndRoutes(router, cfg)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func initDatabaseAndRoutes(router *gin.Engine, cfg *config.Config) {
	log.Println("Attempting database connection...")

	var err error
	db, err = config.InitDB()
	if err != nil {
		log.Printf("Database connection failed: %v", err)
		log.Println("Running in maintenance mode")
		return
	}

	dbReady = true
	log.Println("Database ready, running migrations...")

	// Run migrations
	if err := models.MigrateStockModels(db); err != nil {
		log.Printf("MigrateStockModels failed: %v", err)
	}
	if err := models.MigrateTradingModels(db); err != nil {
		log.Printf("MigrateTradingModels failed: %v", err)
	}
	if err := models.MigrateUserModels(db); err != nil {
		log.Printf("MigrateUserModels failed: %v", err)
	}
	if err := models.MigrateSubscriptionModels(db); err != nil {
		log.Printf("MigrateSubscriptionModels failed: %v", err)
	}
	if err := models.MigrateAdminModels(db); err != nil {
		log.Printf("MigrateAdminModels failed: %v", err)
	}

	// Seed admin
	if err := models.SeedDefaultAdminUser(db); err != nil {
		log.Printf("SeedDefaultAdminUser failed: %v", err)
	}

	// Setup full routes
	routes.SetupRoutes(router, db)

	// Start scheduler
	sched := scheduler.NewScheduler(db)
	sched.Start()

	log.Println("=== Application fully initialized ===")
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
			if !dbReady {
				c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
					"error": "Database connection failed. Please check configuration.",
				})
				return
			}
			c.HTML(http.StatusOK, "login.html", nil)
		})
		admin.POST("/login", func(c *gin.Context) {
			if !dbReady {
				c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
					"error": "Database connection failed.",
				})
				return
			}
			c.HTML(http.StatusOK, "login.html", gin.H{
				"error": "Please use the full routes after DB is ready.",
			})
		})
	}
}
