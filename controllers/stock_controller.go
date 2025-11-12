package controllers

import (
	"net/http"
	"strconv"
	"time"

	"go_backend_project/models"
	"go_backend_project/services/analysis"
	"go_backend_project/services/datafetcher"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StockController handles stock-related requests
type StockController struct {
	db          *gorm.DB
	dataFetcher *datafetcher.DataFetcher
	analysis    *analysis.TechnicalAnalysis
}

// NewStockController creates a new stock controller
func NewStockController(db *gorm.DB) *StockController {
	return &StockController{
		db:          db,
		dataFetcher: datafetcher.NewDataFetcher(db),
		analysis:    analysis.NewTechnicalAnalysis(db),
	}
}

// GetStocks returns list of all stocks
// GET /api/stocks
func (sc *StockController) GetStocks(c *gin.Context) {
	var stocks []models.Stock

	// Parse query parameters
	exchange := c.Query("exchange")
	industry := c.Query("industry")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit

	query := sc.db.Model(&models.Stock{})

	if exchange != "" {
		query = query.Where("exchange = ?", exchange)
	}
	if industry != "" {
		query = query.Where("industry = ?", industry)
	}

	var total int64
	query.Count(&total)

	if err := query.Limit(limit).Offset(offset).Find(&stocks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stocks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": stocks,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetStock returns a single stock by ID or symbol
// GET /api/stocks/:id
func (sc *StockController) GetStock(c *gin.Context) {
	id := c.Param("id")

	var stock models.Stock

	// Try to find by ID first, then by symbol
	if err := sc.db.Where("id = ? OR symbol = ?", id, id).First(&stock).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Stock not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stock"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stock})
}

// GetStockPrice returns price data for a stock
// GET /api/stocks/:symbol/prices
func (sc *StockController) GetStockPrice(c *gin.Context) {
	symbol := c.Param("symbol")

	var stock models.Stock
	if err := sc.db.Where("symbol = ?", symbol).First(&stock).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Stock not found"})
		return
	}

	// Parse date range
	startDate := c.DefaultQuery("start_date", time.Now().AddDate(0, -1, 0).Format("2006-01-02"))
	endDate := c.DefaultQuery("end_date", time.Now().Format("2006-01-02"))

	var prices []models.StockPrice
	err := sc.db.Where("stock_id = ? AND date BETWEEN ? AND ?", stock.ID, startDate, endDate).
		Order("date DESC").
		Find(&prices).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prices"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": prices,
		"stock": stock,
	})
}

// GetRealtimeQuote returns real-time quote for a stock
// GET /api/stocks/:symbol/quote
func (sc *StockController) GetRealtimeQuote(c *gin.Context) {
	symbol := c.Param("symbol")

	quote, err := sc.dataFetcher.FetchRealtimeQuote(symbol)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": quote})
}

// GetTechnicalIndicators returns technical indicators for a stock
// GET /api/stocks/:symbol/indicators
func (sc *StockController) GetTechnicalIndicators(c *gin.Context) {
	symbol := c.Param("symbol")

	var stock models.Stock
	if err := sc.db.Where("symbol = ?", symbol).First(&stock).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Stock not found"})
		return
	}

	date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	indicatorType := c.Query("type")

	query := sc.db.Where("stock_id = ? AND DATE(date) = ?", stock.ID, date)
	if indicatorType != "" {
		query = query.Where("type = ?", indicatorType)
	}

	var indicators []models.TechnicalIndicator
	if err := query.Find(&indicators).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch indicators"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   indicators,
		"stock":  stock,
		"date":   date,
	})
}

// CalculateIndicators calculates and saves technical indicators
// POST /api/stocks/:symbol/indicators/calculate
func (sc *StockController) CalculateIndicators(c *gin.Context) {
	symbol := c.Param("symbol")

	var stock models.Stock
	if err := sc.db.Where("symbol = ?", symbol).First(&stock).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Stock not found"})
		return
	}

	date := time.Now()
	if dateStr := c.Query("date"); dateStr != "" {
		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err == nil {
			date = parsedDate
		}
	}

	if err := sc.analysis.CalculateAllIndicators(stock.ID, date); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to calculate indicators"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Indicators calculated successfully"})
}

// FetchHistoricalData fetches historical data for a stock
// POST /api/stocks/:symbol/fetch-historical
func (sc *StockController) FetchHistoricalData(c *gin.Context) {
	symbol := c.Param("symbol")

	var request struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	startDate, err := time.Parse("2006-01-02", request.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start date format"})
		return
	}

	endDate, err := time.Parse("2006-01-02", request.EndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end date format"})
		return
	}

	if err := sc.dataFetcher.FetchHistoricalData(symbol, startDate, endDate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Historical data fetched successfully"})
}

// GetMarketIndices returns market indices data
// GET /api/market/indices
func (sc *StockController) GetMarketIndices(c *gin.Context) {
	var indices []models.MarketIndex

	date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))

	err := sc.db.Where("DATE(date) = ?", date).Find(&indices).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch indices"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": indices, "date": date})
}

// SearchStocks searches for stocks by symbol or name
// GET /api/stocks/search
func (sc *StockController) SearchStocks(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Search query required"})
		return
	}

	var stocks []models.Stock
	err := sc.db.Where("symbol ILIKE ? OR name ILIKE ?", "%"+query+"%", "%"+query+"%").
		Limit(20).
		Find(&stocks).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stocks})
}

// GetTopGainers returns top gaining stocks
// GET /api/market/top-gainers
func (sc *StockController) GetTopGainers(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	var prices []models.StockPrice
	err := sc.db.
		Preload("Stock").
		Where("DATE(date) = ?", time.Now().Format("2006-01-02")).
		Order("change_percent DESC").
		Limit(limit).
		Find(&prices).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch top gainers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": prices})
}

// GetTopLosers returns top losing stocks
// GET /api/market/top-losers
func (sc *StockController) GetTopLosers(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	var prices []models.StockPrice
	err := sc.db.
		Preload("Stock").
		Where("DATE(date) = ?", time.Now().Format("2006-01-02")).
		Order("change_percent ASC").
		Limit(limit).
		Find(&prices).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch top losers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": prices})
}

// GetMostActive returns most actively traded stocks
// GET /api/market/most-active
func (sc *StockController) GetMostActive(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	var prices []models.StockPrice
	err := sc.db.
		Preload("Stock").
		Where("DATE(date) = ?", time.Now().Format("2006-01-02")).
		Order("volume DESC").
		Limit(limit).
		Find(&prices).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch most active"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": prices})
}
