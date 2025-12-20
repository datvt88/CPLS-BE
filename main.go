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
	"go_backend_project/services"

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
		"add":      func(a, b int) int { return a + b },
		"sub":      func(a, b int) int { return a - b },
		"subtract": func(a, b int) int { return a - b },
		"iterate": func(count int) []int {
			result := make([]int, count)
			for i := 0; i < count; i++ {
				result[i] = i
			}
			return result
		},
		"slice": func(s string, start, end int) string {
			if start < 0 {
				start = 0
			}
			if end > len(s) {
				end = len(s)
			}
			if start >= len(s) {
				return ""
			}
			return s[start:end]
		},
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

	// Close DuckDB connection
	if services.GlobalDuckDB != nil {
		if err := services.GlobalDuckDB.Close(); err != nil {
			log.Printf("Error closing DuckDB: %v", err)
		} else {
			log.Println("DuckDB closed successfully")
		}
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

	// Initialize DuckDB for local data storage
	if err := services.InitDuckDB(); err != nil {
		log.Printf("Warning: Failed to initialize DuckDB: %v", err)
	} else {
		log.Println("DuckDB initialized successfully")
	}

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
	// Create controllers
	supabaseClient := supabaseAuth.GetSupabaseClient()
	userMgmtCtrl := admin.NewUserManagementController(supabaseClient)
	stockCtrl := admin.NewStockController(supabaseClient)

	adminRoutes := router.Group("/admin")
	{
		protected := adminRoutes.Group("")
		protected.Use(supabaseAuth.AuthMiddleware())
		{
			// Dashboard
			protected.GET("", func(c *gin.Context) {
				c.HTML(http.StatusOK, "dashboard_simple.html", gin.H{
					"Title":     "CPLS Admin Dashboard",
					"AdminUser": c.GetString("admin_username"),
				})
			})
			protected.GET("/logout", supabaseAuth.Logout)

			// User Management Pages
			protected.GET("/users", userMgmtCtrl.ListUsers)

			// Stock Management Pages
			protected.GET("/stocks", stockCtrl.ListStocks)

			// User Management API
			userAPI := protected.Group("/api/users")
			{
				userAPI.GET("", func(c *gin.Context) {
					// API version of list users
					page := 1
					pageSize := 20
					search := c.Query("search")
					sortBy := c.DefaultQuery("sort_by", "created_at")
					sortOrder := c.DefaultQuery("sort_order", "desc")

					result, err := supabaseClient.GetProfiles(page, pageSize, search, sortBy, sortOrder)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
						return
					}
					c.JSON(http.StatusOK, result)
				})
				userAPI.GET("/stats", userMgmtCtrl.GetStats)
				userAPI.GET("/export", userMgmtCtrl.ExportUsers)
				userAPI.POST("", userMgmtCtrl.CreateUser)
				userAPI.POST("/sync-all", userMgmtCtrl.SyncAllUsers)
				userAPI.GET("/:id", userMgmtCtrl.GetUser)
				userAPI.PUT("/:id", userMgmtCtrl.UpdateUser)
				userAPI.DELETE("/:id", userMgmtCtrl.DeleteUser)
				userAPI.POST("/:id/ban", userMgmtCtrl.BanUser)
				userAPI.POST("/:id/unban", userMgmtCtrl.UnbanUser)
				userAPI.POST("/:id/subscription", userMgmtCtrl.UpdateSubscription)
				userAPI.POST("/:id/reset-password", userMgmtCtrl.ResetPassword)
				userAPI.POST("/:id/sync", userMgmtCtrl.SyncUser)
			}

			// Stock Management API
			stockAPI := protected.Group("/api/stocks")
			{
				stockAPI.GET("/stats", stockCtrl.GetStats)
				stockAPI.GET("/search", stockCtrl.SearchStocks)
				stockAPI.GET("/export", stockCtrl.ExportStocks)
				stockAPI.POST("/sync", stockCtrl.SyncStocks)
				stockAPI.POST("/import", stockCtrl.ImportStocks)
				stockAPI.GET("/:code", stockCtrl.GetStock)
				stockAPI.DELETE("/:code", stockCtrl.DeleteStock)
			}
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

		// Get profile stats too
		profileCount, _ := supabaseClient.GetProfileCount()

		c.JSON(http.StatusOK, gin.H{
			"status":       "ok",
			"message":      "Supabase connection successful",
			"admin_users":  count,
			"profiles":     profileCount,
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
