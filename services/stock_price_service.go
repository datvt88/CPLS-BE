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
	"sync/atomic"
	"time"
)

// Price data constants
const (
	VNDirectPriceAPIURL = "https://api-finfo.vndirect.com.vn/v4/stock_prices"
	StockPriceDir       = "data/stocks"
	PriceSyncConfigFile = "data/price_sync_config.json"
	DefaultPriceSize    = 270 // ~1 year of trading days
	DefaultWorkerCount  = 10  // Concurrent workers for fetching
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
	Indicators  *StockIndicators `json:"indicators,omitempty"`
}

// StockIndicators holds calculated technical indicators
type StockIndicators struct {
	RS3D      float64 `json:"rs_3d"`
	RS1M      float64 `json:"rs_1m"`
	RS3M      float64 `json:"rs_3m"`
	RS1Y      float64 `json:"rs_1y"`
	RSAvg     float64 `json:"rs_avg"`
	MACDHist  float64 `json:"macd_hist"`
	AvgVol    float64 `json:"avg_vol"`
	RSI       float64 `json:"rsi"`
	UpdatedAt string  `json:"updated_at"`
}

// PriceSyncConfig holds price sync configuration
type PriceSyncConfig struct {
	DelayMS        int    `json:"delay_ms"`
	BatchSize      int    `json:"batch_size"`
	BatchPauseMS   int    `json:"batch_pause_ms"`
	PriceSize      int    `json:"price_size"`
	WorkerCount    int    `json:"worker_count"`
	LastFullSync   string `json:"last_full_sync"`
	CurrentStock   string `json:"current_stock"`
	SyncInProgress bool   `json:"sync_in_progress"`
}

// PriceSyncProgress represents sync progress
type PriceSyncProgress struct {
	TotalStocks     int      `json:"total_stocks"`
	ProcessedStocks int      `json:"processed_stocks"`
	SuccessCount    int      `json:"success_count"`
	FailedCount     int      `json:"failed_count"`
	FailedStocks    []string `json:"failed_stocks"`
	CurrentStock    string   `json:"current_stock"`
	StartTime       string   `json:"start_time"`
	ElapsedTime     string   `json:"elapsed_time"`
	EstimatedTime   string   `json:"estimated_time"`
	Status          string   `json:"status"`
	WorkerCount     int      `json:"worker_count"`
}

// fetchJob represents a job for the worker pool
type fetchJob struct {
	code string
	size int
}

// fetchResult represents the result of a fetch job
type fetchResult struct {
	code    string
	prices  []StockPriceData
	err     error
}

// StockPriceService handles stock price fetching and storage
type StockPriceService struct {
	config     PriceSyncConfig
	progress   PriceSyncProgress
	mu         sync.RWMutex
	stopChan   chan struct{}
	isRunning  bool
	httpClient *http.Client

	// Atomic counters for concurrent updates
	successCount int64
	failedCount  int64
	processedCount int64
}

// Global price service instance
var GlobalPriceService *StockPriceService

// InitPriceService initializes the price service
func InitPriceService() error {
	GlobalPriceService = &StockPriceService{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		stopChan: make(chan struct{}),
	}

	if err := os.MkdirAll(StockPriceDir, 0755); err != nil {
		return fmt.Errorf("failed to create price directory: %w", err)
	}

	if err := GlobalPriceService.LoadConfig(); err != nil {
		log.Printf("No price sync config found, using defaults: %v", err)
		GlobalPriceService.config = PriceSyncConfig{
			DelayMS:      100,                // Reduced delay for concurrent
			BatchSize:    50,
			BatchPauseMS: 2000,               // Reduced pause
			PriceSize:    DefaultPriceSize,
			WorkerCount:  DefaultWorkerCount,
		}
	}

	// Ensure worker count is set
	if GlobalPriceService.config.WorkerCount == 0 {
		GlobalPriceService.config.WorkerCount = DefaultWorkerCount
	}

	log.Printf("Stock Price Service initialized (workers: %d)", GlobalPriceService.config.WorkerCount)
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

// SetWorkerCount sets the number of concurrent workers
func (s *StockPriceService) SetWorkerCount(count int) {
	if count < 1 {
		count = 1
	}
	if count > 20 {
		count = 20
	}
	s.mu.Lock()
	s.config.WorkerCount = count
	s.mu.Unlock()
	s.SaveConfig()
}

// GetProgress returns current sync progress
func (s *StockPriceService) GetProgress() PriceSyncProgress {
	s.mu.RLock()
	progress := s.progress
	s.mu.RUnlock()

	// Update with atomic counters
	progress.SuccessCount = int(atomic.LoadInt64(&s.successCount))
	progress.FailedCount = int(atomic.LoadInt64(&s.failedCount))
	progress.ProcessedStocks = int(atomic.LoadInt64(&s.processedCount))

	return progress
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

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
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

// SaveStockPrice saves price data to file, DuckDB, and MongoDB
func (s *StockPriceService) SaveStockPrice(code string, prices []StockPriceData) error {
	priceFile := StockPriceFile{
		Code:        code,
		LastUpdated: time.Now().Format(time.RFC3339),
		DataCount:   len(prices),
		Prices:      prices,
	}

	data, err := json.MarshalIndent(priceFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal price data: %w", err)
	}

	filePath := filepath.Join(StockPriceDir, fmt.Sprintf("%s.json", code))
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write price file: %w", err)
	}

	// Save to DuckDB
	if GlobalDuckDB != nil {
		if err := GlobalDuckDB.SavePriceHistory(code, prices); err != nil {
			log.Printf("Warning: failed to save %s prices to DuckDB: %v", code, err)
		}
	}

	// Save to MongoDB Atlas (async to not block)
	go func() {
		if GlobalMongoClient != nil && GlobalMongoClient.IsConfigured() {
			if err := GlobalMongoClient.SavePriceData(code, &priceFile); err != nil {
				log.Printf("Warning: failed to save %s prices to MongoDB: %v", code, err)
			}
		}
	}()

	return nil
}

// LoadStockPrice loads price data from file, DuckDB, or MongoDB Atlas
func (s *StockPriceService) LoadStockPrice(code string) (*StockPriceFile, error) {
	filePath := filepath.Join(StockPriceDir, fmt.Sprintf("%s.json", code))

	// Try local file first (fastest)
	data, err := os.ReadFile(filePath)
	if err == nil {
		var priceFile StockPriceFile
		if err := json.Unmarshal(data, &priceFile); err == nil {
			return &priceFile, nil
		}
	}

	// Try DuckDB (local database)
	if GlobalDuckDB != nil {
		prices, err := GlobalDuckDB.LoadPriceHistory(code, 270)
		if err == nil && len(prices) > 0 {
			priceFile := &StockPriceFile{
				Code:        code,
				LastUpdated: time.Now().Format(time.RFC3339),
				DataCount:   len(prices),
				Prices:      prices,
			}
			if cacheData, err := json.MarshalIndent(priceFile, "", "  "); err == nil {
				os.WriteFile(filePath, cacheData, 0644)
			}
			return priceFile, nil
		}
	}

	// Fallback to MongoDB Atlas (persists across deploys)
	if GlobalMongoClient != nil && GlobalMongoClient.IsConfigured() {
		priceFile, err := GlobalMongoClient.LoadPriceData(code)
		if err == nil && priceFile != nil && len(priceFile.Prices) > 0 {
			// Cache to local file and DuckDB for faster future reads
			if cacheData, err := json.MarshalIndent(priceFile, "", "  "); err == nil {
				os.WriteFile(filePath, cacheData, 0644)
			}
			if GlobalDuckDB != nil {
				GlobalDuckDB.SavePriceHistory(code, priceFile.Prices)
			}
			return priceFile, nil
		}
	}

	return nil, fmt.Errorf("price data not found for %s", code)
}

// StartFullSync starts syncing prices for all stocks using worker pool
func (s *StockPriceService) StartFullSync() error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("sync already in progress")
	}
	s.isRunning = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	go s.runFullSyncConcurrent()
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

// worker processes fetch jobs from the job channel
func (s *StockPriceService) worker(id int, jobs <-chan fetchJob, results chan<- fetchResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		// Check if stopped
		select {
		case <-s.stopChan:
			return
		default:
		}

		// Small delay to avoid rate limiting
		time.Sleep(time.Duration(s.config.DelayMS) * time.Millisecond)

		priceResp, err := s.FetchStockPrice(job.code, job.size)
		if err != nil {
			results <- fetchResult{code: job.code, err: err}
			continue
		}

		if len(priceResp.Data) > 0 {
			results <- fetchResult{code: job.code, prices: priceResp.Data}
		} else {
			results <- fetchResult{code: job.code, err: fmt.Errorf("no data")}
		}
	}
}

// resultProcessor processes results and saves them
func (s *StockPriceService) resultProcessor(results <-chan fetchResult, failedStocks *[]string, failedMu *sync.Mutex, done chan<- bool) {
	for result := range results {
		atomic.AddInt64(&s.processedCount, 1)

		if result.err != nil {
			atomic.AddInt64(&s.failedCount, 1)
			failedMu.Lock()
			*failedStocks = append(*failedStocks, result.code)
			failedMu.Unlock()
			continue
		}

		if err := s.SaveStockPrice(result.code, result.prices); err != nil {
			atomic.AddInt64(&s.failedCount, 1)
			failedMu.Lock()
			*failedStocks = append(*failedStocks, result.code)
			failedMu.Unlock()
		} else {
			atomic.AddInt64(&s.successCount, 1)
		}
	}
	done <- true
}

// runFullSyncConcurrent performs concurrent sync using worker pool
func (s *StockPriceService) runFullSyncConcurrent() {
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

	// Reset atomic counters
	atomic.StoreInt64(&s.successCount, 0)
	atomic.StoreInt64(&s.failedCount, 0)
	atomic.StoreInt64(&s.processedCount, 0)

	// Get worker count
	s.mu.RLock()
	workerCount := s.config.WorkerCount
	priceSize := s.config.PriceSize
	s.mu.RUnlock()

	if workerCount == 0 {
		workerCount = DefaultWorkerCount
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
		WorkerCount:     workerCount,
	}
	s.config.SyncInProgress = true
	s.mu.Unlock()

	log.Printf("Starting concurrent price sync for %d stocks with %d workers", len(stocks), workerCount)

	// Create channels
	jobs := make(chan fetchJob, len(stocks))
	results := make(chan fetchResult, len(stocks))
	done := make(chan bool)

	// Track failed stocks
	var failedStocks []string
	var failedMu sync.Mutex

	// Start result processor
	go s.resultProcessor(results, &failedStocks, &failedMu, done)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go s.worker(i, jobs, results, &wg)
	}

	// Send jobs
	go func() {
		for _, stock := range stocks {
			select {
			case <-s.stopChan:
				break
			case jobs <- fetchJob{code: stock.Code, size: priceSize}:
			}
		}
		close(jobs)
	}()

	// Start progress updater
	progressDone := make(chan bool)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-progressDone:
				return
			case <-ticker.C:
				processed := int(atomic.LoadInt64(&s.processedCount))
				elapsed := time.Since(startTime)

				s.mu.Lock()
				s.progress.ProcessedStocks = processed
				s.progress.SuccessCount = int(atomic.LoadInt64(&s.successCount))
				s.progress.FailedCount = int(atomic.LoadInt64(&s.failedCount))
				s.progress.ElapsedTime = elapsed.Round(time.Second).String()

				if processed > 0 {
					avgTime := elapsed / time.Duration(processed)
					remaining := avgTime * time.Duration(len(stocks)-processed)
					s.progress.EstimatedTime = remaining.Round(time.Second).String()
				}
				s.mu.Unlock()
			}
		}
	}()

	// Wait for workers to finish
	wg.Wait()
	close(results)

	// Wait for result processor to finish
	<-done
	close(progressDone)

	// Final update
	s.mu.Lock()
	s.isRunning = false
	s.progress.Status = "completed"
	s.progress.ProcessedStocks = int(atomic.LoadInt64(&s.processedCount))
	s.progress.SuccessCount = int(atomic.LoadInt64(&s.successCount))
	s.progress.FailedCount = int(atomic.LoadInt64(&s.failedCount))
	s.progress.ElapsedTime = time.Since(startTime).Round(time.Second).String()
	s.progress.FailedStocks = failedStocks
	s.config.SyncInProgress = false
	s.config.LastFullSync = time.Now().Format(time.RFC3339)
	s.mu.Unlock()

	s.SaveConfig()

	log.Printf("Price sync completed: success=%d, failed=%d, time=%s (workers: %d)",
		s.progress.SuccessCount, s.progress.FailedCount, s.progress.ElapsedTime, workerCount)
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
		"total_files":  totalFiles,
		"last_sync":    s.config.LastFullSync,
		"worker_count": s.config.WorkerCount,
	}

	if !oldestUpdate.IsZero() {
		stats["oldest_update"] = oldestUpdate.Format(time.RFC3339)
	}
	if !newestUpdate.IsZero() {
		stats["newest_update"] = newestUpdate.Format(time.RFC3339)
	}

	return stats, nil
}

// HasLocalPriceData checks if local price data exists
func (s *StockPriceService) HasLocalPriceData() bool {
	files, err := os.ReadDir(StockPriceDir)
	if err != nil {
		return false
	}

	count := 0
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			count++
			if count >= 10 { // At least 10 price files exist
				return true
			}
		}
	}
	return false
}

// RestoreFromMongoDB restores all price data from MongoDB Atlas
// This should be called on startup if local data is missing (after redeploy)
func (s *StockPriceService) RestoreFromMongoDB() error {
	if GlobalMongoClient == nil || !GlobalMongoClient.IsConfigured() {
		return fmt.Errorf("MongoDB not configured")
	}

	log.Println("Restoring price data from MongoDB Atlas...")

	priceFiles, err := GlobalMongoClient.LoadAllPriceData()
	if err != nil {
		return fmt.Errorf("failed to load price data from MongoDB: %w", err)
	}

	if len(priceFiles) == 0 {
		return fmt.Errorf("no price data found in MongoDB")
	}

	// Ensure directory exists
	os.MkdirAll(StockPriceDir, 0755)

	savedCount := 0
	for code, priceFile := range priceFiles {
		if priceFile == nil || len(priceFile.Prices) == 0 {
			continue
		}

		// Save to local file
		data, err := json.MarshalIndent(priceFile, "", "  ")
		if err != nil {
			continue
		}

		filePath := filepath.Join(StockPriceDir, fmt.Sprintf("%s.json", code))
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			continue
		}

		// Also save to DuckDB if available
		if GlobalDuckDB != nil {
			GlobalDuckDB.SavePriceHistory(code, priceFile.Prices)
		}

		savedCount++
	}

	log.Printf("Restored price data for %d stocks from MongoDB Atlas", savedCount)
	return nil
}

// SyncAllToMongoDB syncs all local price data to MongoDB Atlas
func (s *StockPriceService) SyncAllToMongoDB() error {
	if GlobalMongoClient == nil || !GlobalMongoClient.IsConfigured() {
		return fmt.Errorf("MongoDB not configured")
	}

	files, err := os.ReadDir(StockPriceDir)
	if err != nil {
		return fmt.Errorf("failed to read price directory: %w", err)
	}

	priceFiles := make(map[string]*StockPriceFile)
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		code := file.Name()[:len(file.Name())-5]
		priceFile, err := s.LoadStockPrice(code)
		if err != nil {
			continue
		}
		priceFiles[code] = priceFile
	}

	if len(priceFiles) == 0 {
		return fmt.Errorf("no price files to sync")
	}

	if err := GlobalMongoClient.SaveAllPriceData(priceFiles); err != nil {
		return fmt.Errorf("failed to sync price data to MongoDB: %w", err)
	}

	log.Printf("Synced %d price files to MongoDB Atlas", len(priceFiles))
	return nil
}
