package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go_backend_project/admin"
	"go_backend_project/admin/templates"
	"go_backend_project/config"
	"go_backend_project/controllers"
	"go_backend_project/models"
	"go_backend_project/routes"
	"go_backend_project/scheduler"
	"go_backend_project/services"
	"go_backend_project/services/signals"

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

	// Initialize FAST local services first (non-blocking)
	initFastLocalServices()

	// Initialize connection (Supabase auth) - needed for routes
	initializeConnection(router)

	// Setup basic routes
	setupBasicRoutes(router)

	// Setup admin login routes
	setupAdminLoginRoutes(router)

	// Start server FIRST - Cloud Run needs port to be listening quickly
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

	// Initialize SLOW services in background (MongoDB, data restore)
	go initSlowServices()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	if globalScheduler != nil {
		globalScheduler.Stop()
	}

	// Stop Stock Scheduler
	if services.GlobalStockScheduler != nil {
		services.GlobalStockScheduler.Stop()
	}

	// Shutdown Realtime Price Service
	if services.GlobalRealtimeService != nil {
		services.GlobalRealtimeService.Shutdown()
	}

	// Close MongoDB connection
	if services.GlobalMongoClient != nil {
		if err := services.GlobalMongoClient.Close(); err != nil {
			log.Printf("Error closing MongoDB: %v", err)
		} else {
			log.Println("MongoDB connection closed successfully")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

// initFastLocalServices initializes fast, non-blocking local services
// These must complete quickly so the server can start listening on the port
func initFastLocalServices() {
	log.Println("Initializing fast local services...")

	// Initialize Stock Scheduler
	if err := services.InitStockScheduler(); err != nil {
		log.Printf("Warning: Failed to initialize Stock Scheduler: %v", err)
	} else {
		log.Println("Stock Scheduler initialized successfully")
	}

	// Initialize Price Service
	if err := services.InitPriceService(); err != nil {
		log.Printf("Warning: Failed to initialize Price Service: %v", err)
	} else {
		log.Println("Price Service initialized successfully")
	}

	// Initialize Indicator Service
	if err := services.InitIndicatorService(); err != nil {
		log.Printf("Warning: Failed to initialize Indicator Service: %v", err)
	} else {
		log.Println("Indicator Service initialized successfully")
	}

	// Initialize Realtime Price Service (WebSocket)
	if err := services.InitRealtimePriceService(); err != nil {
		log.Printf("Warning: Failed to initialize Realtime Price Service: %v", err)
	} else {
		log.Println("Realtime Price Service initialized successfully")
	}

	// Initialize Signal Service for algorithmic trading
	if err := signals.InitSignalService(); err != nil {
		log.Printf("Warning: Failed to initialize Signal Service: %v", err)
	} else {
		log.Println("Signal Service initialized successfully")
	}

	// Initialize Signal Condition Evaluator for custom conditions
	// Note: This requires database connection, defer to after DB is ready
	log.Println("Signal Condition Evaluator will be initialized after database connection")

	log.Println("Fast local services initialized")
}

// initSlowServices initializes slow services in background after server starts
// This includes MongoDB connection and data restoration which may take time
func initSlowServices() {
	log.Println("Initializing slow services in background...")

	// Initialize MongoDB Atlas for cloud persistence (may timeout, run in background)
	if err := services.InitMongoDBClient(); err != nil {
		log.Printf("Warning: Failed to initialize MongoDB Atlas: %v", err)
	} else if services.GlobalMongoClient != nil && services.GlobalMongoClient.IsConfigured() {
		log.Println("MongoDB Atlas initialized successfully")

		// Only restore data if MongoDB connected successfully
		restoreDataFromMongoDB()
	}

	log.Println("Slow services initialization complete")
}

// restoreDataFromMongoDB restores data from MongoDB Atlas if local data is missing
func restoreDataFromMongoDB() {
	if services.GlobalMongoClient == nil || !services.GlobalMongoClient.IsConfigured() {
		return
	}

	// 1. Restore stock list if local file is missing
	stocks, err := services.LoadStocksFromFile()
	if err != nil || len(stocks) == 0 {
		log.Println("Local stock list not found, attempting restore from MongoDB Atlas...")
		stocks, err := services.GlobalMongoClient.LoadStockList()
		if err == nil && len(stocks) > 0 {
			if err := services.SaveStocksToFile(stocks); err != nil {
				log.Printf("Warning: Could not cache stock list locally: %v", err)
			} else {
				log.Printf("Restored %d stocks from MongoDB Atlas", len(stocks))
			}
		} else if err != nil {
			log.Printf("Warning: Could not restore stock list from MongoDB: %v", err)
		}
	}

	// 2. Restore price data if local files are missing
	if services.GlobalPriceService != nil && !services.GlobalPriceService.HasLocalPriceData() {
		log.Println("Local price data not found, attempting restore from MongoDB Atlas...")
		if err := services.GlobalPriceService.RestoreFromMongoDB(); err != nil {
			log.Printf("Warning: Could not restore price data from MongoDB: %v", err)
		}
	}

	// 3. Restore indicators from MongoDB if not cached locally
	if services.GlobalIndicatorService != nil {
		if _, err := services.GlobalIndicatorService.LoadIndicatorSummary(); err != nil {
			log.Printf("Note: Indicators will be calculated when needed: %v", err)
		}
	}
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

	// Initialize Signal Condition Evaluator (requires DB)
	if err := signals.InitConditionEvaluator(globalDB); err != nil {
		log.Printf("Warning: Failed to initialize Signal Condition Evaluator: %v", err)
	} else {
		log.Println("Signal Condition Evaluator initialized successfully")
	}

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

			// API Status Page
			protected.GET("/api-status", stockCtrl.APIStatusPage)

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
				stockAPI.GET("/scheduler", stockCtrl.GetSchedulerConfig)
				stockAPI.PUT("/scheduler", stockCtrl.UpdateSchedulerConfig)
				stockAPI.GET("/:code", stockCtrl.GetStock)
				stockAPI.DELETE("/:code", stockCtrl.DeleteStock)
			}

			// Price Data API
			priceAPI := protected.Group("/api/prices")
			{
				priceAPI.GET("/config", stockCtrl.GetPriceConfig)
				priceAPI.PUT("/config", stockCtrl.UpdatePriceConfig)
				priceAPI.GET("/stats", stockCtrl.GetPriceSyncStats)
				priceAPI.GET("/progress", stockCtrl.GetPriceSyncProgress)
				priceAPI.POST("/sync", stockCtrl.StartPriceSync)
				priceAPI.POST("/stop", stockCtrl.StopPriceSync)
				priceAPI.GET("/:code", stockCtrl.GetStockPrice)
				priceAPI.POST("/:code", stockCtrl.SyncSingleStockPrice)
			}

			// Technical Indicators API
			indicatorAPI := protected.Group("/api/indicators")
			{
				indicatorAPI.POST("/calculate", stockCtrl.CalculateAllIndicators)
				indicatorAPI.GET("/summary", stockCtrl.GetIndicatorSummary)
				indicatorAPI.GET("/top-rs", stockCtrl.GetTopRSStocks)
				indicatorAPI.POST("/filter", stockCtrl.FilterStocks)
				indicatorAPI.GET("/:code", stockCtrl.GetStockIndicators)
			}

			// Realtime Price API
			realtimeAPI := protected.Group("/api/realtime")
			{
				realtimeAPI.GET("/status", stockCtrl.GetRealtimeStatus)
				realtimeAPI.POST("/start", stockCtrl.StartRealtimePolling)
				realtimeAPI.POST("/stop", stockCtrl.StopRealtimePolling)
			}

			// MongoDB Atlas API
			mongoAPI := protected.Group("/api/mongodb")
			{
				mongoAPI.GET("/status", stockCtrl.GetMongoDBStatus)
				mongoAPI.POST("/sync-to", stockCtrl.SyncToMongoDB)
				mongoAPI.POST("/restore-from", stockCtrl.RestoreFromMongoDB)
				mongoAPI.POST("/reconnect", stockCtrl.ReconnectMongoDB)
			}

			// Files Status API
			filesAPI := protected.Group("/api/files")
			{
				filesAPI.GET("/status", stockCtrl.GetFilesStatus)
				filesAPI.GET("/view", stockCtrl.ViewFile)
			}

			// Public API Toggle
			protected.POST("/api/toggle-public-api", stockCtrl.TogglePublicAPI)
		}

		// WebSocket endpoint (outside auth middleware for direct connection)
		adminRoutes.GET("/ws/realtime", stockCtrl.HandleRealtimeWebSocket)
	}

	// Public Signal API routes (no auth required for frontend)
	// These work without database - use GlobalSignalService and GlobalIndicatorService
	api := router.Group("/api/v1")
	{
		// Health check for Signal API
		api.GET("/health", func(c *gin.Context) {
			status := gin.H{
				"status":    "ok",
				"service":   "CPLS Signal API",
				"timestamp": time.Now().Format(time.RFC3339),
			}

			// Check Signal Service
			if signals.GlobalSignalService != nil {
				status["signal_service"] = "available"
			} else {
				status["signal_service"] = "not_initialized"
			}

			// Check Indicator Service
			if services.GlobalIndicatorService != nil {
				status["indicator_service"] = "available"
			} else {
				status["indicator_service"] = "not_initialized"
			}

			// Check MongoDB
			if services.GlobalMongoClient != nil && services.GlobalMongoClient.IsConfigured() {
				status["mongodb"] = "connected"
			} else {
				status["mongodb"] = "not_connected"
			}

			c.JSON(http.StatusOK, status)
		})

		// Public Signal API - optimized for frontend consumption
		publicSignalController := controllers.NewPublicSignalController()
		publicSignalController.RegisterPublicSignalRoutes(api)
	}

	log.Println("Public Signal API routes registered")

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

// loadTemplates loads HTML templates from embedded filesystem
func loadTemplates(router *gin.Engine) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("template loading panic: %v", r)
		}
	}()

	// Parse templates from embedded filesystem
	tmpl := template.New("").Funcs(router.FuncMap)
	tmpl, err = tmpl.ParseFS(templates.TemplateFS, "*.html")
	if err != nil {
		return fmt.Errorf("failed to parse embedded templates: %v", err)
	}

	router.SetHTMLTemplate(tmpl)
	log.Println("Templates loaded from embedded filesystem")
	return nil
}

func runMigrations(db *gorm.DB) {
	if err := models.MigrateStockModels(db); err != nil {
		log.Printf("MigrateStockModels: %v", err)
	}
	if err := models.MigrateTradingModels(db); err != nil {
		log.Printf("MigrateTradingModels: %v", err)
	}
	if err := models.MigrateSignalConditionModels(db); err != nil {
		log.Printf("MigrateSignalConditionModels: %v", err)
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
	// Get allowed origins from environment variable (comma-separated)
	corsOrigins := os.Getenv("CORS_ORIGINS")
	allowedOrigins := make(map[string]bool)

	// Parse allowed origins
	if corsOrigins != "" {
		for _, origin := range strings.Split(corsOrigins, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				allowedOrigins[origin] = true
			}
		}
	}

	// Log CORS configuration on startup
	if len(allowedOrigins) > 0 {
		log.Printf("CORS: Allowing origins: %v", corsOrigins)
	} else {
		log.Println("CORS: No restrictions configured, allowing all origins")
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Determine which origin to allow
		if len(allowedOrigins) == 0 {
			// No restrictions - allow all origins
			if origin != "" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			}
		} else if allowedOrigins[origin] {
			// Origin is in allowed list
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else if origin != "" {
			// Origin not allowed - still set header but with first allowed origin
			// This prevents silent failures, browser will show clear CORS error
			for allowedOrigin := range allowedOrigins {
				c.Writer.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				break
			}
		}

		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
