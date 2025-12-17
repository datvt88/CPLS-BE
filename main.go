package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go_backend_project/admin"
	"go_backend_project/config"
	"go_backend_project/models"
	"go_backend_project/routes"
	"go_backend_project/scheduler"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var globalDB *gorm.DB
var globalScheduler *scheduler.Scheduler
var globalAuthController *admin.AuthController
var authControllerMutex sync.RWMutex

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

	// Load templates with error handling (don't fail if templates can't be loaded)
	router.SetFuncMap(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	})
	if err := loadTemplates(router); err != nil {
		log.Printf("Warning: Failed to load templates: %v", err)
	}

	// Health check - always available (responds immediately for Cloud Run health checks)
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

	// Set up the auth controller setter callback for routes package
	routes.AuthControllerSetter = func(ac *admin.AuthController) {
		authControllerMutex.Lock()
		globalAuthController = ac
		authControllerMutex.Unlock()
	}

	// Setup initial admin routes before server starts (so /admin/login is always available)
	setupInitialAdminRoutes(router)

	// Start server IMMEDIATELY to respond to Cloud Run health checks
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

	log.Println("=== Server is ready to accept connections ===")

	// Initialize database and routes in background AFTER server is listening
	go initializeApp(router)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	// Stop scheduler if running
	if globalScheduler != nil {
		globalScheduler.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

// loadTemplates loads HTML templates with error recovery
func loadTemplates(router *gin.Engine) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("template loading panic: %v", r)
		}
	}()
	router.LoadHTMLGlob("admin/templates/*.html")
	return nil
}

// initializeApp performs database connection and route setup after server starts
func initializeApp(router *gin.Engine) {
	log.Println("Initializing application...")

	// Try to connect to database (with timeout)
	log.Println("Attempting database connection...")

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

	// Wait for DB connection with timeout (30 seconds for Cloud Run)
	var dbConnected bool
	select {
	case db := <-dbChan:
		if db != nil {
			globalDB = db
			dbConnected = true
			log.Println("Database connected successfully!")
		}
	case <-time.After(30 * time.Second):
		log.Println("Database connection timeout")
	}

	if dbConnected {
		// Run migrations
		log.Println("Running migrations...")
		runMigrations(globalDB)

		// Setup full routes
		routes.SetupRoutes(router, globalDB)

		// Start scheduler
		globalScheduler = scheduler.NewScheduler(globalDB)
		globalScheduler.Start()

		log.Println("=== Application fully initialized ===")
	} else {
		// Setup maintenance routes only
		setupMaintenanceRoutes(router)
		log.Println("=== Application started in maintenance mode (no database) ===")
	}
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
	// Use NoRoute handler for unknown paths instead of wildcard to avoid conflicts
	router.NoRoute(func(c *gin.Context) {
		// Check if it's an admin route
		if len(c.Request.URL.Path) >= 6 && c.Request.URL.Path[:6] == "/admin" {
			c.Redirect(http.StatusFound, "/admin/login")
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
	})

	// Note: admin login routes are registered in setupInitialAdminRoutes
	// Only register the redirect route here
	admin := router.Group("/admin")
	{
		admin.GET("", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/admin/login")
		})
	}
}

// setupInitialAdminRoutes registers admin login routes before server starts
// This ensures /admin/login is always available, even during DB initialization
func setupInitialAdminRoutes(router *gin.Engine) {
	adminGroup := router.Group("/admin")
	{
		adminGroup.GET("/login", func(c *gin.Context) {
			// If auth controller is set (DB connected and routes initialized), use it
			authControllerMutex.RLock()
			ac := globalAuthController
			authControllerMutex.RUnlock()
			if ac != nil {
				ac.LoginPage(c)
				return
			}
			// DB is not yet connected, show initializing message
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error": "System is initializing. Please wait a moment and refresh the page.",
			})
		})
		adminGroup.POST("/login", func(c *gin.Context) {
			// If auth controller is set (DB connected and routes initialized), use it
			authControllerMutex.RLock()
			ac := globalAuthController
			authControllerMutex.RUnlock()
			if ac != nil {
				ac.Login(c)
				return
			}
			// DB is not yet connected, cannot login
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error": "System is initializing. Please wait a moment and try again.",
			})
		})
	}
}
