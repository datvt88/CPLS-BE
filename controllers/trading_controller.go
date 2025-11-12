package controllers

import (
	"net/http"
	"strconv"
	"time"

	"go_backend_project/models"
	"go_backend_project/services/backtesting"
	"go_backend_project/services/trading"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TradingController handles trading-related requests
type TradingController struct {
	db             *gorm.DB
	tradingBot     *trading.TradingBot
	backtestEngine *backtesting.BacktestEngine
}

// NewTradingController creates a new trading controller
func NewTradingController(db *gorm.DB) *TradingController {
	return &TradingController{
		db:             db,
		tradingBot:     trading.NewTradingBot(db),
		backtestEngine: backtesting.NewBacktestEngine(db),
	}
}

// GetStrategies returns all trading strategies
// GET /api/strategies
func (tc *TradingController) GetStrategies(c *gin.Context) {
	var strategies []models.TradingStrategy

	if err := tc.db.Find(&strategies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch strategies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": strategies})
}

// CreateStrategy creates a new trading strategy
// POST /api/strategies
func (tc *TradingController) CreateStrategy(c *gin.Context) {
	var strategy models.TradingStrategy

	if err := c.ShouldBindJSON(&strategy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := tc.db.Create(&strategy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create strategy"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": strategy})
}

// UpdateStrategy updates a trading strategy
// PUT /api/strategies/:id
func (tc *TradingController) UpdateStrategy(c *gin.Context) {
	id := c.Param("id")

	var strategy models.TradingStrategy
	if err := tc.db.First(&strategy, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}

	if err := c.ShouldBindJSON(&strategy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := tc.db.Save(&strategy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update strategy"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": strategy})
}

// DeleteStrategy deletes a trading strategy
// DELETE /api/strategies/:id
func (tc *TradingController) DeleteStrategy(c *gin.Context) {
	id := c.Param("id")

	if err := tc.db.Delete(&models.TradingStrategy{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete strategy"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy deleted successfully"})
}

// RunBacktest runs a backtest for a strategy
// POST /api/backtests
func (tc *TradingController) RunBacktest(c *gin.Context) {
	var request struct {
		StrategyID     uint     `json:"strategy_id" binding:"required"`
		StartDate      string   `json:"start_date" binding:"required"`
		EndDate        string   `json:"end_date" binding:"required"`
		InitialCapital float64  `json:"initial_capital" binding:"required"`
		Symbols        []string `json:"symbols" binding:"required"`
		Commission     float64  `json:"commission"`
		RiskPerTrade   float64  `json:"risk_per_trade"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	startDate, err := time.Parse("2006-01-02", request.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start date"})
		return
	}

	endDate, err := time.Parse("2006-01-02", request.EndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end date"})
		return
	}

	config := &backtesting.BacktestConfig{
		StrategyID:     request.StrategyID,
		StartDate:      startDate,
		EndDate:        endDate,
		InitialCapital: decimal.NewFromFloat(request.InitialCapital),
		Commission:     decimal.NewFromFloat(request.Commission),
		Symbols:        request.Symbols,
		RiskPerTrade:   decimal.NewFromFloat(request.RiskPerTrade),
	}

	backtest, err := tc.backtestEngine.RunBacktest(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": backtest})
}

// GetBacktests returns all backtests
// GET /api/backtests
func (tc *TradingController) GetBacktests(c *gin.Context) {
	var backtests []models.Backtest

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	tc.db.Model(&models.Backtest{}).Count(&total)

	err := tc.db.Preload("Strategy").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&backtests).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch backtests"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": backtests,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetBacktest returns a single backtest with details
// GET /api/backtests/:id
func (tc *TradingController) GetBacktest(c *gin.Context) {
	id := c.Param("id")

	var backtest models.Backtest
	if err := tc.db.Preload("Strategy").First(&backtest, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Backtest not found"})
		return
	}

	// Get trades for this backtest
	var trades []models.BacktestTrade
	tc.db.Where("backtest_id = ?", id).Preload("Stock").Find(&trades)

	c.JSON(http.StatusOK, gin.H{
		"data":   backtest,
		"trades": trades,
	})
}

// GetSignals returns trading signals
// GET /api/signals
func (tc *TradingController) GetSignals(c *gin.Context) {
	var signals []models.Signal

	isActive := c.DefaultQuery("is_active", "true")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit

	query := tc.db.Model(&models.Signal{})
	if isActive == "true" {
		query = query.Where("is_active = ?", true)
	}

	var total int64
	query.Count(&total)

	err := query.Preload("Stock").
		Preload("Strategy").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&signals).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch signals"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": signals,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// StartTradingBot starts the automated trading bot
// POST /api/trading/bot/start
func (tc *TradingController) StartTradingBot(c *gin.Context) {
	if err := tc.tradingBot.Start(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Trading bot started successfully"})
}

// StopTradingBot stops the trading bot
// POST /api/trading/bot/stop
func (tc *TradingController) StopTradingBot(c *gin.Context) {
	tc.tradingBot.Stop()
	c.JSON(http.StatusOK, gin.H{"message": "Trading bot stopped"})
}

// GetTradingBotStatus returns the status of the trading bot
// GET /api/trading/bot/status
func (tc *TradingController) GetTradingBotStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"is_running": tc.tradingBot.IsRunning(),
	})
}

// ExecuteManualTrade executes a manual trade
// POST /api/trading/manual
func (tc *TradingController) ExecuteManualTrade(c *gin.Context) {
	var request struct {
		UserID   uint    `json:"user_id" binding:"required"`
		StockID  uint    `json:"stock_id" binding:"required"`
		Type     string  `json:"type" binding:"required"`
		Quantity int64   `json:"quantity" binding:"required"`
		Price    float64 `json:"price" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if request.Type != "BUY" && request.Type != "SELL" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Type must be BUY or SELL"})
		return
	}

	err := tc.tradingBot.ManualTrade(
		request.UserID,
		request.StockID,
		request.Type,
		request.Quantity,
		decimal.NewFromFloat(request.Price),
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Trade executed successfully"})
}

// GetTrades returns trade history
// GET /api/trades
func (tc *TradingController) GetTrades(c *gin.Context) {
	var trades []models.Trade

	userID := c.Query("user_id")
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit

	query := tc.db.Model(&models.Trade{})

	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	err := query.Preload("Stock").
		Preload("Strategy").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&trades).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch trades"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": trades,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetPortfolio returns user's portfolio
// GET /api/portfolio
func (tc *TradingController) GetPortfolio(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		return
	}

	var portfolio []models.Portfolio
	err := tc.db.Where("user_id = ? AND quantity > 0", userID).
		Preload("Stock").
		Find(&portfolio).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch portfolio"})
		return
	}

	// Calculate total values
	totalCost := decimal.Zero
	totalValue := decimal.Zero

	for _, pos := range portfolio {
		totalCost = totalCost.Add(pos.TotalCost)
		totalValue = totalValue.Add(pos.MarketValue)
	}

	totalPnL := totalValue.Sub(totalCost)
	totalPnLPercent := decimal.Zero
	if totalCost.GreaterThan(decimal.Zero) {
		totalPnLPercent = totalPnL.Div(totalCost).Mul(decimal.NewFromInt(100))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": portfolio,
		"summary": gin.H{
			"total_cost":        totalCost,
			"total_value":       totalValue,
			"total_pnl":         totalPnL,
			"total_pnl_percent": totalPnLPercent,
		},
	})
}
