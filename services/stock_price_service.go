package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Price data constants
const (
	VNDirectPriceAPIURL = "https://api-finfo.vndirect.com.vn/v4/stock_prices"
	StockPriceDir       = "data/stocks"
	PriceSyncConfigFile = "data/price_sync_config.json"
	DefaultPriceSize    = 270 // ~1 year of trading days
)

// VNDirectPriceResponse represents the API response
type VNDirectPriceResponse struct {
	Data          []StockPriceData `json:"data"`
	CurrentPage   int              `json:"currentPage"`
	Size          int              `json:"size"`
	TotalElements int              `json:"totalElements"`
	TotalPages    int              `json:"totalPages"`
}

// StockPriceData represents daily price data from VNDirect
type StockPriceData struct {
	Code         string  `json:"code"`
	Date         string  `json:"date"`
	Time         string  `json:"time"`
	Floor        string  `json:"floor"`
	Type         string  `json:"type"`
	BasicPrice   float64 `json:"basicPrice"`
	CeilingPrice float64 `json:"ceilingPrice"`
	FloorPrice   float64 `json:"floorPrice"`
	Open         float64 `json:"open"`
	High         float64 `json:"high"`
	Low          float64 `json:"low"`
	Close        float64 `json:"close"`
	Average      float64 `json:"average"`
	AdOpen       float64 `json:"adOpen"`
	AdHigh       float64 `json:"adHigh"`
	AdLow        float64 `json:"adLow"`
	AdClose      float64 `json:"adClose"`
	AdAverage    float64 `json:"adAverage"`
	NmVolume     float64 `json:"nmVolume"`
	NmValue      float64 `json:"nmValue"`
	PtVolume     float64 `json:"ptVolume"`
	PtValue      float64 `json:"ptValue"`
	Change       float64 `json:"change"`
	AdChange     float64 `json:"adChange"`
	PctChange    float64 `json:"pctChange"`
}

// StockPriceFile represents the stored price file with metadata
type StockPriceFile struct {
	Code        string           `json:"code"`
	LastUpdated string           `json:"last_updated"`
	DataCount   int              `json:"data_count"`
	Prices      []StockPriceData `json:"prices"`
	// Technical indicators (to be calculated)
	Indicators *StockIndicators `json:"indicators,omitempty"`
}

// StockIndicators holds calculated technical indicators
type StockIndicators struct {
	RS3D     float64 `json:"rs_3d"`     // Relative Strength 3 days
	RS1M     float64 `json:"rs_1m"`     // Relative Strength 1 month
	RS3M     float64 `json:"rs_3m"`     // Relative Strength 3 months
	RS1Y     float64 `json:"rs_1y"`     // Relative Strength 1 year
	RSAvg    float64 `json:"rs_avg"`    // Average of all RS
	MACDHist float64 `json:"macd_hist"` // MACD Histogram
	AvgVol   float64 `json:"avg_vol"`   // Average Volume (20 days)
	RSI      float64 `json:"rsi"`       // RSI (14 days)
	UpdatedAt string `json:"updated_at"`
}

// PriceSyncConfig holds price sync configuration
type PriceSyncConfig struct {
	DelayMS        int    `json:"delay_ms"`         // Delay between requests in milliseconds
	BatchSize      int    `json:"batch_size"`       // Number of stocks per batch
	BatchPauseMS   int    `json:"batch_pause_ms"`   // Pause between batches
	PriceSize      int    `json:"price_size"`       // Number of price records to fetch
	LastFullSync   string `json:"last_full_sync"`   // Last full sync timestamp
	CurrentStock   string `json:"current_stock"`    // Current stock being synced (for resume)
	SyncInProgress bool   `json:"sync_in_progress"` // Whether sync is in progress
}

// PriceSyncProgress represents sync progress
type PriceSyncProgress struct {
	TotalStocks   int      `json:"total_stocks"`
	ProcessedStocks int    `json:"processed_stocks"`
	SuccessCount  int      `json:"success_count"`
	FailedCount   int      `json:"failed_count"`
	FailedStocks  []string `json:"failed_stocks"`
	CurrentStock  string   `json:"current_stock"`
	StartTime     string   `json:"start_time"`
	ElapsedTime   string   `json:"elapsed_time"`
	EstimatedTime string   `json:"estimated_time"`
	Status        string   `json:"status"` // "running", "completed", "stopped", "error"
}

// StockPriceService handles stock price fetching and storage
type StockPriceService struct {
	config      PriceSyncConfig
	progress    PriceSyncProgress
	mu          sync.RWMutex
	stopChan    chan struct{}
	isRunning   bool
	httpClient  *http.Client
}

// Global price service instance
var GlobalPriceService *StockPriceService

// InitPriceService initializes the price service
func InitPriceService() error {
	GlobalPriceService = &StockPriceService{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		stopChan:   make(chan struct{}),
	}

	// Create price directory
	if err := os.MkdirAll(StockPriceDir, 0755); err != nil {
		return fmt.Errorf("failed to create price directory: %w", err)
	}

	// Load config
	if err := GlobalPriceService.LoadConfig(); err != nil {
		log.Printf("No price sync config found, using defaults: %v", err)
		GlobalPriceService.config = PriceSyncConfig{
			DelayMS:      500,  // 500ms between requests
			BatchSize:    50,   // 50 stocks per batch
			BatchPauseMS: 5000, // 5 second pause between batches
			PriceSize:    270,  // ~1 year
		}
	}

	log.Println("Stock Price Service initialized")
	return nil
}

// LoadConfig loads price sync config from file
func (s *StockPriceService) LoadConfig() error {
	data, err := os.ReadFile(PriceSyncConfigFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.config)
}

// SaveConfig saves price sync config to file
func (s *StockPriceService) SaveConfig() error {
	dir := filepath.Dir(PriceSyncConfigFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(PriceSyncConfigFile, data, 0644)
}

// GetConfig returns the current config
func (s *StockPriceService) GetConfig() PriceSyncConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// UpdateConfig updates the config
func (s *StockPriceService) UpdateConfig(delayMS, batchSize, batchPauseMS, priceSize int) error {
	s.mu.Lock()
	s.config.DelayMS = delayMS
	s.config.BatchSize = batchSize
	s.config.BatchPauseMS = batchPauseMS
	s.config.PriceSize = priceSize
	s.mu.Unlock()

	return s.SaveConfig()
}

// GetProgress returns current sync progress
func (s *StockPriceService) GetProgress() PriceSyncProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.progress
}

// IsRunning returns whether sync is running
func (s *StockPriceService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// FetchStockPrice fetches price data for a single stock
func (s *StockPriceService) FetchStockPrice(code string, size int) (*VNDirectPriceResponse, error) {
	url := fmt.Sprintf("%s?sort=date:desc&q=code:%s&size=%d", VNDirectPriceAPIURL, code, size)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://www.vndirect.com.vn/")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var priceResp VNDirectPriceResponse
	if err := json.Unmarshal(body, &priceResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &priceResp, nil
}

// SaveStockPrice saves price data to file and optionally to Supabase
func (s *StockPriceService) SaveStockPrice(code string, prices []StockPriceData) error {
	priceFile := StockPriceFile{
		Code:        code,
		LastUpdated: time.Now().Format(time.RFC3339),
		DataCount:   len(prices),
		Prices:      prices,
	}

	// Save to local file
	data, err := json.MarshalIndent(priceFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal price data: %w", err)
	}

	filePath := filepath.Join(StockPriceDir, fmt.Sprintf("%s.json", code))
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write price file: %w", err)
	}

	// Also save to local DuckDB database
	if GlobalDuckDB != nil {
		if err := GlobalDuckDB.SavePriceHistory(code, prices); err != nil {
			log.Printf("Warning: failed to save %s prices to DuckDB: %v", code, err)
		}
	}

	return nil
}

// LoadStockPrice loads price data from file or DuckDB
func (s *StockPriceService) LoadStockPrice(code string) (*StockPriceFile, error) {
	filePath := filepath.Join(StockPriceDir, fmt.Sprintf("%s.json", code))

	// Try local JSON file first (fastest)
	data, err := os.ReadFile(filePath)
	if err == nil {
		var priceFile StockPriceFile
		if err := json.Unmarshal(data, &priceFile); err == nil {
			return &priceFile, nil
		}
	}

	// Fallback to DuckDB if local file not found
	if GlobalDuckDB != nil {
		prices, err := GlobalDuckDB.LoadPriceHistory(code, 270)
		if err == nil && len(prices) > 0 {
			priceFile := &StockPriceFile{
				Code:        code,
				LastUpdated: time.Now().Format(time.RFC3339),
				DataCount:   len(prices),
				Prices:      prices,
			}
			// Cache to local JSON file for faster future reads
			if cacheData, err := json.MarshalIndent(priceFile, "", "  "); err == nil {
				os.WriteFile(filePath, cacheData, 0644)
			}
			return priceFile, nil
		}
	}

	return nil, fmt.Errorf("price data not found for %s", code)
}

// StartFullSync starts syncing prices for all stocks
func (s *StockPriceService) StartFullSync() error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("sync already in progress")
	}
	s.isRunning = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	go s.runFullSync()
	return nil
}

// StopSync stops the current sync
func (s *StockPriceService) StopSync() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}

	close(s.stopChan)
	s.isRunning = false
	s.progress.Status = "stopped"
	log.Println("Price sync stopped by user")
}

// runFullSync performs the actual sync
func (s *StockPriceService) runFullSync() {
	startTime := time.Now()

	// Load stock list
	stocks, err := LoadStocksFromFile()
	if err != nil {
		s.mu.Lock()
		s.isRunning = false
		s.progress.Status = "error"
		s.mu.Unlock()
		log.Printf("Failed to load stock list: %v", err)
		return
	}

	// Initialize progress
	s.mu.Lock()
	s.progress = PriceSyncProgress{
		TotalStocks:     len(stocks),
		ProcessedStocks: 0,
		SuccessCount:    0,
		FailedCount:     0,
		FailedStocks:    []string{},
		StartTime:       startTime.Format(time.RFC3339),
		Status:          "running",
	}
	s.config.SyncInProgress = true
	s.mu.Unlock()

	log.Printf("Starting price sync for %d stocks", len(stocks))

	batchCount := 0
	for i, stock := range stocks {
		// Check if stopped
		select {
		case <-s.stopChan:
			s.mu.Lock()
			s.isRunning = false
			s.config.SyncInProgress = false
			s.config.CurrentStock = stock.Code
			s.mu.Unlock()
			s.SaveConfig()
			return
		default:
		}

		// Update progress
		s.mu.Lock()
		s.progress.CurrentStock = stock.Code
		s.progress.ProcessedStocks = i
		elapsed := time.Since(startTime)
		s.progress.ElapsedTime = elapsed.Round(time.Second).String()

		// Estimate remaining time
		if i > 0 {
			avgTime := elapsed / time.Duration(i)
			remaining := avgTime * time.Duration(len(stocks)-i)
			s.progress.EstimatedTime = remaining.Round(time.Second).String()
		}
		s.mu.Unlock()

		// Fetch price
		priceResp, err := s.FetchStockPrice(stock.Code, s.config.PriceSize)
		if err != nil {
			log.Printf("Failed to fetch price for %s: %v", stock.Code, err)
			s.mu.Lock()
			s.progress.FailedCount++
			s.progress.FailedStocks = append(s.progress.FailedStocks, stock.Code)
			s.mu.Unlock()
		} else if len(priceResp.Data) > 0 {
			// Save price data
			if err := s.SaveStockPrice(stock.Code, priceResp.Data); err != nil {
				log.Printf("Failed to save price for %s: %v", stock.Code, err)
				s.mu.Lock()
				s.progress.FailedCount++
				s.progress.FailedStocks = append(s.progress.FailedStocks, stock.Code)
				s.mu.Unlock()
			} else {
				s.mu.Lock()
				s.progress.SuccessCount++
				s.mu.Unlock()
			}
		}

		batchCount++

		// Delay between requests
		time.Sleep(time.Duration(s.config.DelayMS) * time.Millisecond)

		// Batch pause
		if batchCount >= s.config.BatchSize {
			log.Printf("Batch complete (%d/%d), pausing for %dms...", i+1, len(stocks), s.config.BatchPauseMS)
			time.Sleep(time.Duration(s.config.BatchPauseMS) * time.Millisecond)
			batchCount = 0
		}
	}

	// Complete
	s.mu.Lock()
	s.isRunning = false
	s.progress.Status = "completed"
	s.progress.ProcessedStocks = len(stocks)
	s.progress.ElapsedTime = time.Since(startTime).Round(time.Second).String()
	s.config.SyncInProgress = false
	s.config.LastFullSync = time.Now().Format(time.RFC3339)
	s.mu.Unlock()

	s.SaveConfig()

	log.Printf("Price sync completed: success=%d, failed=%d, time=%s",
		s.progress.SuccessCount, s.progress.FailedCount, s.progress.ElapsedTime)
}

// SyncSingleStock syncs price for a single stock
func (s *StockPriceService) SyncSingleStock(code string) (*StockPriceFile, error) {
	priceResp, err := s.FetchStockPrice(code, s.config.PriceSize)
	if err != nil {
		return nil, err
	}

	if len(priceResp.Data) == 0 {
		return nil, fmt.Errorf("no price data found for %s", code)
	}

	if err := s.SaveStockPrice(code, priceResp.Data); err != nil {
		return nil, err
	}

	return s.LoadStockPrice(code)
}

// GetPriceSyncStats returns statistics about price data
func (s *StockPriceService) GetPriceSyncStats() (map[string]interface{}, error) {
	files, err := os.ReadDir(StockPriceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{
				"total_files":   0,
				"last_sync":     nil,
				"oldest_update": nil,
				"newest_update": nil,
			}, nil
		}
		return nil, err
	}

	var oldestUpdate, newestUpdate time.Time
	totalFiles := 0

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}
		totalFiles++

		filePath := filepath.Join(StockPriceDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var priceFile StockPriceFile
		if err := json.Unmarshal(data, &priceFile); err != nil {
			continue
		}

		updateTime, err := time.Parse(time.RFC3339, priceFile.LastUpdated)
		if err != nil {
			continue
		}

		if oldestUpdate.IsZero() || updateTime.Before(oldestUpdate) {
			oldestUpdate = updateTime
		}
		if newestUpdate.IsZero() || updateTime.After(newestUpdate) {
			newestUpdate = updateTime
		}
	}

	stats := map[string]interface{}{
		"total_files": totalFiles,
		"last_sync":   s.config.LastFullSync,
	}

	if !oldestUpdate.IsZero() {
		stats["oldest_update"] = oldestUpdate.Format(time.RFC3339)
	}
	if !newestUpdate.IsZero() {
		stats["newest_update"] = newestUpdate.Format(time.RFC3339)
	}

	return stats, nil
}
