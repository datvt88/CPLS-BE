package routes

import (
	"go_backend_project/admin"
	"go_backend_project/controllers"
	"go_backend_project/services/trading"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupRoutes sets up all API routes
func SetupRoutes(router *gin.Engine, db *gorm.DB) {
	// Initialize shared trading bot
	tradingBot := trading.NewTradingBot(db)

	// Load HTML templates
	router.LoadHTMLGlob("admin/templates/*.html")

	// Initialize controllers
	stockController := controllers.NewStockController(db)
	tradingController := controllers.NewTradingController(db)
	userController := controllers.NewUserController(db)
	subscriptionController := controllers.NewSubscriptionController(db)
	screenerController := controllers.NewScreenerController(db)

	// Initialize admin controller
	adminController := admin.NewAdminController(db, tradingBot)

	// API v1 group
	api := router.Group("/api/v1")
	{
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

		// Signal routes
		signals := api.Group("/signals")
		{
			signals.GET("", tradingController.GetSignals)
		}

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
	adminRoutes := router.Group("/admin")
	{
		adminRoutes.GET("", adminController.Dashboard)
		adminRoutes.GET("/stocks", adminController.StocksPage)
		adminRoutes.GET("/strategies", adminController.StrategiesPage)
		adminRoutes.GET("/backtests", adminController.BacktestsPage)
		adminRoutes.GET("/trading-bot", adminController.TradingBotPage)

		// Admin actions
		actions := adminRoutes.Group("/actions")
		{
			actions.POST("/fetch-data", adminController.FetchHistoricalDataAction)
			actions.POST("/create-strategy", adminController.CreateStrategyAction)
			actions.POST("/run-backtest", adminController.RunBacktestAction)
			actions.POST("/start-bot", adminController.StartBotAction)
			actions.POST("/stop-bot", adminController.StopBotAction)
			actions.POST("/initialize-data", adminController.InitializeStockData)
		}
	}

}