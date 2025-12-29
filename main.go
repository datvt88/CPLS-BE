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
var connectionError string

func main() {
	log.Println("=== CPLS Backend Starting ===")

	// Get port from environment - CRITICAL for Cloud Run
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Create minimal router
	router := gin.New()
	router.Use(gin.Recovery())

	// Setup health check IMMEDIATELY
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "message": "CPLS Backend API"})
	})

	// Start server IMMEDIATELY
	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		log.Printf("Server listening on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Give server time to bind port
	time.Sleep(50 * time.Millisecond)
	log.Println("=== Server is listening ===")

	// Initialize everything else in background
	go initializeApp(router)

	// Wait for shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	shutdownServices()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func initializeApp(router *gin.Engine) {
	log.Println("Initializing application...")

	// Load config
	_, err := config.LoadConfig()
	if err != nil {
		log.Printf("Warning: LoadConfig failed: %v", err)
	}

	// Add CORS middleware
	router.Use(corsMiddleware())

	// Setup templates
	setupTemplates(router)

	// Initialize services
	initServices()

	// Initialize connection and routes
	initializeConnection(router)

	// Setup admin login routes
	setupAdminLoginRoutes(router)

	log.Println("=== Application initialized ===")
}

func setupTemplates(router *gin.Engine) {
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

	// Load embedded templates
	tmpl := template.New("").Funcs(router.FuncMap)
	tmpl, err := tmpl.ParseFS(templates.TemplateFS, "*.html")
	if err != nil {
		log.Printf("Warning: Failed to parse templates: %v", err)
		return
	}
	router.SetHTMLTemplate(tmpl)
	log.Println("Templates loaded")
}

func initServices() {
	log.Println("Initializing services...")

	// Initialize local services
	if err := services.InitStockScheduler(); err != nil {
		log.Printf("Warning: Stock Scheduler: %v", err)
	}
	if err := services.InitPriceService(); err != nil {
		log.Printf("Warning: Price Service: %v", err)
	}
	if err := services.InitIndicatorService(); err != nil {
		log.Printf("Warning: Indicator Service: %v", err)
	}
	if err := services.InitRealtimePriceService(); err != nil {
		log.Printf("Warning: Realtime Service: %v", err)
	}
	if err := signals.InitSignalService(); err != nil {
		log.Printf("Warning: Signal Service: %v", err)
	}

	// Initialize MongoDB in background
	go func() {
		if err := services.InitMongoDBClient(); err != nil {
			log.Printf("Warning: MongoDB: %v", err)
		} else if services.GlobalMongoClient != nil && services.GlobalMongoClient.IsConfigured() {
			log.Println("MongoDB connected")
			restoreDataFromMongoDB()
		}
	}()

	log.Println("Services initialized")
}

func restoreDataFromMongoDB() {
	if services.GlobalMongoClient == nil || !services.GlobalMongoClient.IsConfigured() {
		return
	}

	stocks, err := services.LoadStocksFromFile()
	if err != nil || len(stocks) == 0 {
		stocks, err := services.GlobalMongoClient.LoadStockList()
		if err == nil && len(stocks) > 0 {
			services.SaveStocksToFile(stocks)
			log.Printf("Restored %d stocks from MongoDB", len(stocks))
		}
	}

	if services.GlobalPriceService != nil && !services.GlobalPriceService.HasLocalPriceData() {
		services.GlobalPriceService.RestoreFromMongoDB()
	}
}

func initializeConnection(router *gin.Engine) {
	dbHost := os.Getenv("DB_HOST")
	supabaseURL := os.Getenv("SUPABASE_URL")

	if dbHost == "" && supabaseURL != "" {
		log.Println("Using Supabase-only mode...")
		initSupabaseOnly(router)
		return
	}

	if dbHost != "" {
		log.Println("Using direct database connection...")
		initDirectDB(router)
		return
	}

	connectionError = "No database configuration found"
	log.Println("Warning: " + connectionError)
}

func initSupabaseOnly(router *gin.Engine) {
	supabaseAuth, err := admin.NewSupabaseAuthController()
	if err != nil {
		connectionError = fmt.Sprintf("Supabase init failed: %v", err)
		log.Println("ERROR: " + connectionError)
		return
	}

	if err := supabaseAuth.TestConnection(); err != nil {
		connectionError = fmt.Sprintf("Supabase connection failed: %v", err)
		log.Println("ERROR: " + connectionError)
		return
	}

	authControllerMutex.Lock()
	globalSupabaseAuthController = supabaseAuth
	authControllerMutex.Unlock()

	setupSupabaseAdminRoutes(router, supabaseAuth)
	log.Println("Supabase connected!")
}

func initDirectDB(router *gin.Engine) {
	db, err := config.InitDB()
	if err != nil {
		connectionError = fmt.Sprintf("Database connection failed: %v", err)
		log.Println("ERROR: " + connectionError)
		return
	}

	globalDB = db
	runMigrations(globalDB)

	if err := signals.InitConditionEvaluator(globalDB); err != nil {
		log.Printf("Warning: Signal Condition Evaluator: %v", err)
	}

	routes.SetupRoutes(router, globalDB)

	globalScheduler = scheduler.NewScheduler(globalDB)
	globalScheduler.Start()

	log.Println("Database connected!")
}

func setupAdminLoginRoutes(router *gin.Engine) {
	adminGroup := router.Group("/admin")
	{
		adminGroup.GET("/login", func(c *gin.Context) {
			if connectionError != "" {
				c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{"error": connectionError})
				return
			}

			authControllerMutex.RLock()
			supabaseAC := globalSupabaseAuthController
			authControllerMutex.RUnlock()

			if supabaseAC != nil {
				supabaseAC.LoginPage(c)
				return
			}

			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{"error": "System not initialized"})
		})

		adminGroup.POST("/login", func(c *gin.Context) {
			if connectionError != "" {
				c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{"error": connectionError})
				return
			}

			authControllerMutex.RLock()
			supabaseAC := globalSupabaseAuthController
			authControllerMutex.RUnlock()

			if supabaseAC != nil {
				supabaseAC.Login(c)
				return
			}

			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{"error": "System not initialized"})
		})
	}
}

func setupSupabaseAdminRoutes(router *gin.Engine, supabaseAuth *admin.SupabaseAuthController) {
	supabaseClient := supabaseAuth.GetSupabaseClient()
	userMgmtCtrl := admin.NewUserManagementController(supabaseClient)
	stockCtrl := admin.NewStockController(supabaseClient)

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
			protected.GET("/users", userMgmtCtrl.ListUsers)
			protected.GET("/stocks", stockCtrl.ListStocks)
			protected.GET("/api-status", stockCtrl.APIStatusPage)

			// User API
			userAPI := protected.Group("/api/users")
			{
				userAPI.GET("", func(c *gin.Context) {
					result, err := supabaseClient.GetProfiles(1, 20, c.Query("search"), c.DefaultQuery("sort_by", "created_at"), c.DefaultQuery("sort_order", "desc"))
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

			// Stock API
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

			// Price API
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

			// Indicator API
			indicatorAPI := protected.Group("/api/indicators")
			{
				indicatorAPI.POST("/calculate", stockCtrl.CalculateAllIndicators)
				indicatorAPI.GET("/summary", stockCtrl.GetIndicatorSummary)
				indicatorAPI.GET("/top-rs", stockCtrl.GetTopRSStocks)
				indicatorAPI.POST("/filter", stockCtrl.FilterStocks)
				indicatorAPI.GET("/:code", stockCtrl.GetStockIndicators)
			}

			// Realtime API
			realtimeAPI := protected.Group("/api/realtime")
			{
				realtimeAPI.GET("/status", stockCtrl.GetRealtimeStatus)
				realtimeAPI.POST("/start", stockCtrl.StartRealtimePolling)
				realtimeAPI.POST("/stop", stockCtrl.StopRealtimePolling)
			}

			// MongoDB API
			mongoAPI := protected.Group("/api/mongodb")
			{
				mongoAPI.GET("/status", stockCtrl.GetMongoDBStatus)
				mongoAPI.POST("/sync-to", stockCtrl.SyncToMongoDB)
				mongoAPI.POST("/restore-from", stockCtrl.RestoreFromMongoDB)
				mongoAPI.POST("/reconnect", stockCtrl.ReconnectMongoDB)
			}

			// Files API
			filesAPI := protected.Group("/api/files")
			{
				filesAPI.GET("/status", stockCtrl.GetFilesStatus)
				filesAPI.GET("/view", stockCtrl.ViewFile)
			}

			protected.POST("/api/toggle-public-api", stockCtrl.TogglePublicAPI)
		}

		adminRoutes.GET("/ws/realtime", stockCtrl.HandleRealtimeWebSocket)
	}

	// Public Signal API
	api := router.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":    "ok",
				"service":   "CPLS Signal API",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		})

		publicSignalController := controllers.NewPublicSignalController()
		publicSignalController.RegisterPublicSignalRoutes(api)
	}

	router.GET("/api/v1/health/db", func(c *gin.Context) {
		if connectionError != "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "message": connectionError})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db_connected": true})
	})

	log.Println("All routes registered")
}

func runMigrations(db *gorm.DB) {
	models.MigrateStockModels(db)
	models.MigrateTradingModels(db)
	models.MigrateSignalConditionModels(db)
	models.MigrateUserModels(db)
	models.MigrateSubscriptionModels(db)
	models.MigrateAdminModels(db)
	models.SeedDefaultAdminUser(db)
	log.Println("Migrations completed")
}

func shutdownServices() {
	if globalScheduler != nil {
		globalScheduler.Stop()
	}
	if services.GlobalStockScheduler != nil {
		services.GlobalStockScheduler.Stop()
	}
	if services.GlobalRealtimeService != nil {
		services.GlobalRealtimeService.Shutdown()
	}
	if services.GlobalMongoClient != nil {
		services.GlobalMongoClient.Close()
	}
}

func corsMiddleware() gin.HandlerFunc {
	corsOrigins := os.Getenv("CORS_ORIGINS")
	allowedOrigins := make(map[string]bool)

	if corsOrigins != "" {
		for _, origin := range strings.Split(corsOrigins, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				allowedOrigins[origin] = true
			}
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		if len(allowedOrigins) == 0 {
			if origin != "" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			}
		} else if allowedOrigins[origin] {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}

		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
