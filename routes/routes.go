package routes

import (
	"net/http"
	"os"

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

	// Initialize admin controllers
	adminController := admin.NewAdminController(db, tradingBot)
	authController := admin.NewAuthController(db)

	// Set the global auth controller so main.go's login handlers can use it
	if AuthControllerSetter != nil {
		AuthControllerSetter(authController)
	}

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

	// Admin UI routes
	// Note: /admin/login routes are registered in main.go's setupInitialAdminRoutes
	// to ensure they're available immediately when server starts
	adminRoutes := router.Group("/admin")
	{
		// Protected routes (auth required)
		protected := adminRoutes.Group("")
		protected.Use(authController.AuthMiddleware())
		{
			protected.GET("", adminController.Dashboard)
			protected.GET("/logout", authController.Logout)
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
	}

}