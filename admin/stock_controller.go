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
