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
var globalSupabaseAuthController *admin.SupabaseAuthController
var authControllerMutex sync.RWMutex
var connectionError string // Store connection error message

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
	if err := loadTemplates(router); err != nil {
		log.Printf("Warning: Failed to load templates: %v", err)
	}

	// Initialize connection BEFORE starting server
	initializeConnection(router)

	// Setup basic routes
	setupBasicRoutes(router)

	// Setup admin login routes
	setupAdminLoginRoutes(router)

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

	if globalScheduler != nil {
		globalScheduler.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

// initializeConnection initializes database or Supabase connection
func initializeConnection(router *gin.Engine) {
	dbHost := os.Getenv("DB_HOST")
	supabaseURL := os.Getenv("SUPABASE_URL")

	// Supabase-only mode: no DB_HOST, only SUPABASE_URL
	if dbHost == "" && supabaseURL != "" {
		log.Println("Using Supabase-only mode...")
		initSupabaseOnly(router)
		return
	}

	// Direct database connection mode
	if dbHost != "" {
		log.Println("Using direct database connection...")
		initDirectDB(router)
		return
	}

	// No configuration - set error
	connectionError = "No database configuration found. Please set SUPABASE_URL or DB_HOST in environment variables."
	log.Println("ERROR: " + connectionError)
}

// initSupabaseOnly initializes Supabase-only mode
func initSupabaseOnly(router *gin.Engine) {
	supabaseAuth, err := admin.NewSupabaseAuthController()
	if err != nil {
		connectionError = fmt.Sprintf("Failed to initialize Supabase: %v", err)
		log.Println("ERROR: " + connectionError)
		return
	}

	// Test connection
	if err := supabaseAuth.TestConnection(); err != nil {
		connectionError = fmt.Sprintf("Failed to connect to Supabase: %v", err)
		log.Println("ERROR: " + connectionError)
		return
	}

	// Success - set global controller
	authControllerMutex.Lock()
	globalSupabaseAuthController = supabaseAuth
	authControllerMutex.Unlock()

	// Setup protected admin routes
	setupSupabaseAdminRoutes(router, supabaseAuth)

	log.Println("Supabase connection successful!")
}

// initDirectDB initializes direct database connection
func initDirectDB(router *gin.Engine) {
	db, err := config.InitDB()
	if err != nil {
		connectionError = fmt.Sprintf("Failed to connect to database: %v", err)
		log.Println("ERROR: " + connectionError)
		return
	}

	globalDB = db

	// Run migrations
	log.Println("Running migrations...")
	runMigrations(globalDB)

	// Setup full routes
	routes.SetupRoutes(router, globalDB)

	// Start scheduler
	globalScheduler = scheduler.NewScheduler(globalDB)
	globalScheduler.Start()

	log.Println("Database connection successful!")
}

// setupBasicRoutes sets up health check and basic routes
func setupBasicRoutes(router *gin.Engine) {
	router.GET("/health", func(c *gin.Context) {
		status := "ok"
		if connectionError != "" {
			status = "error"
		}
		c.JSON(200, gin.H{
			"status":  status,
			"message": connectionError,
		})
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "CPLS Backend API",
		})
	})
}

// setupAdminLoginRoutes sets up admin login routes
func setupAdminLoginRoutes(router *gin.Engine) {
	adminGroup := router.Group("/admin")
	{
		adminGroup.GET("/login", func(c *gin.Context) {
			// Check if we have a connection error
			if connectionError != "" {
				c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
					"error": connectionError,
				})
				return
			}

			// Check for Supabase controller
			authControllerMutex.RLock()
			supabaseAC := globalSupabaseAuthController
			authControllerMutex.RUnlock()

			if supabaseAC != nil {
				supabaseAC.LoginPage(c)
				return
			}

			// Should not reach here if properly initialized
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error": "System not properly initialized",
			})
		})

		adminGroup.POST("/login", func(c *gin.Context) {
			// Check if we have a connection error
			if connectionError != "" {
				c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
					"error": connectionError,
				})
				return
			}

			// Check for Supabase controller
			authControllerMutex.RLock()
			supabaseAC := globalSupabaseAuthController
			authControllerMutex.RUnlock()

			if supabaseAC != nil {
				supabaseAC.Login(c)
				return
			}

			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error": "System not properly initialized",
			})
		})
	}
}

// setupSupabaseAdminRoutes sets up protected admin routes for Supabase mode
func setupSupabaseAdminRoutes(router *gin.Engine, supabaseAuth *admin.SupabaseAuthController) {
	adminRoutes := router.Group("/admin")
	{
		protected := adminRoutes.Group("")
		protected.Use(supabaseAuth.AuthMiddleware())
		{
			protected.GET("", func(c *gin.Context) {
				c.HTML(http.StatusOK, "dashboard_simple.html", gin.H{
					"Title":     "CPLS Admin Dashboard",
					"AdminUser": c.GetString("admin_username"),
				})
			})
			protected.GET("/logout", supabaseAuth.Logout)
		}
	}

	// API health check for Supabase mode
	router.GET("/api/v1/health/db", func(c *gin.Context) {
		if connectionError != "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"message": connectionError,
			})
			return
		}

		client := supabaseAuth.GetSupabaseClient()
		count, err := client.GetAdminUserCount()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Supabase error: %v", err),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":       "ok",
			"message":      "Supabase connection successful",
			"admin_users":  count,
			"db_connected": true,
		})
	})
}

// loadTemplates loads HTML templates
func loadTemplates(router *gin.Engine) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("template loading panic: %v", r)
		}
	}()
	router.LoadHTMLGlob("admin/templates/*.html")
	return nil
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
