package controllers

import (
	"log"
	"net/http"
	"strconv"

	"go_backend_project/config"
	"go_backend_project/services"

	"github.com/gin-gonic/gin"
)

// CrawlerController handles stock data crawling operations
type CrawlerController struct {
	crawler *services.CrawlerService
}

// NewCrawlerController creates a new crawler controller
func NewCrawlerController() *CrawlerController {
	ctrl := &CrawlerController{}

	// Initialize crawler if MongoDB is available
	if config.GlobalDBConfig != nil && config.GlobalDBConfig.IsMongoDBAvailable() {
		ctrl.crawler = services.NewCrawlerService(
			config.GlobalDBConfig.MongoDB,
			config.GlobalDBConfig.MongoDBName,
		)
		log.Println("CrawlerController: Initialized with MongoDB")
	} else {
		log.Println("CrawlerController: MongoDB not available")
	}

	return ctrl
}

// CrawlStocks fetches stock list from VNDirect and saves to MongoDB
// POST /api/v1/crawler/stocks
func (cc *CrawlerController) CrawlStocks(c *gin.Context) {
	if cc.crawler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB crawler not available",
		})
		return
	}

	count, err := cc.crawler.CrawlStocks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Stock list crawled successfully",
		"stocks_count":  count,
	})
}

// CrawlPricesAndIndicators fetches price data and calculates indicators
// POST /api/v1/crawler/prices
func (cc *CrawlerController) CrawlPricesAndIndicators(c *gin.Context) {
	if cc.crawler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB crawler not available",
		})
		return
	}

	// Get stock codes from request body
	var request struct {
		StockCodes []string `json:"stock_codes"`
		Workers    int      `json:"workers"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(request.StockCodes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "stock_codes is required",
		})
		return
	}

	workers := request.Workers
	if workers <= 0 {
		workers = 5 // Default 5 workers
	}

	// Run crawler asynchronously
	go func() {
		err := cc.crawler.CrawlAndCalculatePrices(request.StockCodes, workers)
		if err != nil {
			log.Printf("Error crawling prices: %v", err)
		} else {
			log.Printf("Successfully crawled %d stocks", len(request.StockCodes))
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message":      "Price crawling started in background",
		"stocks_count": len(request.StockCodes),
		"workers":      workers,
	})
}

// CrawlAll crawls stock list and then prices for all stocks
// POST /api/v1/crawler/all
func (cc *CrawlerController) CrawlAll(c *gin.Context) {
	if cc.crawler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB crawler not available",
		})
		return
	}

	workers, _ := strconv.Atoi(c.DefaultQuery("workers", "5"))
	if workers <= 0 || workers > 10 {
		workers = 5
	}

	// Run in background
	go func() {
		// Step 1: Crawl stock list
		log.Println("Starting full crawl - Step 1: Crawling stock list...")
		count, err := cc.crawler.CrawlStocks()
		if err != nil {
			log.Printf("Error crawling stock list: %v", err)
			return
		}
		log.Printf("Crawled %d stocks", count)

		// Step 2: Get all stock codes from MongoDB stocks collection
		// TODO: Query MongoDB to get actual stock list instead of hardcoded list
		// For initial implementation, using top Vietnamese stocks
		stockCodes := []string{
			"VNM", "VIC", "VHM", "HPG", "TCB", "VCB", "BID", "CTG", "MBB", "ACB",
			"MSN", "VRE", "VPB", "PLX", "GAS", "SAB", "POW", "SSI", "FPT", "VHC",
		}

		log.Printf("Step 2: Crawling prices and indicators for %d stocks...", len(stockCodes))
		err = cc.crawler.CrawlAndCalculatePrices(stockCodes, workers)
		if err != nil {
			log.Printf("Error crawling prices: %v", err)
		} else {
			log.Printf("Full crawl completed successfully")
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Full crawl started in background",
		"workers": workers,
	})
}

// CreateIndexes creates MongoDB indexes
// POST /api/v1/crawler/indexes
func (cc *CrawlerController) CreateIndexes(c *gin.Context) {
	if cc.crawler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MongoDB crawler not available",
		})
		return
	}

	err := cc.crawler.CreateIndexes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "MongoDB indexes created successfully",
	})
}

// GetCrawlerStatus returns crawler status
// GET /api/v1/crawler/status
func (cc *CrawlerController) GetCrawlerStatus(c *gin.Context) {
	status := gin.H{
		"mongodb_available": cc.crawler != nil,
	}

	if cc.crawler != nil {
		status["status"] = "ready"
		status["message"] = "Crawler is ready to use"
	} else {
		status["status"] = "unavailable"
		status["message"] = "MongoDB not configured"
	}

	c.JSON(http.StatusOK, status)
}
