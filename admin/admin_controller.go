package admin

import (
	"net/http"
	"strconv"
	"time"

	"go_backend_project/models"
	"go_backend_project/services/backtesting"
	"go_backend_project/services/datafetcher"
	"go_backend_project/services/trading"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// AdminController handles admin UI requests
type AdminController struct {
	db             *gorm.DB
	dataFetcher    *datafetcher.DataFetcher
	backtestEngine *backtesting.BacktestEngine
	tradingBot     *trading.TradingBot
}

// NewAdminController creates a new admin controller
func NewAdminController(db *gorm.DB, tradingBot *trading.TradingBot) *AdminController {
	return &AdminController{
		db:             db,
		dataFetcher:    datafetcher.NewDataFetcher(db),
		backtestEngine: backtesting.NewBacktestEngine(db),
		tradingBot:     tradingBot,
	}
}

// Dashboard shows admin dashboard
func (ac *AdminController) Dashboard(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	// Get statistics
	var stockCount int64
	ac.db.Model(&models.Stock{}).Count(&stockCount)

	var strategyCount int64
	ac.db.Model(&models.TradingStrategy{}).Count(&strategyCount)

	var backtestCount int64
	ac.db.Model(&models.Backtest{}).Count(&backtestCount)

	var tradeCount int64
	ac.db.Model(&models.Trade{}).Count(&tradeCount)

	var userCount int64
	ac.db.Model(&models.User{}).Count(&userCount)

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"stockCount":    stockCount,
		"strategyCount": strategyCount,
		"backtestCount": backtestCount,
		"tradeCount":    tradeCount,
		"userCount":     userCount,
		"botRunning":    ac.tradingBot.IsRunning(),
		"adminUser":     adminUser,
		"page":          "dashboard",
		"title":         "Dashboard",
	})
}

// getAdminUser retrieves the admin user from context
func (ac *AdminController) getAdminUser(c *gin.Context) *models.AdminUser {
	if user, exists := c.Get("admin_user"); exists {
		if adminUser, ok := user.(models.AdminUser); ok {
			return &adminUser
		}
	}
	return nil
}

// StocksPage shows stocks management page
func (ac *AdminController) StocksPage(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	var stocks []models.Stock
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit := 50
	offset := (page - 1) * limit

	var total int64
	ac.db.Model(&models.Stock{}).Count(&total)

	ac.db.Limit(limit).Offset(offset).Find(&stocks)

	c.HTML(http.StatusOK, "stocks.html", gin.H{
		"stocks":    stocks,
		"page":      page,
		"total":     total,
		"adminUser": adminUser,
		"title":     "Stocks",
	})
}

// StrategiesPage shows strategies management page
func (ac *AdminController) StrategiesPage(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	var strategies []models.TradingStrategy
	ac.db.Find(&strategies)

	c.HTML(http.StatusOK, "strategies.html", gin.H{
		"strategies": strategies,
		"adminUser":  adminUser,
		"page":       "strategies",
		"title":      "Strategies",
	})
}

// BacktestsPage shows backtests page
func (ac *AdminController) BacktestsPage(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	var backtests []models.Backtest
	ac.db.Preload("Strategy").Order("created_at DESC").Limit(50).Find(&backtests)

	c.HTML(http.StatusOK, "backtests.html", gin.H{
		"backtests": backtests,
		"adminUser": adminUser,
		"page":      "backtests",
		"title":     "Backtests",
	})
}

// TradingBotPage shows trading bot control page
func (ac *AdminController) TradingBotPage(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	var signals []models.Signal
	ac.db.Preload("Stock").Preload("Strategy").
		Where("is_active = ?", true).
		Order("created_at DESC").
		Limit(20).
		Find(&signals)

	var trades []models.Trade
	ac.db.Preload("Stock").Preload("Strategy").
		Order("created_at DESC").
		Limit(20).
		Find(&trades)

	c.HTML(http.StatusOK, "trading_bot.html", gin.H{
		"botRunning": ac.tradingBot.IsRunning(),
		"signals":    signals,
		"trades":     trades,
		"adminUser":  adminUser,
		"page":       "bot",
		"title":      "Trading Bot",
	})
}

// SignalsPage shows trading signals management page
func (ac *AdminController) SignalsPage(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	c.HTML(http.StatusOK, "signals.html", gin.H{
		"adminUser":  adminUser,
		"botRunning": ac.tradingBot.IsRunning(),
		"page":       "signals",
		"title":      "Trading Signals",
	})
}

// FetchHistoricalDataAction fetches historical data
func (ac *AdminController) FetchHistoricalDataAction(c *gin.Context) {
	symbol := c.PostForm("symbol")
	startDate, _ := time.Parse("2006-01-02", c.PostForm("start_date"))
	endDate, _ := time.Parse("2006-01-02", c.PostForm("end_date"))

	err := ac.dataFetcher.FetchHistoricalData(symbol, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Data fetched successfully"})
}

// CreateStrategyAction creates a new strategy
func (ac *AdminController) CreateStrategyAction(c *gin.Context) {
	var strategy models.TradingStrategy
	if err := c.ShouldBind(&strategy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ac.db.Create(&strategy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy created", "id": strategy.ID})
}

// RunBacktestAction runs a backtest
func (ac *AdminController) RunBacktestAction(c *gin.Context) {
	strategyID, _ := strconv.ParseUint(c.PostForm("strategy_id"), 10, 32)
	startDate, _ := time.Parse("2006-01-02", c.PostForm("start_date"))
	endDate, _ := time.Parse("2006-01-02", c.PostForm("end_date"))
	initialCapital, _ := strconv.ParseFloat(c.PostForm("initial_capital"), 64)

	symbols := c.PostFormArray("symbols[]")
	if len(symbols) == 0 {
		symbols = []string{"VNM", "VIC", "HPG"}
	}

	config := &backtesting.BacktestConfig{
		StrategyID:     uint(strategyID),
		StartDate:      startDate,
		EndDate:        endDate,
		InitialCapital: decimal.NewFromFloat(initialCapital),
		Commission:     decimal.NewFromFloat(0.0015),
		Symbols:        symbols,
		RiskPerTrade:   decimal.NewFromFloat(0.02),
	}

	backtest, err := ac.backtestEngine.RunBacktest(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Backtest completed",
		"backtest_id": backtest.ID,
		"total_return": backtest.TotalReturn,
		"win_rate": backtest.WinRate,
	})
}

// StartBotAction starts the trading bot
func (ac *AdminController) StartBotAction(c *gin.Context) {
	if err := ac.tradingBot.Start(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Bot started"})
}

// StopBotAction stops the trading bot
func (ac *AdminController) StopBotAction(c *gin.Context) {
	ac.tradingBot.Stop()
	c.JSON(http.StatusOK, gin.H{"message": "Bot stopped"})
}

// InitializeStockData initializes sample stock data
func (ac *AdminController) InitializeStockData(c *gin.Context) {
	if err := ac.dataFetcher.FetchStockList(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch sample historical data for top stocks
	symbols := []string{"VNM", "VIC", "HPG", "VCB", "TCB"}
	startDate := time.Now().AddDate(0, -6, 0) // 6 months ago
	endDate := time.Now()

	for _, symbol := range symbols {
		go ac.dataFetcher.FetchHistoricalData(symbol, startDate, endDate)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Stock data initialization started"})
}

// UsersPage shows users management page (Supabase users)
func (ac *AdminController) UsersPage(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	var users []models.User
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit := 20
	offset := (page - 1) * limit

	var total int64
	ac.db.Model(&models.User{}).Count(&total)

	ac.db.Limit(limit).Offset(offset).Order("created_at DESC").Find(&users)

	c.HTML(http.StatusOK, "users.html", gin.H{
		"users":     users,
		"page":      page,
		"total":     total,
		"limit":     limit,
		"adminUser": adminUser,
		"title":     "Users",
	})
}

// AdminUsersPage shows admin users management page
func (ac *AdminController) AdminUsersPage(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	var adminUsers []models.AdminUser
	ac.db.Order("created_at DESC").Find(&adminUsers)

	c.HTML(http.StatusOK, "admin_users.html", gin.H{
		"adminUsers":    adminUsers,
		"currentAdmin":  adminUser,
		"adminUser":     adminUser,
		"page":          "admin_users",
		"title":         "Admin Users",
	})
}

// APIOverviewPage shows API management overview page
func (ac *AdminController) APIOverviewPage(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	// Get API statistics
	var userCount int64
	ac.db.Model(&models.User{}).Count(&userCount)

	var stockCount int64
	ac.db.Model(&models.Stock{}).Count(&stockCount)

	var subscriptionCount int64
	ac.db.Model(&models.Subscription{}).Where("status = ?", "active").Count(&subscriptionCount)

	c.HTML(http.StatusOK, "api_overview.html", gin.H{
		"userCount":         userCount,
		"stockCount":        stockCount,
		"subscriptionCount": subscriptionCount,
		"adminUser":         adminUser,
		"page":              "api",
		"title":             "API Overview",
	})
}

// CreateAdminUserAction creates a new admin user
func (ac *AdminController) CreateAdminUserAction(c *gin.Context) {
	var request struct {
		Username string `form:"username" binding:"required"`
		Password string `form:"password" binding:"required"`
		Email    string `form:"email"`
		FullName string `form:"full_name"`
		Role     string `form:"role"`
	}

	if err := c.ShouldBind(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if username exists
	var existing models.AdminUser
	if err := ac.db.Where("username = ?", request.Username).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Username already exists"})
		return
	}

	adminUser := &models.AdminUser{
		Username: request.Username,
		Email:    request.Email,
		FullName: request.FullName,
		Role:     request.Role,
		IsActive: true,
	}

	if request.Role == "" {
		adminUser.Role = "admin"
	}

	if err := adminUser.SetPassword(request.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	if err := ac.db.Create(adminUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create admin user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Admin user created successfully", "id": adminUser.ID})
}

// UpdateUserStatusAction updates user active status
func (ac *AdminController) UpdateUserStatusAction(c *gin.Context) {
	userID := c.PostForm("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	// Validate userID is a valid number
	if _, err := strconv.ParseUint(userID, 10, 32); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}

	isActive := c.PostForm("is_active") == "true"

	if err := ac.db.Model(&models.User{}).Where("id = ?", userID).Update("is_active", isActive).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User status updated"})
}

// UpdateUserRoleAction updates user role
func (ac *AdminController) UpdateUserRoleAction(c *gin.Context) {
	userID := c.PostForm("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	// Validate userID is a valid number
	if _, err := strconv.ParseUint(userID, 10, 32); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}

	role := c.PostForm("role")
	if role == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role is required"})
		return
	}

	// Validate role is a valid value
	validRoles := map[string]bool{"user": true, "premium": true, "admin": true}
	if !validRoles[role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role. Must be user, premium, or admin"})
		return
	}

	if err := ac.db.Model(&models.User{}).Where("id = ?", userID).Update("role", role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User role updated"})
}
