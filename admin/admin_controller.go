package admin

import (
	"net/http"
	"strconv"

	"go_backend_project/models"
	"go_backend_project/services/datafetcher"
	"go_backend_project/services/backtesting"
	"go_backend_project/services/trading"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"time"
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
	// Get statistics
	var stockCount int64
	ac.db.Model(&models.Stock{}).Count(&stockCount)

	var strategyCount int64
	ac.db.Model(&models.TradingStrategy{}).Count(&strategyCount)

	var backtestCount int64
	ac.db.Model(&models.Backtest{}).Count(&backtestCount)

	var tradeCount int64
	ac.db.Model(&models.Trade{}).Count(&tradeCount)

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"stockCount":    stockCount,
		"strategyCount": strategyCount,
		"backtestCount": backtestCount,
		"tradeCount":    tradeCount,
		"botRunning":    ac.tradingBot.IsRunning(),
	})
}

// StocksPage shows stocks management page
func (ac *AdminController) StocksPage(c *gin.Context) {
	var stocks []models.Stock
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit := 50
	offset := (page - 1) * limit

	var total int64
	ac.db.Model(&models.Stock{}).Count(&total)

	ac.db.Limit(limit).Offset(offset).Find(&stocks)

	c.HTML(http.StatusOK, "stocks.html", gin.H{
		"stocks": stocks,
		"page":   page,
		"total":  total,
	})
}

// StrategiesPage shows strategies management page
func (ac *AdminController) StrategiesPage(c *gin.Context) {
	var strategies []models.TradingStrategy
	ac.db.Find(&strategies)

	c.HTML(http.StatusOK, "strategies.html", gin.H{
		"strategies": strategies,
	})
}

// BacktestsPage shows backtests page
func (ac *AdminController) BacktestsPage(c *gin.Context) {
	var backtests []models.Backtest
	ac.db.Preload("Strategy").Order("created_at DESC").Limit(50).Find(&backtests)

	c.HTML(http.StatusOK, "backtests.html", gin.H{
		"backtests": backtests,
	})
}

// TradingBotPage shows trading bot control page
func (ac *AdminController) TradingBotPage(c *gin.Context) {
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
