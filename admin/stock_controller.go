package admin

import (
	"net/http"
	"strconv"

	"go_backend_project/services"

	"github.com/gin-gonic/gin"
)

// StockController handles stock management operations
type StockController struct {
	supabaseClient *services.SupabaseDBClient
}

// NewStockController creates a new stock controller
func NewStockController(client *services.SupabaseDBClient) *StockController {
	return &StockController{
		supabaseClient: client,
	}
}

// ListStocks handles GET /admin/stocks - displays stock list page
func (ctrl *StockController) ListStocks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	search := c.Query("search")
	floor := c.DefaultQuery("floor", "all")
	sortBy := c.DefaultQuery("sort_by", "code")
	sortOrder := c.DefaultQuery("sort_order", "asc")

	result, err := ctrl.supabaseClient.GetStocks(page, pageSize, search, floor, sortBy, sortOrder)
	if err != nil {
		c.HTML(http.StatusOK, "stocks_management.html", gin.H{
			"Title":     "Stock Management",
			"AdminUser": c.GetString("admin_username"),
			"Error":     err.Error(),
		})
		return
	}

	// Get stats
	stats, _ := ctrl.supabaseClient.GetStockStats()

	// Get last sync time
	lastSync, _ := ctrl.supabaseClient.GetLastSyncTime()

	c.HTML(http.StatusOK, "stocks_management.html", gin.H{
		"Title":      "Stock Management",
		"AdminUser":  c.GetString("admin_username"),
		"Stocks":     result.Stocks,
		"Total":      result.Total,
		"Page":       result.Page,
		"PageSize":   result.PageSize,
		"TotalPages": result.TotalPages,
		"Search":     search,
		"Floor":      floor,
		"SortBy":     sortBy,
		"SortOrder":  sortOrder,
		"Stats":      stats,
		"LastSync":   lastSync,
	})
}

// GetStock handles GET /admin/api/stocks/:code - returns stock details as JSON
func (ctrl *StockController) GetStock(c *gin.Context) {
	code := c.Param("code")

	stock, err := ctrl.supabaseClient.GetStockByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stock)
}

// SyncStocks handles POST /admin/api/stocks/sync - syncs stocks from VNDirect
func (ctrl *StockController) SyncStocks(c *gin.Context) {
	result, err := ctrl.supabaseClient.SyncStocksFromVNDirect()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Stock sync completed",
		"result":  result,
	})
}

// DeleteStock handles DELETE /admin/api/stocks/:code - deletes a stock
func (ctrl *StockController) DeleteStock(c *gin.Context) {
	code := c.Param("code")

	if err := ctrl.supabaseClient.DeleteStock(code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Stock deleted successfully"})
}

// GetStats handles GET /admin/api/stocks/stats - returns stock statistics
func (ctrl *StockController) GetStats(c *gin.Context) {
	stats, err := ctrl.supabaseClient.GetStockStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// SearchStocks handles GET /admin/api/stocks/search - searches stocks
func (ctrl *StockController) SearchStocks(c *gin.Context) {
	search := c.Query("q")
	floor := c.DefaultQuery("floor", "all")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if limit > 100 {
		limit = 100
	}

	result, err := ctrl.supabaseClient.GetStocks(1, limit, search, floor, "code", "asc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result.Stocks)
}

// ExportStocks handles GET /admin/api/stocks/export - exports stocks to JSON
func (ctrl *StockController) ExportStocks(c *gin.Context) {
	floor := c.DefaultQuery("floor", "all")

	result, err := ctrl.supabaseClient.GetStocks(1, 10000, "", floor, "code", "asc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Disposition", "attachment; filename=stocks_export.json")
	c.JSON(http.StatusOK, result.Stocks)
}

// ImportStocks handles POST /admin/api/stocks/import - imports stocks from JSON file
func (ctrl *StockController) ImportStocks(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	// Open the file
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer f.Close()

	// Read file content
	data := make([]byte, file.Size)
	if _, err := f.Read(data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
		return
	}

	// Import stocks
	result, err := services.ImportStocksFromJSON(data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Stocks imported successfully",
		"result":  result,
	})
}

// GetSchedulerConfig handles GET /admin/api/stocks/scheduler - returns scheduler config
func (ctrl *StockController) GetSchedulerConfig(c *gin.Context) {
	if services.GlobalStockScheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler not initialized"})
		return
	}

	config := services.GlobalStockScheduler.GetConfig()
	c.JSON(http.StatusOK, gin.H{
		"enabled":       config.Enabled,
		"schedule_time": config.ScheduleTime,
		"last_run":      config.LastRun,
		"next_run":      config.NextRun,
		"is_running":    services.GlobalStockScheduler.IsRunning(),
	})
}

// UpdateSchedulerConfig handles PUT /admin/api/stocks/scheduler - updates scheduler config
func (ctrl *StockController) UpdateSchedulerConfig(c *gin.Context) {
	if services.GlobalStockScheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler not initialized"})
		return
	}

	var req struct {
		Enabled      bool   `json:"enabled"`
		ScheduleTime string `json:"schedule_time"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate schedule time format (HH:MM)
	if len(req.ScheduleTime) < 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schedule time format. Use HH:MM"})
		return
	}

	if err := services.GlobalStockScheduler.UpdateConfig(req.Enabled, req.ScheduleTime); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	config := services.GlobalStockScheduler.GetConfig()
	c.JSON(http.StatusOK, gin.H{
		"message":       "Scheduler updated successfully",
		"enabled":       config.Enabled,
		"schedule_time": config.ScheduleTime,
		"next_run":      config.NextRun,
	})
}

// ==================== Price Sync Endpoints ====================

// GetPriceConfig handles GET /admin/api/prices/config - returns price sync config
func (ctrl *StockController) GetPriceConfig(c *gin.Context) {
	if services.GlobalPriceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Price service not initialized"})
		return
	}

	config := services.GlobalPriceService.GetConfig()
	c.JSON(http.StatusOK, config)
}

// UpdatePriceConfig handles PUT /admin/api/prices/config - updates price sync config
func (ctrl *StockController) UpdatePriceConfig(c *gin.Context) {
	if services.GlobalPriceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Price service not initialized"})
		return
	}

	var req struct {
		DelayMS      int `json:"delay_ms"`
		BatchSize    int `json:"batch_size"`
		BatchPauseMS int `json:"batch_pause_ms"`
		PriceSize    int `json:"price_size"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate
	if req.DelayMS < 100 {
		req.DelayMS = 100
	}
	if req.BatchSize < 1 {
		req.BatchSize = 10
	}
	if req.BatchPauseMS < 1000 {
		req.BatchPauseMS = 1000
	}
	if req.PriceSize < 30 {
		req.PriceSize = 30
	}

	if err := services.GlobalPriceService.UpdateConfig(req.DelayMS, req.BatchSize, req.BatchPauseMS, req.PriceSize); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Price config updated successfully",
		"config":  services.GlobalPriceService.GetConfig(),
	})
}

// StartPriceSync handles POST /admin/api/prices/sync - starts price sync for all stocks
func (ctrl *StockController) StartPriceSync(c *gin.Context) {
	if services.GlobalPriceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Price service not initialized"})
		return
	}

	if err := services.GlobalPriceService.StartFullSync(); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Price sync started",
	})
}

// StopPriceSync handles POST /admin/api/prices/stop - stops price sync
func (ctrl *StockController) StopPriceSync(c *gin.Context) {
	if services.GlobalPriceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Price service not initialized"})
		return
	}

	services.GlobalPriceService.StopSync()
	c.JSON(http.StatusOK, gin.H{
		"message": "Price sync stopped",
	})
}

// GetPriceSyncProgress handles GET /admin/api/prices/progress - returns sync progress
func (ctrl *StockController) GetPriceSyncProgress(c *gin.Context) {
	if services.GlobalPriceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Price service not initialized"})
		return
	}

	progress := services.GlobalPriceService.GetProgress()
	c.JSON(http.StatusOK, progress)
}

// GetPriceSyncStats handles GET /admin/api/prices/stats - returns price sync statistics
func (ctrl *StockController) GetPriceSyncStats(c *gin.Context) {
	if services.GlobalPriceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Price service not initialized"})
		return
	}

	stats, err := services.GlobalPriceService.GetPriceSyncStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// SyncSingleStockPrice handles POST /admin/api/prices/:code - syncs price for single stock
func (ctrl *StockController) SyncSingleStockPrice(c *gin.Context) {
	if services.GlobalPriceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Price service not initialized"})
		return
	}

	code := c.Param("code")
	priceFile, err := services.GlobalPriceService.SyncSingleStock(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Price synced successfully",
		"code":        priceFile.Code,
		"data_count":  priceFile.DataCount,
		"last_updated": priceFile.LastUpdated,
	})
}

// GetStockPrice handles GET /admin/api/prices/:code - returns price data for a stock
func (ctrl *StockController) GetStockPrice(c *gin.Context) {
	if services.GlobalPriceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Price service not initialized"})
		return
	}

	code := c.Param("code")
	priceFile, err := services.GlobalPriceService.LoadStockPrice(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Price data not found for " + code})
		return
	}

	c.JSON(http.StatusOK, priceFile)
}

// ==================== Indicator Endpoints ====================

// CalculateAllIndicators handles POST /admin/api/indicators/calculate - calculates all indicators
func (ctrl *StockController) CalculateAllIndicators(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Indicator service not initialized"})
		return
	}

	if err := services.GlobalIndicatorService.CalculateAndSaveAllIndicators(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Indicators calculated successfully",
	})
}

// GetIndicatorSummary handles GET /admin/api/indicators/summary - returns all indicators summary
func (ctrl *StockController) GetIndicatorSummary(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Indicator service not initialized"})
		return
	}

	summary, err := services.GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Indicator summary not found. Please calculate indicators first."})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// GetStockIndicators handles GET /admin/api/indicators/:code - returns indicators for a stock
func (ctrl *StockController) GetStockIndicators(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Indicator service not initialized"})
		return
	}

	code := c.Param("code")
	indicators, err := services.GlobalIndicatorService.GetStockIndicators(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Indicators not found for " + code})
		return
	}

	c.JSON(http.StatusOK, indicators)
}

// FilterStocks handles POST /admin/api/indicators/filter - filters stocks by indicators
func (ctrl *StockController) FilterStocks(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Indicator service not initialized"})
		return
	}

	var filter services.IndicatorFilter
	if err := c.ShouldBindJSON(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results, err := services.GlobalIndicatorService.FilterStocks(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count":  len(results),
		"stocks": results,
	})
}

// GetTopRSStocks handles GET /admin/api/indicators/top-rs - returns top RS ranked stocks
// Filter conditions: avgVol >= 600000, RS1YRank >= 80, RSAvg >= 40, MACDHist > -0.1
func (ctrl *StockController) GetTopRSStocks(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Indicator service not initialized"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit > 200 {
		limit = 200
	}

	summary, err := services.GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Indicator summary not found"})
		return
	}

	// Collect and filter stocks
	type stockRS struct {
		Code       string                             `json:"code"`
		Indicators *services.ExtendedStockIndicators `json:"indicators"`
	}

	var stocks []stockRS
	for code, ind := range summary.Stocks {
		if ind == nil {
			continue
		}

		// Apply filter conditions:
		// avgVol (5 days) >= 600000
		if ind.AvgVol < 600000 {
			continue
		}

		// RS1YRank >= 80
		if ind.RS1YRank < 80 {
			continue
		}

		// RSAvg >= 40
		if ind.RSAvg < 40 {
			continue
		}

		// MACDHist > -0.1
		if ind.MACDHist <= -0.1 {
			continue
		}

		stocks = append(stocks, stockRS{Code: code, Indicators: ind})
	}

	// Sort by RSAvg descending
	for i := 0; i < len(stocks)-1; i++ {
		for j := i + 1; j < len(stocks); j++ {
			if stocks[j].Indicators.RSAvg > stocks[i].Indicators.RSAvg {
				stocks[i], stocks[j] = stocks[j], stocks[i]
			}
		}
	}

	// Limit results
	if len(stocks) > limit {
		stocks = stocks[:limit]
	}

	c.JSON(http.StatusOK, gin.H{
		"count":  len(stocks),
		"stocks": stocks,
		"filter": gin.H{
			"avg_vol_min":    600000,
			"rs_1y_rank_min": 80,
			"rs_avg_min":     40,
			"macd_hist_min":  -0.1,
		},
	})
}

// ==================== Realtime WebSocket Endpoints ====================

// HandleRealtimeWebSocket handles WebSocket connections for realtime prices
func (ctrl *StockController) HandleRealtimeWebSocket(c *gin.Context) {
	if services.GlobalRealtimeService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Realtime service not initialized"})
		return
	}

	services.GlobalRealtimeService.HandleWebSocket(c.Writer, c.Request)
}

// StartRealtimePolling starts realtime price polling
func (ctrl *StockController) StartRealtimePolling(c *gin.Context) {
	if services.GlobalRealtimeService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Realtime service not initialized"})
		return
	}

	var req struct {
		Codes    []string `json:"codes"`
		Interval int      `json:"interval"` // seconds
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		// Start with top RS stocks if no codes provided
		req.Codes = nil
	}

	if req.Interval > 0 {
		services.GlobalRealtimeService.SetPollingInterval(req.Interval)
	}

	if err := services.GlobalRealtimeService.StartPolling(req.Codes); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Realtime polling started",
		"codes":   len(req.Codes),
	})
}

// StopRealtimePolling stops realtime price polling
func (ctrl *StockController) StopRealtimePolling(c *gin.Context) {
	if services.GlobalRealtimeService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Realtime service not initialized"})
		return
	}

	services.GlobalRealtimeService.StopPolling()
	c.JSON(http.StatusOK, gin.H{"message": "Realtime polling stopped"})
}

// GetRealtimeStatus returns realtime service status
func (ctrl *StockController) GetRealtimeStatus(c *gin.Context) {
	if services.GlobalRealtimeService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Realtime service not initialized"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"is_polling":    services.GlobalRealtimeService.IsPolling(),
		"client_count":  services.GlobalRealtimeService.GetClientCount(),
		"websocket_url": "/ws/realtime",
	})
}
