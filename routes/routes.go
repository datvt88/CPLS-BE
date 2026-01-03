package routes

import (
	"log"
	"net/http"
	"os"
	"sync"

	"go_backend_project/admin"
	"go_backend_project/controllers"
	"go_backend_project/middleware"
	"go_backend_project/models"
	"go_backend_project/services/trading"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuthControllerSetter is called to set the global auth controller in main.go
var AuthControllerSetter func(ac *admin.AuthController)

// Global cached auth controllers to avoid re-initialization
var cachedAuthControllers *authControllers

// Track if protected routes have been set up to avoid double-registration
var setupProtectedRoutesOnce sync.Once

// authControllers holds the initialized auth controllers
type authControllers struct {
	authController         *admin.AuthController
	supabaseAuthController *admin.SupabaseAuthController
	useSupabaseAuth        bool
}

// initializeAuthControllers initializes the appropriate auth controller based on configuration
// This function caches the result to avoid redundant initializations and connection tests
func initializeAuthControllers(db *gorm.DB) *authControllers {
	// Return cached controllers if already initialized with same DB state
	if cachedAuthControllers != nil {
		// If we now have a DB but didn't before, need to reinitialize for GORM mode
		if db != nil && !cachedAuthControllers.useSupabaseAuth && cachedAuthControllers.authController == nil {
			// Upgrade to GORM auth now that DB is available
			cachedAuthControllers.authController = admin.NewAuthController(db)
			if AuthControllerSetter != nil {
				AuthControllerSetter(cachedAuthControllers.authController)
			}
			log.Printf("Initialized GORM auth controller with database")
		}
		return cachedAuthControllers
	}

	controllers := &authControllers{}

	// Check if Supabase keys are configured
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseAnonKey := os.Getenv("SUPABASE_ANON_KEY")
	supabaseServiceKey := os.Getenv("SUPABASE_SERVICE_KEY")

	if supabaseURL != "" && (supabaseAnonKey != "" || supabaseServiceKey != "") {
		// Try to create Supabase auth controller
		if sac, err := admin.NewSupabaseAuthController(); err == nil {
			// Test connection to verify admin_users table exists and is accessible
			if err := sac.TestConnection(); err != nil {
				log.Printf("ERROR: Supabase connection test failed: %v", err)
				log.Printf("ERROR: Cannot access admin_users table on Supabase")
				log.Printf("ERROR: Please ensure the admin_users table exists by running migrations/001_admin_users.sql")
				log.Printf("Falling back to GORM-based authentication")
				// Don't use Supabase auth if connection test fails
			} else {
				// Connection test successful - use Supabase auth
				controllers.supabaseAuthController = sac
				controllers.useSupabaseAuth = true
				log.Printf("✓ Using Supabase REST API for admin authentication")
				log.Printf("✓ Supabase connection test successful - admin_users table is accessible")
			}
		} else {
			log.Printf("Warning: Failed to create Supabase auth controller: %v", err)
			log.Printf("Falling back to GORM-based authentication")
		}
	} else {
		log.Printf("Supabase keys not found, will use GORM-based authentication when DB is ready")
	}

	// If not using Supabase auth and DB is available, initialize GORM auth controller
	if !controllers.useSupabaseAuth && db != nil {
		controllers.authController = admin.NewAuthController(db)

		// Set the global auth controller for backward compatibility
		if AuthControllerSetter != nil {
			AuthControllerSetter(controllers.authController)
		}
	}

	// Cache the controllers to avoid re-initialization
	cachedAuthControllers = controllers

	return controllers
}

// SetupAdminRoutes sets up admin authentication routes
// This should be called early, before database initialization, to ensure
// admin login is accessible even if database connection fails
func SetupAdminRoutes(router *gin.Engine, db *gorm.DB) {
	controllers := initializeAuthControllers(db)

	// Admin UI routes - only login/logout (public routes)
	adminRoutes := router.Group("/admin")
	{
		// Root admin path and login routes
		// Use appropriate auth controller based on availability
		if controllers.useSupabaseAuth && controllers.supabaseAuthController != nil {
			// Use Supabase REST API authentication
			adminRoutes.GET("", func(c *gin.Context) {
				// Check if already logged in by checking cookie
				if token, err := c.Cookie("admin_session"); err == nil && token != "" {
					c.Redirect(http.StatusFound, "/admin/dashboard")
				} else {
					c.Redirect(http.StatusFound, "/admin/login")
				}
			})
			adminRoutes.GET("/login", controllers.supabaseAuthController.LoginPage)
			adminRoutes.POST("/login", controllers.supabaseAuthController.Login)

			// Add logout as well since it needs the controller
			adminRoutes.GET("/logout", controllers.supabaseAuthController.Logout)
		} else if controllers.authController != nil {
			// Use GORM-based authentication (only if DB is available)
			adminRoutes.GET("", controllers.authController.RootRedirect)
			adminRoutes.GET("/login", controllers.authController.LoginPage)
			adminRoutes.POST("/login", controllers.authController.Login)

			// Add logout as well since it needs the controller
			adminRoutes.GET("/logout", controllers.authController.Logout)
		} else {
			// No auth controller available - check if Supabase is configured
			supabaseURL := os.Getenv("SUPABASE_URL")
			supabaseConfigured := supabaseURL != ""

			var errorMessage string
			if supabaseConfigured {
				// Supabase is configured but connection failed - likely missing admin_users table
				errorMessage = "Database Error: Cannot access admin_users table on Supabase. " +
					"Please run the migration script (migrations/001_admin_users.sql) in your Supabase SQL Editor. " +
					"See FIX_ADMIN_LOGIN_SERVICE_UNAVAILABLE.md for instructions."
			} else {
				// Neither Supabase nor GORM database is available
				errorMessage = "Database not connected. Please wait for the system to initialize or contact your administrator."
			}

			adminRoutes.GET("", func(c *gin.Context) {
				c.Redirect(http.StatusFound, "/admin/login")
			})
			adminRoutes.GET("/login", func(c *gin.Context) {
				c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
					"error":           errorMessage,
					"supabaseEnabled": false,
				})
			})
			adminRoutes.POST("/login", func(c *gin.Context) {
				c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
					"error":           "Cannot authenticate at this time. " + errorMessage,
					"supabaseEnabled": false,
				})
			})
			adminRoutes.GET("/logout", func(c *gin.Context) {
				c.Redirect(http.StatusFound, "/admin/login")
			})
		}
	}
}

// SetupAdminProtectedRoutesEarly sets up protected admin routes early if Supabase auth is available
// This is called before database initialization. If Supabase is configured and working,
// protected routes will be available immediately. Otherwise, they'll be set up after DB init.
func SetupAdminProtectedRoutesEarly(router *gin.Engine) {
	// Check if Supabase is configured
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseServiceKey := os.Getenv("SUPABASE_SERVICE_KEY")
	
	// Only attempt early setup if Supabase is configured
	if supabaseURL == "" || supabaseServiceKey == "" {
		log.Printf("Deferring admin protected routes setup until database is ready (Supabase not configured)")
		return
	}
	
	// Try to initialize auth controllers
	// Note: initializeAuthControllers uses caching, so calling it multiple times is safe
	controllers := initializeAuthControllers(nil)
	
	// Only setup protected routes early if Supabase auth is actually available
	// This allows dashboard access after Supabase login without waiting for database
	if controllers.useSupabaseAuth && controllers.supabaseAuthController != nil {
		log.Printf("Setting up admin protected routes early (Supabase auth available)")
		SetupAdminProtectedRoutes(router, nil, nil)
	} else {
		log.Printf("Deferring admin protected routes setup until database is ready (Supabase auth test failed)")
	}
}


// SetupAdminProtectedRoutes sets up protected admin routes after database is initialized
// This can be called early with nil db/tradingBot when Supabase auth is available,
// or later after database initialization when using GORM auth.
// The routes will gracefully handle nil database/tradingBot by showing appropriate errors.
func SetupAdminProtectedRoutes(router *gin.Engine, db *gorm.DB, tradingBot *trading.TradingBot) {
	// Use sync.Once to prevent double-registration of routes
	setupProtectedRoutesOnce.Do(func() {
		setupProtectedRoutesImpl(router, db, tradingBot)
	})
}

// setupProtectedRoutesImpl is the actual implementation called by sync.Once
func setupProtectedRoutesImpl(router *gin.Engine, db *gorm.DB, tradingBot *trading.TradingBot) {
	// Initialize admin controller with trading bot (can be nil)
	adminController := admin.NewAdminController(db, tradingBot)

	// Get auth controllers (will be re-initialized with DB if not using Supabase)
	// Note: This uses caching, so even if called before with nil db, it will update
	// the cached controllers with GORM auth if db is now available
	controllers := initializeAuthControllers(db)

	// Determine which auth middleware to use
	var authMiddleware gin.HandlerFunc
	if controllers.useSupabaseAuth && controllers.supabaseAuthController != nil {
		authMiddleware = controllers.supabaseAuthController.AuthMiddleware()
	} else if controllers.authController != nil {
		authMiddleware = controllers.authController.AuthMiddleware()
	} else {
		// Should not happen if SetupAdminProtectedRoutesEarly works correctly
		// Return 503 Service Unavailable to avoid redirect loops
		log.Printf("Warning: Setting up admin protected routes without authentication middleware")
		authMiddleware = func(c *gin.Context) {
			c.HTML(http.StatusServiceUnavailable, "dashboard.html", gin.H{
				"stockCount":    0,
				"strategyCount": 0,
				"backtestCount": 0,
				"tradeCount":    0,
				"userCount":     0,
				"botRunning":    false,
				"adminUser":     nil,
				"page":          "dashboard",
				"title":         "Dashboard",
				"dbError":       "Service temporarily unavailable. Authentication system is initializing.",
			})
			c.Abort()
		}
	}

	// Set up protected routes under /admin path
	// Note: Calling router.Group("/admin") multiple times is safe in Gin - it creates separate
	// route groups that can have different middlewares. Each group is independent.
	adminRoutes := router.Group("/admin")
	protected := adminRoutes.Group("")
	protected.Use(authMiddleware)

	{
		protected.GET("/dashboard", adminController.Dashboard)
		protected.GET("/stocks", adminController.StocksPage)
		protected.GET("/strategies", adminController.StrategiesPage)
		protected.GET("/backtests", adminController.BacktestsPage)
		protected.GET("/trading-bot", adminController.TradingBotPage)
		protected.GET("/signals", adminController.SignalsPage)
		protected.GET("/users", adminController.UsersPage)
		protected.GET("/admin-users", adminController.AdminUsersPage)
		protected.GET("/api-overview", adminController.APIOverviewPage)
		protected.GET("/signal-conditions", adminController.SignalConditionsPage)
		protected.GET("/stock-indicators", adminController.StockIndicatorsPage)
		protected.GET("/stock-indicators/search", adminController.SearchStockIndicators)

		// Signal Conditions Management
		signalConds := protected.Group("/signal-conditions")
		{
			// Condition Groups
			signalConds.POST("/groups", adminController.CreateConditionGroupAction)
			signalConds.PUT("/groups/:id", adminController.UpdateConditionGroupAction)
			signalConds.DELETE("/groups/:id", adminController.DeleteConditionGroupAction)

			// Individual Conditions
			signalConds.POST("/conditions", adminController.AddConditionAction)
			signalConds.PUT("/conditions/:id", adminController.UpdateConditionAction)
			signalConds.DELETE("/conditions/:id", adminController.DeleteConditionAction)

			// Signal Rules
			signalConds.POST("/rules", adminController.CreateSignalRuleAction)
			signalConds.PUT("/rules/:id", adminController.UpdateSignalRuleAction)
			signalConds.DELETE("/rules/:id", adminController.DeleteSignalRuleAction)
			signalConds.GET("/rules/:id/test", adminController.TestSignalRuleAction)
			signalConds.GET("/rules/:id/stats", adminController.GetRuleStatisticsAction)

			// Templates
			signalConds.GET("/templates", adminController.GetTemplatesAction)
			signalConds.GET("/templates/:id/test", adminController.TestTemplateAction)
			signalConds.POST("/templates/from-group", adminController.CreateTemplateFromGroupAction)

			// Testing
			signalConds.GET("/test", adminController.TestStockWithConditionsAction)
		}

		// Admin actions
		actions := protected.Group("/actions")
		{
			actions.POST("/fetch-data", adminController.FetchHistoricalDataAction)
			actions.POST("/create-strategy", adminController.CreateStrategyAction)
			actions.POST("/run-backtest", adminController.RunBacktestAction)
			actions.POST("/start-bot", adminController.StartBotAction)
			actions.POST("/stop-bot", adminController.StopBotAction)
			actions.POST("/initialize-data", adminController.InitializeStockData)
			actions.POST("/create-admin-user", adminController.CreateAdminUserAction)
			actions.POST("/update-user-status", adminController.UpdateUserStatusAction)
			actions.POST("/update-user-role", adminController.UpdateUserRoleAction)
		}
	}
	
	log.Printf("Admin protected routes setup completed")
}

// SetupRoutes sets up all API routes
func SetupRoutes(router *gin.Engine, db *gorm.DB) {
	// Initialize shared trading bot
	tradingBot := trading.NewTradingBot(db)

	// Note: HTML templates are loaded in main.go before this function is called

	// Initialize controllers
	stockController := controllers.NewStockController(db)
	tradingController := controllers.NewTradingController(db)
	userController := controllers.NewUserController(db)
	subscriptionController := controllers.NewSubscriptionController(db)
	screenerController := controllers.NewScreenerController(db)

	// Setup protected admin routes now that DB is ready
	SetupAdminProtectedRoutes(router, db, tradingBot)

	// Check if API auth is required (can be configured via environment)
	requireAPIAuth := os.Getenv("REQUIRE_API_AUTH") == "true"

	// API v1 group
	api := router.Group("/api/v1")

	// Apply optional JWT middleware to allow both authenticated and anonymous access
	// If REQUIRE_API_AUTH is true, use strict JWT middleware
	if requireAPIAuth {
		api.Use(middleware.JWTAuthMiddleware())
	} else {
		api.Use(middleware.OptionalJWTAuthMiddleware())
	}

	{
		// Database health check endpoint (always public)
		api.GET("/health/db", func(c *gin.Context) {
			sqlDB, err := db.DB()
			if err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status":  "error",
					"message": "Failed to get database connection",
				})
				return
			}

			if err := sqlDB.Ping(); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status":  "error",
					"message": "Database connection failed",
				})
				return
			}

			// Check if admin_users table exists and has admin users
			var adminCount int64
			db.Model(&models.AdminUser{}).Count(&adminCount)

			c.JSON(http.StatusOK, gin.H{
				"status":       "ok",
				"message":      "Database connection successful",
				"admin_users":  adminCount,
				"db_connected": true,
			})
		})

		// Auth info endpoint - returns current authentication status
		api.GET("/auth/me", func(c *gin.Context) {
			authenticated, exists := c.Get("authenticated")
			if !exists || !authenticated.(bool) {
				c.JSON(http.StatusOK, gin.H{
					"authenticated": false,
					"message":       "Not authenticated",
				})
				return
			}

			userID, _ := c.Get("user_id")
			email, _ := c.Get("user_email")
			role, _ := c.Get("user_role")

			c.JSON(http.StatusOK, gin.H{
				"authenticated": true,
				"user_id":       userID,
				"email":         email,
				"role":          role,
			})
		})

		// User routes
		users := api.Group("/users")
		{
			users.GET("", userController.GetUsers)
			users.GET("/:id", userController.GetUser)
			users.POST("", userController.CreateUser)
			users.PUT("/:id", userController.UpdateUser)
			users.DELETE("/:id", userController.DeleteUser)
			users.POST("/:id/login", userController.UpdateLastLogin)
			users.POST("/sync", userController.SyncFromSupabase)

			// Watchlist
			users.GET("/:id/watchlist", userController.GetUserWatchlist)
			users.POST("/:id/watchlist", userController.AddToWatchlist)
			users.DELETE("/:id/watchlist/:stock_id", userController.RemoveFromWatchlist)

			// Alerts
			users.GET("/:id/alerts", userController.GetUserAlerts)
			users.POST("/:id/alerts", userController.CreateUserAlert)
			users.DELETE("/:id/alerts/:alert_id", userController.DeleteUserAlert)
		}

		// Subscription routes
		subscriptions := api.Group("/subscriptions")
		{
			subscriptions.GET("/plans", subscriptionController.GetPlans)
			subscriptions.GET("/plans/:id", subscriptionController.GetPlan)
			subscriptions.POST("/plans", subscriptionController.CreatePlan)
			subscriptions.GET("/user/:user_id", subscriptionController.GetUserSubscription)
			subscriptions.POST("/subscribe", subscriptionController.Subscribe)
			subscriptions.POST("/cancel", subscriptionController.CancelSubscription)
			subscriptions.GET("/payments/:user_id", subscriptionController.GetPaymentHistory)
		}

		// Stock routes
		stocks := api.Group("/stocks")
		{
			stocks.GET("", stockController.GetStocks)
			stocks.GET("/search", stockController.SearchStocks)
			stocks.GET("/:id", stockController.GetStock)
			stocks.GET("/:symbol/prices", stockController.GetStockPrice)
			stocks.GET("/:symbol/quote", stockController.GetRealtimeQuote)
			stocks.GET("/:symbol/indicators", stockController.GetTechnicalIndicators)
			stocks.POST("/:symbol/indicators/calculate", stockController.CalculateIndicators)
			stocks.POST("/:symbol/fetch-historical", stockController.FetchHistoricalData)
		}

		// Stock Screener routes
		screener := api.Group("/screener")
		{
			screener.POST("/screen", screenerController.Screen)
			screener.GET("/presets", screenerController.GetPresets)
			screener.GET("/presets/:id", screenerController.RunPreset)
			screener.GET("/top-gainers", screenerController.GetTopGainers)
			screener.GET("/top-losers", screenerController.GetTopLosers)
			screener.GET("/most-active", screenerController.GetMostActive)
			screener.GET("/oversold", screenerController.GetOversoldStocks)
			screener.GET("/overbought", screenerController.GetOverboughtStocks)
			screener.GET("/bullish", screenerController.GetBullishStocks)
			screener.GET("/volume-spike", screenerController.GetVolumeSpike)
		}

		// Market routes
		market := api.Group("/market")
		{
			market.GET("/indices", stockController.GetMarketIndices)
			market.GET("/top-gainers", stockController.GetTopGainers)
			market.GET("/top-losers", stockController.GetTopLosers)
			market.GET("/most-active", stockController.GetMostActive)
		}

		// Trading strategy routes
		strategies := api.Group("/strategies")
		{
			strategies.GET("", tradingController.GetStrategies)
			strategies.POST("", tradingController.CreateStrategy)
			strategies.PUT("/:id", tradingController.UpdateStrategy)
			strategies.DELETE("/:id", tradingController.DeleteStrategy)
		}

		// Backtest routes
		backtests := api.Group("/backtests")
		{
			backtests.GET("", tradingController.GetBacktests)
			backtests.GET("/:id", tradingController.GetBacktest)
			backtests.POST("", tradingController.RunBacktest)
		}

		// Signal routes - using new SignalController with algorithmic trading strategies
		controllers.RegisterSignalRoutes(api)

		// Public Signal API routes - optimized for frontend consumption
		publicSignalController := controllers.NewPublicSignalController()
		publicSignalController.RegisterPublicSignalRoutes(api)

		// Trading routes
		trading := api.Group("/trading")
		{
			// Bot control
			trading.POST("/bot/start", tradingController.StartTradingBot)
			trading.POST("/bot/stop", tradingController.StopTradingBot)
			trading.GET("/bot/status", tradingController.GetTradingBotStatus)

			// Manual trading
			trading.POST("/manual", tradingController.ExecuteManualTrade)

			// Trade history
			trading.GET("/trades", tradingController.GetTrades)

			// Portfolio
			trading.GET("/portfolio", tradingController.GetPortfolio)
		}
	}
}
