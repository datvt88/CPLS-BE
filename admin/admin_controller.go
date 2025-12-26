package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"go_backend_project/models"
	"go_backend_project/services"
	"go_backend_project/services/backtesting"
	"go_backend_project/services/datafetcher"
	"go_backend_project/services/signals"
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

// =============================================================================
// SIGNAL CONDITIONS MANAGEMENT
// =============================================================================

// SignalConditionsPage shows signal conditions management page
func (ac *AdminController) SignalConditionsPage(c *gin.Context) {
	adminUser := ac.getAdminUser(c)

	// Get all condition groups
	var groups []models.SignalConditionGroup
	ac.db.Preload("Conditions").Order("priority DESC, name ASC").Find(&groups)

	// Get all templates
	var templates []models.SignalTemplate
	ac.db.Order("category ASC, name ASC").Find(&templates)

	// Get all rules
	var rules []models.SignalRule
	ac.db.Order("priority DESC, name ASC").Find(&rules)

	// Get indicator types for dropdown
	indicatorTypes := []map[string]string{
		{"value": "RSI", "label": "RSI (14)", "category": "Oscillators"},
		{"value": "MACD", "label": "MACD Line", "category": "Oscillators"},
		{"value": "MACD_SIGNAL", "label": "MACD Signal", "category": "Oscillators"},
		{"value": "MACD_HISTOGRAM", "label": "MACD Histogram", "category": "Oscillators"},
		{"value": "MA10", "label": "MA 10", "category": "Moving Averages"},
		{"value": "MA30", "label": "MA 30", "category": "Moving Averages"},
		{"value": "MA50", "label": "MA 50", "category": "Moving Averages"},
		{"value": "MA200", "label": "MA 200", "category": "Moving Averages"},
		{"value": "RS_3D", "label": "RS 3 Day", "category": "Relative Strength"},
		{"value": "RS_1M", "label": "RS 1 Month", "category": "Relative Strength"},
		{"value": "RS_3M", "label": "RS 3 Month", "category": "Relative Strength"},
		{"value": "RS_1Y", "label": "RS 1 Year", "category": "Relative Strength"},
		{"value": "RS_AVG", "label": "RS Average", "category": "Relative Strength"},
		{"value": "VOLUME", "label": "Volume", "category": "Volume"},
		{"value": "VOL_RATIO", "label": "Volume Ratio", "category": "Volume"},
		{"value": "PRICE", "label": "Price", "category": "Price"},
		{"value": "PRICE_CHANGE", "label": "Price Change %", "category": "Price"},
		{"value": "TRADING_VALUE", "label": "Trading Value (Ty)", "category": "Volume"},
	}

	operators := []map[string]string{
		{"value": "eq", "label": "= (Equal)"},
		{"value": "neq", "label": "!= (Not Equal)"},
		{"value": "gt", "label": "> (Greater Than)"},
		{"value": "gte", "label": ">= (Greater or Equal)"},
		{"value": "lt", "label": "< (Less Than)"},
		{"value": "lte", "label": "<= (Less or Equal)"},
		{"value": "between", "label": "Between"},
		{"value": "cross_above", "label": "Crosses Above"},
		{"value": "cross_below", "label": "Crosses Below"},
	}

	c.HTML(http.StatusOK, "signal_conditions.html", gin.H{
		"adminUser":      adminUser,
		"page":           "signal_conditions",
		"title":          "Signal Conditions",
		"groups":         groups,
		"templates":      templates,
		"rules":          rules,
		"indicatorTypes": indicatorTypes,
		"operators":      operators,
	})
}

// CreateConditionGroupAction creates a new condition group
func (ac *AdminController) CreateConditionGroupAction(c *gin.Context) {
	var request struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		SignalType  string `json:"signal_type"`
		Priority    int    `json:"priority"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminUser := ac.getAdminUser(c)
	createdBy := uint(0)
	if adminUser != nil {
		createdBy = adminUser.ID
	}

	group := &models.SignalConditionGroup{
		Name:        request.Name,
		Description: request.Description,
		SignalType:  request.SignalType,
		Priority:    request.Priority,
		IsActive:    true,
		CreatedBy:   createdBy,
	}

	if err := ac.db.Create(group).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create condition group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Condition group created", "id": group.ID})
}

// UpdateConditionGroupAction updates a condition group
func (ac *AdminController) UpdateConditionGroupAction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var request struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		SignalType  string `json:"signal_type"`
		Priority    int    `json:"priority"`
		IsActive    bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{
		"name":        request.Name,
		"description": request.Description,
		"signal_type": request.SignalType,
		"priority":    request.Priority,
		"is_active":   request.IsActive,
	}

	if err := ac.db.Model(&models.SignalConditionGroup{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update condition group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Condition group updated"})
}

// DeleteConditionGroupAction deletes a condition group
func (ac *AdminController) DeleteConditionGroupAction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	// Delete conditions first
	ac.db.Where("group_id = ?", id).Delete(&models.SignalCondition{})

	if err := ac.db.Delete(&models.SignalConditionGroup{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete condition group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Condition group deleted"})
}

// AddConditionAction adds a condition to a group
func (ac *AdminController) AddConditionAction(c *gin.Context) {
	var request struct {
		GroupID          uint    `json:"group_id" binding:"required"`
		Name             string  `json:"name"`
		Indicator        string  `json:"indicator" binding:"required"`
		Operator         string  `json:"operator" binding:"required"`
		Value            float64 `json:"value"`
		Value2           float64 `json:"value2"`
		CompareIndicator string  `json:"compare_indicator"`
		LogicalOperator  string  `json:"logical_operator"`
		Weight           int     `json:"weight"`
		IsRequired       bool    `json:"is_required"`
		Description      string  `json:"description"`
		OrderIndex       int     `json:"order_index"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if request.LogicalOperator == "" {
		request.LogicalOperator = "AND"
	}
	if request.Weight == 0 {
		request.Weight = 1
	}

	condition := &models.SignalCondition{
		GroupID:          request.GroupID,
		Name:             request.Name,
		Indicator:        models.IndicatorType(request.Indicator),
		Operator:         models.ConditionOperator(request.Operator),
		Value:            decimal.NewFromFloat(request.Value),
		Value2:           decimal.NewFromFloat(request.Value2),
		CompareIndicator: models.IndicatorType(request.CompareIndicator),
		LogicalOperator:  models.LogicalOperator(request.LogicalOperator),
		Weight:           request.Weight,
		IsRequired:       request.IsRequired,
		Description:      request.Description,
		OrderIndex:       request.OrderIndex,
	}

	if err := ac.db.Create(condition).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add condition"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Condition added", "id": condition.ID})
}

// UpdateConditionAction updates a condition
func (ac *AdminController) UpdateConditionAction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var request struct {
		Name             string  `json:"name"`
		Indicator        string  `json:"indicator"`
		Operator         string  `json:"operator"`
		Value            float64 `json:"value"`
		Value2           float64 `json:"value2"`
		CompareIndicator string  `json:"compare_indicator"`
		LogicalOperator  string  `json:"logical_operator"`
		Weight           int     `json:"weight"`
		IsRequired       bool    `json:"is_required"`
		Description      string  `json:"description"`
		OrderIndex       int     `json:"order_index"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{
		"name":              request.Name,
		"indicator":         request.Indicator,
		"operator":          request.Operator,
		"value":             decimal.NewFromFloat(request.Value),
		"value2":            decimal.NewFromFloat(request.Value2),
		"compare_indicator": request.CompareIndicator,
		"logical_operator":  request.LogicalOperator,
		"weight":            request.Weight,
		"is_required":       request.IsRequired,
		"description":       request.Description,
		"order_index":       request.OrderIndex,
	}

	if err := ac.db.Model(&models.SignalCondition{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update condition"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Condition updated"})
}

// DeleteConditionAction deletes a condition
func (ac *AdminController) DeleteConditionAction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if err := ac.db.Delete(&models.SignalCondition{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete condition"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Condition deleted"})
}

// CreateSignalRuleAction creates a new signal rule
func (ac *AdminController) CreateSignalRuleAction(c *gin.Context) {
	var request struct {
		Name            string   `json:"name" binding:"required"`
		Description     string   `json:"description"`
		SignalType      string   `json:"signal_type" binding:"required"`
		StrategyType    string   `json:"strategy_type"`
		MinScore        int      `json:"min_score"`
		TargetPercent   float64  `json:"target_percent"`
		StopLossPercent float64  `json:"stop_loss_percent"`
		Priority        int      `json:"priority"`
		GroupIDs        []uint   `json:"group_ids"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if request.MinScore == 0 {
		request.MinScore = 60
	}
	if request.TargetPercent == 0 {
		request.TargetPercent = 10
	}
	if request.StopLossPercent == 0 {
		request.StopLossPercent = 5
	}

	// Build condition groups JSON
	var groupConfigs []map[string]interface{}
	for i, gid := range request.GroupIDs {
		groupConfigs = append(groupConfigs, map[string]interface{}{
			"group_id": gid,
			"logic":    "AND",
			"required": i == 0, // First group is required
		})
	}
	groupsJSON, _ := json.Marshal(groupConfigs)

	adminUser := ac.getAdminUser(c)
	createdBy := uint(0)
	if adminUser != nil {
		createdBy = adminUser.ID
	}

	rule := &models.SignalRule{
		Name:            request.Name,
		Description:     request.Description,
		SignalType:      request.SignalType,
		StrategyType:    request.StrategyType,
		MinScore:        request.MinScore,
		TargetPercent:   decimal.NewFromFloat(request.TargetPercent),
		StopLossPercent: decimal.NewFromFloat(request.StopLossPercent),
		Priority:        request.Priority,
		ConditionGroups: string(groupsJSON),
		IsActive:        true,
		CreatedBy:       createdBy,
	}

	if err := ac.db.Create(rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create signal rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Signal rule created", "id": rule.ID})
}

// UpdateSignalRuleAction updates a signal rule
func (ac *AdminController) UpdateSignalRuleAction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var request struct {
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		SignalType      string   `json:"signal_type"`
		StrategyType    string   `json:"strategy_type"`
		MinScore        int      `json:"min_score"`
		TargetPercent   float64  `json:"target_percent"`
		StopLossPercent float64  `json:"stop_loss_percent"`
		Priority        int      `json:"priority"`
		IsActive        bool     `json:"is_active"`
		GroupIDs        []uint   `json:"group_ids"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var groupsJSON []byte
	if len(request.GroupIDs) > 0 {
		var groupConfigs []map[string]interface{}
		for i, gid := range request.GroupIDs {
			groupConfigs = append(groupConfigs, map[string]interface{}{
				"group_id": gid,
				"logic":    "AND",
				"required": i == 0,
			})
		}
		groupsJSON, _ = json.Marshal(groupConfigs)
	}

	updates := map[string]interface{}{
		"name":              request.Name,
		"description":       request.Description,
		"signal_type":       request.SignalType,
		"strategy_type":     request.StrategyType,
		"min_score":         request.MinScore,
		"target_percent":    decimal.NewFromFloat(request.TargetPercent),
		"stop_loss_percent": decimal.NewFromFloat(request.StopLossPercent),
		"priority":          request.Priority,
		"is_active":         request.IsActive,
	}

	if len(groupsJSON) > 0 {
		updates["condition_groups"] = string(groupsJSON)
	}

	if err := ac.db.Model(&models.SignalRule{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update signal rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Signal rule updated"})
}

// DeleteSignalRuleAction deletes a signal rule
func (ac *AdminController) DeleteSignalRuleAction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if err := ac.db.Delete(&models.SignalRule{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete signal rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Signal rule deleted"})
}

// TestSignalRuleAction tests a signal rule against current stock data
func (ac *AdminController) TestSignalRuleAction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)

	minTradingValStr := c.DefaultQuery("min_trading_val", "1")
	minTradingVal, _ := strconv.ParseFloat(minTradingValStr, 64)

	if signals.GlobalConditionEvaluator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Condition evaluator not initialized"})
		return
	}

	results, err := signals.GlobalConditionEvaluator.ScreenStocksWithRule(uint(id), minTradingVal, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Convert results to JSON-friendly format
	var signalsOut []map[string]interface{}
	for _, sig := range results {
		signalsOut = append(signalsOut, map[string]interface{}{
			"code":         sig.StockCode,
			"signal_type":  sig.SignalType,
			"score":        sig.Score,
			"max_score":    sig.MaxScore,
			"confidence":   sig.Confidence,
			"price":        sig.Price,
			"target_price": sig.TargetPrice,
			"stop_loss":    sig.StopLoss,
			"reasons":      sig.Reasons,
			"indicators":   sig.Indicators,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"signals": signalsOut,
		"count":   len(signalsOut),
	})
}

// TestTemplateAction tests a signal template against current stock data
func (ac *AdminController) TestTemplateAction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)

	minTradingValStr := c.DefaultQuery("min_trading_val", "1")
	minTradingVal, _ := strconv.ParseFloat(minTradingValStr, 64)

	if signals.GlobalConditionEvaluator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Condition evaluator not initialized"})
		return
	}

	results, err := signals.GlobalConditionEvaluator.ScreenStocksWithTemplate(uint(id), minTradingVal, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var signalsOut []map[string]interface{}
	for _, sig := range results {
		signalsOut = append(signalsOut, map[string]interface{}{
			"code":         sig.StockCode,
			"signal_type":  sig.SignalType,
			"score":        sig.Score,
			"max_score":    sig.MaxScore,
			"confidence":   sig.Confidence,
			"price":        sig.Price,
			"target_price": sig.TargetPrice,
			"stop_loss":    sig.StopLoss,
			"reasons":      sig.Reasons,
			"indicators":   sig.Indicators,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"signals": signalsOut,
		"count":   len(signalsOut),
	})
}

// GetRuleStatisticsAction returns performance statistics for a rule
func (ac *AdminController) GetRuleStatisticsAction(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if signals.GlobalConditionEvaluator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Condition evaluator not initialized"})
		return
	}

	stats, err := signals.GlobalConditionEvaluator.GetRuleStatistics(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// TestStockWithConditionsAction tests a specific stock against a condition group or rule
func (ac *AdminController) TestStockWithConditionsAction(c *gin.Context) {
	stockCode := c.Query("stock")
	groupIDStr := c.Query("group_id")
	ruleIDStr := c.Query("rule_id")

	if stockCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Stock code is required"})
		return
	}

	// Get stock indicators
	indicators, err := services.GlobalIndicatorService.GetStockIndicators(stockCode)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Stock indicators not found"})
		return
	}

	if signals.GlobalConditionEvaluator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Condition evaluator not initialized"})
		return
	}

	result := gin.H{
		"stock":      stockCode,
		"price":      indicators.CurrentPrice,
		"indicators": map[string]interface{}{
			"rsi":              indicators.RSI,
			"macd":             indicators.MACD,
			"macd_signal":      indicators.MACDSignal,
			"macd_hist":        indicators.MACDHist,
			"ma10":             indicators.MA10,
			"ma30":             indicators.MA30,
			"ma50":             indicators.MA50,
			"ma200":            indicators.MA200,
			"rs_3d":            indicators.RS3DRank,
			"rs_1m":            indicators.RS1MRank,
			"rs_3m":            indicators.RS3MRank,
			"rs_1y":            indicators.RS1YRank,
			"rs_avg":           indicators.RSAvg,
			"vol_ratio":        indicators.VolRatio,
			"avg_trading_val":  indicators.AvgTradingVal,
			"ma10_above_ma30":  indicators.MA10AboveMA30,
			"ma50_above_ma200": indicators.MA50AboveMA200,
		},
	}

	// Test against group if specified
	if groupIDStr != "" {
		groupID, _ := strconv.ParseUint(groupIDStr, 10, 32)
		var group models.SignalConditionGroup
		if err := ac.db.Preload("Conditions").First(&group, groupID).Error; err == nil {
			groupResult := signals.GlobalConditionEvaluator.EvaluateConditionGroup(&group, indicators)
			result["group_result"] = map[string]interface{}{
				"passed":      groupResult.Passed,
				"total_score": groupResult.TotalScore,
				"max_score":   groupResult.MaxScore,
				"conditions":  groupResult.Results,
			}
		}
	}

	// Test against rule if specified
	if ruleIDStr != "" {
		ruleID, _ := strconv.ParseUint(ruleIDStr, 10, 32)
		var rule models.SignalRule
		if err := ac.db.First(&rule, ruleID).Error; err == nil {
			signal, err := signals.GlobalConditionEvaluator.EvaluateRule(&rule, indicators)
			if err == nil && signal != nil {
				result["rule_signal"] = map[string]interface{}{
					"signal_type":  signal.SignalType,
					"score":        signal.Score,
					"max_score":    signal.MaxScore,
					"confidence":   signal.Confidence,
					"target_price": signal.TargetPrice,
					"stop_loss":    signal.StopLoss,
					"reasons":      signal.Reasons,
				}
			} else {
				result["rule_signal"] = nil
				result["rule_message"] = "No signal triggered"
			}
		}
	}

	c.JSON(http.StatusOK, result)
}

// GetTemplatesAction returns all signal templates
func (ac *AdminController) GetTemplatesAction(c *gin.Context) {
	var templates []models.SignalTemplate
	ac.db.Order("category ASC, popularity DESC, name ASC").Find(&templates)

	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

// CreateTemplateFromGroupAction creates a template from a condition group
func (ac *AdminController) CreateTemplateFromGroupAction(c *gin.Context) {
	var request struct {
		GroupID     uint   `json:"group_id" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Load the group with conditions
	var group models.SignalConditionGroup
	if err := ac.db.Preload("Conditions").First(&group, request.GroupID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Condition group not found"})
		return
	}

	// Convert conditions to JSON format
	var conditions []map[string]interface{}
	for _, cond := range group.Conditions {
		condMap := map[string]interface{}{
			"indicator": string(cond.Indicator),
			"operator":  string(cond.Operator),
			"value":     cond.Value.InexactFloat64(),
			"weight":    cond.Weight,
			"required":  cond.IsRequired,
		}
		if cond.Value2.GreaterThan(decimal.Zero) {
			condMap["value2"] = cond.Value2.InexactFloat64()
		}
		if cond.CompareIndicator != "" {
			condMap["compare_indicator"] = string(cond.CompareIndicator)
		}
		conditions = append(conditions, condMap)
	}

	conditionsJSON, _ := json.Marshal(conditions)

	template := &models.SignalTemplate{
		Name:        request.Name,
		Description: request.Description,
		Category:    request.Category,
		Conditions:  string(conditionsJSON),
		IsBuiltIn:   false,
	}

	if err := ac.db.Create(template).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template created", "id": template.ID})
}
