package admin

import (
	"net/http"
	"strconv"

	"go_backend_project/services"

	"github.com/gin-gonic/gin"
)

// TCBSPriceController handles TCBS stock price operations
type TCBSPriceController struct {
	supabaseClient *services.SupabaseDBClient
}

// NewTCBSPriceController creates a new TCBS price controller
func NewTCBSPriceController(client *services.SupabaseDBClient) *TCBSPriceController {
	return &TCBSPriceController{
		supabaseClient: client,
	}
}

// ListPrices handles GET /admin/prices - displays price list page
func (ctrl *TCBSPriceController) ListPrices(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	search := c.Query("search")
	exchange := c.DefaultQuery("exchange", "all")
	sortBy := c.DefaultQuery("sort_by", "ticker")
	sortOrder := c.DefaultQuery("sort_order", "asc")

	result, err := ctrl.supabaseClient.GetStockPrices(page, pageSize, search, exchange, sortBy, sortOrder)
	if err != nil {
		c.HTML(http.StatusOK, "tcbs_prices.html", gin.H{
			"Title":     "TCBS Stock Prices",
			"AdminUser": c.GetString("admin_username"),
			"Error":     err.Error(),
		})
		return
	}

	// Get stats
	stats, _ := ctrl.supabaseClient.GetPriceStats()

	// Get last sync time
	lastSync, _ := ctrl.supabaseClient.GetPriceLastSyncTime()

	// Get stock count for reference
	stockCount, _ := ctrl.supabaseClient.GetStockCount()

	// Check if syncing
	isSyncing := ctrl.supabaseClient.IsPriceSyncing()

	c.HTML(http.StatusOK, "tcbs_prices.html", gin.H{
		"Title":      "TCBS Stock Prices",
		"AdminUser":  c.GetString("admin_username"),
		"Prices":     result.Prices,
		"Total":      result.Total,
		"Page":       result.Page,
		"PageSize":   result.PageSize,
		"TotalPages": result.TotalPages,
		"Search":     search,
		"Exchange":   exchange,
		"SortBy":     sortBy,
		"SortOrder":  sortOrder,
		"Stats":      stats,
		"LastSync":   lastSync,
		"StockCount": stockCount,
		"IsSyncing":  isSyncing,
	})
}

// GetPrice handles GET /admin/api/prices/:ticker - returns price details as JSON
func (ctrl *TCBSPriceController) GetPrice(c *gin.Context) {
	ticker := c.Param("ticker")

	price, err := ctrl.supabaseClient.GetStockPrice(ticker)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, price)
}

// SyncPrices handles POST /admin/api/prices/sync - syncs prices from TCBS
func (ctrl *TCBSPriceController) SyncPrices(c *gin.Context) {
	// Check if already syncing
	if ctrl.supabaseClient.IsPriceSyncing() {
		c.JSON(http.StatusConflict, gin.H{"error": "Sync already in progress"})
		return
	}

	// Get chunk size and delay from query params (with defaults)
	chunkSize, _ := strconv.Atoi(c.DefaultQuery("chunk_size", "50"))
	delayMs, _ := strconv.Atoi(c.DefaultQuery("delay_ms", "500"))

	// Validate chunk size
	if chunkSize < 10 {
		chunkSize = 10
	}
	if chunkSize > 100 {
		chunkSize = 100
	}

	// Validate delay
	if delayMs < 100 {
		delayMs = 100
	}
	if delayMs > 5000 {
		delayMs = 5000
	}

	result, err := ctrl.supabaseClient.SyncStockPricesFromTCBS(chunkSize, delayMs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Price sync completed",
		"result":  result,
	})
}

// GetStats handles GET /admin/api/prices/stats - returns price statistics
func (ctrl *TCBSPriceController) GetStats(c *gin.Context) {
	stats, err := ctrl.supabaseClient.GetPriceStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Add syncing status
	stats["is_syncing"] = ctrl.supabaseClient.IsPriceSyncing()

	c.JSON(http.StatusOK, stats)
}

// GetTopGainers handles GET /admin/api/prices/top-gainers - returns top gainers
func (ctrl *TCBSPriceController) GetTopGainers(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit > 50 {
		limit = 50
	}

	gainers := services.GlobalPriceStore.GetTopGainers(limit)
	c.JSON(http.StatusOK, gainers)
}

// GetTopLosers handles GET /admin/api/prices/top-losers - returns top losers
func (ctrl *TCBSPriceController) GetTopLosers(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit > 50 {
		limit = 50
	}

	losers := services.GlobalPriceStore.GetTopLosers(limit)
	c.JSON(http.StatusOK, losers)
}

// GetTopVolume handles GET /admin/api/prices/top-volume - returns top volume
func (ctrl *TCBSPriceController) GetTopVolume(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit > 50 {
		limit = 50
	}

	topVol := services.GlobalPriceStore.GetTopVolume(limit)
	c.JSON(http.StatusOK, topVol)
}

// SearchPrices handles GET /admin/api/prices/search - searches prices
func (ctrl *TCBSPriceController) SearchPrices(c *gin.Context) {
	search := c.Query("q")
	exchange := c.DefaultQuery("exchange", "all")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if limit > 100 {
		limit = 100
	}

	result, err := ctrl.supabaseClient.GetStockPrices(1, limit, search, exchange, "ticker", "asc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result.Prices)
}

// ExportPrices handles GET /admin/api/prices/export - exports prices to JSON
func (ctrl *TCBSPriceController) ExportPrices(c *gin.Context) {
	exchange := c.DefaultQuery("exchange", "all")

	result, err := ctrl.supabaseClient.GetStockPrices(1, 10000, "", exchange, "ticker", "asc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Disposition", "attachment; filename=prices_export.json")
	c.JSON(http.StatusOK, result.Prices)
}

// GetSyncStatus handles GET /admin/api/prices/sync-status - returns sync status
func (ctrl *TCBSPriceController) GetSyncStatus(c *gin.Context) {
	isSyncing := ctrl.supabaseClient.IsPriceSyncing()
	lastSync, _ := ctrl.supabaseClient.GetPriceLastSyncTime()
	count := services.GlobalPriceStore.Count()

	c.JSON(http.StatusOK, gin.H{
		"is_syncing": isSyncing,
		"last_sync":  lastSync,
		"count":      count,
	})
}
