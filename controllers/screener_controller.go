package controllers

import (
	"log"
	"net/http"
	"strconv"

	"go_backend_project/config"
	"go_backend_project/services"
	"go_backend_project/services/screener"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ScreenerController handles stock screening requests
type ScreenerController struct {
	db            *gorm.DB
	screener      *screener.StockScreener
	mongoScreener *services.ScreenerService
}

// NewScreenerController creates a new screener controller
func NewScreenerController(db *gorm.DB) *ScreenerController {
	ctrl := &ScreenerController{
		db:       db,
		screener: screener.NewStockScreener(db),
	}

	// Try to initialize MongoDB screener if available
	if config.GlobalDBConfig != nil && config.GlobalDBConfig.IsMongoDBAvailable() {
		mongoDB := config.GlobalDBConfig.GetMongoDatabase()
		ctrl.mongoScreener = services.NewScreenerService(mongoDB)
		log.Println("ScreenerController: MongoDB screener enabled")
	} else {
		log.Println("ScreenerController: MongoDB screener not available, using PostgreSQL fallback")
	}

	return ctrl
}

// ScreenMongo applies filters and returns matching stocks from MongoDB
// POST /api/v1/screener/screen-mongo
func (sc *ScreenerController) ScreenMongo(c *gin.Context) {
	if sc.mongoScreener == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB screener not available",
		})
		return
	}

	var filter services.FilterParams

	if err := c.ShouldBindJSON(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results, err := sc.mongoScreener.GetScreener(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}

// Screen applies filters and returns matching stocks
// POST /api/v1/screener/screen
func (sc *ScreenerController) Screen(c *gin.Context) {
	var filter screener.ScreenerFilter

	if err := c.ShouldBindJSON(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results, total, err := sc.screener.Screen(&filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  results,
		"total": total,
		"pagination": gin.H{
			"page":  filter.Page,
			"limit": filter.Limit,
		},
	})
}

// GetPresets returns predefined screener configurations
// GET /api/v1/screener/presets
func (sc *ScreenerController) GetPresets(c *gin.Context) {
	presets := sc.screener.GetPresetScreeners()
	c.JSON(http.StatusOK, gin.H{"data": presets})
}

// RunPreset runs a predefined screener
// GET /api/v1/screener/presets/:id
func (sc *ScreenerController) RunPreset(c *gin.Context) {
	presetID := c.Param("id")

	presets := sc.screener.GetPresetScreeners()
	var selectedFilter screener.ScreenerFilter
	found := false

	for _, preset := range presets {
		if preset["id"] == presetID {
			if f, ok := preset["filter"].(screener.ScreenerFilter); ok {
				selectedFilter = f
				found = true
				break
			}
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "Preset not found"})
		return
	}

	// Apply pagination from query params
	if page := c.Query("page"); page != "" {
		if pageNum, err := strconv.Atoi(page); err == nil && pageNum > 0 {
			selectedFilter.Page = pageNum
		}
	}
	if limit := c.Query("limit"); limit != "" {
		if limitNum, err := strconv.Atoi(limit); err == nil && limitNum > 0 {
			selectedFilter.Limit = limitNum
		}
	}
	if selectedFilter.Page <= 0 {
		selectedFilter.Page = 1
	}
	if selectedFilter.Limit <= 0 {
		selectedFilter.Limit = 50
	}

	results, total, err := sc.screener.Screen(&selectedFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      results,
		"total":     total,
		"preset_id": presetID,
		"pagination": gin.H{
			"page":  selectedFilter.Page,
			"limit": selectedFilter.Limit,
		},
	})
}

// GetTopGainers returns top gaining stocks
// GET /api/v1/screener/top-gainers
func (sc *ScreenerController) GetTopGainers(c *gin.Context) {
	minChange := 0.0
	filter := screener.ScreenerFilter{
		MinChangePercent: &minChange,
		SortBy:           "change_percent",
		SortOrder:        "desc",
		Page:             1,
		Limit:            20,
	}

	results, total, err := sc.screener.Screen(&filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  results,
		"total": total,
	})
}

// GetTopLosers returns top losing stocks
// GET /api/v1/screener/top-losers
func (sc *ScreenerController) GetTopLosers(c *gin.Context) {
	maxChange := 0.0
	filter := screener.ScreenerFilter{
		MaxChangePercent: &maxChange,
		SortBy:           "change_percent",
		SortOrder:        "asc",
		Page:             1,
		Limit:            20,
	}

	results, total, err := sc.screener.Screen(&filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  results,
		"total": total,
	})
}

// GetMostActive returns most actively traded stocks
// GET /api/v1/screener/most-active
func (sc *ScreenerController) GetMostActive(c *gin.Context) {
	filter := screener.ScreenerFilter{
		SortBy:    "volume",
		SortOrder: "desc",
		Page:      1,
		Limit:     20,
	}

	results, total, err := sc.screener.Screen(&filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  results,
		"total": total,
	})
}

// GetOversoldStocks returns oversold stocks (RSI < 30)
// GET /api/v1/screener/oversold
func (sc *ScreenerController) GetOversoldStocks(c *gin.Context) {
	maxRSI := 30.0
	filter := screener.ScreenerFilter{
		MaxRSI:    &maxRSI,
		SortBy:    "volume",
		SortOrder: "desc",
		Page:      1,
		Limit:     20,
	}

	results, total, err := sc.screener.Screen(&filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  results,
		"total": total,
	})
}

// GetOverboughtStocks returns overbought stocks (RSI > 70)
// GET /api/v1/screener/overbought
func (sc *ScreenerController) GetOverboughtStocks(c *gin.Context) {
	minRSI := 70.0
	filter := screener.ScreenerFilter{
		MinRSI:    &minRSI,
		SortBy:    "volume",
		SortOrder: "desc",
		Page:      1,
		Limit:     20,
	}

	results, total, err := sc.screener.Screen(&filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  results,
		"total": total,
	})
}

// GetBullishStocks returns stocks with bullish indicators
// GET /api/v1/screener/bullish
func (sc *ScreenerController) GetBullishStocks(c *gin.Context) {
	aboveSMA20 := true
	aboveSMA50 := true
	macdBullish := true

	filter := screener.ScreenerFilter{
		AboveSMA20:  &aboveSMA20,
		AboveSMA50:  &aboveSMA50,
		MACDBullish: &macdBullish,
		SortBy:      "change_percent",
		SortOrder:   "desc",
		Page:        1,
		Limit:       20,
	}

	results, total, err := sc.screener.Screen(&filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  results,
		"total": total,
	})
}

// GetVolumeSpike returns stocks with volume spikes
// GET /api/v1/screener/volume-spike
func (sc *ScreenerController) GetVolumeSpike(c *gin.Context) {
	volumeSpike := 2.0
	filter := screener.ScreenerFilter{
		VolumeSpike: &volumeSpike,
		SortBy:      "volume",
		SortOrder:   "desc",
		Page:        1,
		Limit:       20,
	}

	results, total, err := sc.screener.Screen(&filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  results,
		"total": total,
	})
}

// ================ MongoDB-Based Screener Endpoints ================

// GetOversoldMongo returns oversold stocks from MongoDB (RSI < 30)
// GET /api/v1/screener/mongo/oversold
func (sc *ScreenerController) GetOversoldMongo(c *gin.Context) {
	if sc.mongoScreener == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB screener not available",
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	results, err := sc.mongoScreener.GetOversoldStocks(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetOverboughtMongo returns overbought stocks from MongoDB (RSI > 70)
// GET /api/v1/screener/mongo/overbought
func (sc *ScreenerController) GetOverboughtMongo(c *gin.Context) {
	if sc.mongoScreener == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB screener not available",
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	results, err := sc.mongoScreener.GetOverboughtStocks(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetBullishMongo returns bullish stocks from MongoDB
// GET /api/v1/screener/mongo/bullish
func (sc *ScreenerController) GetBullishMongo(c *gin.Context) {
	if sc.mongoScreener == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB screener not available",
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	results, err := sc.mongoScreener.GetBullishStocks(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetStockIndicatorsMongo returns technical indicators for a specific stock from MongoDB
// GET /api/v1/screener/mongo/stock/:code
func (sc *ScreenerController) GetStockIndicatorsMongo(c *gin.Context) {
	if sc.mongoScreener == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB screener not available",
		})
		return
	}

	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Stock code is required"})
		return
	}

	result, err := sc.mongoScreener.GetStockIndicators(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetMarketStatistics returns overall market statistics from MongoDB
// GET /api/v1/screener/mongo/statistics
func (sc *ScreenerController) GetMarketStatistics(c *gin.Context) {
	if sc.mongoScreener == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB screener not available",
		})
		return
	}

	stats, err := sc.mongoScreener.GetStatistics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}
