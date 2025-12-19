package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SSI API URL for stock prices (TCBS API returns 404)
const SSIPriceAPIURL = "https://iboard-query.ssi.com.vn/v2/stock/group/"

// PriceDataFile is the path to persist price data
const PriceDataFile = "data/prices.json"

// PriceDataStore represents persisted price data
type PriceDataStore struct {
	LastSyncAt *time.Time    `json:"last_sync_at"`
	Prices     []*StockPrice `json:"prices"`
}

// SSIPriceResponse represents the response from SSI API
type SSIPriceResponse struct {
	Data []SSIPriceData `json:"data"`
}

// SSIPriceData represents price data from SSI API
type SSIPriceData struct {
	SS            string  `json:"ss"`           // Stock symbol
	SN            string  `json:"sn"`           // Short name
	ST            string  `json:"st"`           // Exchange (hose, hnx, upcom)
	CP            float64 `json:"cp"`           // Ceiling price
	FP            float64 `json:"fp"`           // Floor price
	RP            float64 `json:"rp"`           // Reference price
	OP            float64 `json:"op"`           // Open price
	HP            float64 `json:"hp"`           // Highest price
	LP            float64 `json:"lp"`           // Lowest price
	MP            float64 `json:"mp"`           // Match price (current price)
	MQ            float64 `json:"mq"`           // Match quantity
	CG            float64 `json:"cg"`           // Change
	PCT           float64 `json:"pct"`          // Percent change
	TVOL          float64 `json:"tvol"`         // Total volume
	TVAL          float64 `json:"tval"`         // Total value
	FBQ           float64 `json:"fbq"`          // Foreign buy quantity
	FSQ           float64 `json:"fsq"`          // Foreign sell quantity
	MC            float64 `json:"mc"`           // Market cap
}

// TCBSPriceResponse represents the response from TCBS API
type TCBSPriceResponse struct {
	Data []TCBSPriceData `json:"data"`
}

// TCBSPriceData represents price data from TCBS API
type TCBSPriceData struct {
	Ticker           string  `json:"ticker"`
	Exchange         string  `json:"exchange"`
	Price            float64 `json:"price"`
	PriceChange      float64 `json:"priceChange"`
	PriceChangeRatio float64 `json:"priceChangeRatio"`
	Vol              float64 `json:"vol"`
	HighestPrice     float64 `json:"highestPrice"`
	LowestPrice      float64 `json:"lowestPrice"`
	OpenPrice        float64 `json:"openPrice"`
	ClosePrice       float64 `json:"closePrice"`
	RefPrice         float64 `json:"refPrice"`
	CeilingPrice     float64 `json:"ceilingPrice"`
	FloorPrice       float64 `json:"floorPrice"`
	ForeignBuyVol    float64 `json:"foreignBuyVol"`
	ForeignSellVol   float64 `json:"foreignSellVol"`
	TotalVal         float64 `json:"totalVal"`
	MarketCap        float64 `json:"marketCap"`
	PE               float64 `json:"pe"`
	PB               float64 `json:"pb"`
	EPS              float64 `json:"eps"`
	BVPS             float64 `json:"bvps"`
	ROE              float64 `json:"roe"`
	ROA              float64 `json:"roa"`
	Beta             float64 `json:"beta"`
	Week52High       float64 `json:"week52High"`
	Week52Low        float64 `json:"week52Low"`
	SharesOutstanding float64 `json:"sharesOutstanding"`
}

// StockPrice represents stored price data
type StockPrice struct {
	Ticker           string    `json:"ticker"`
	Exchange         string    `json:"exchange"`
	Price            float64   `json:"price"`
	PriceChange      float64   `json:"price_change"`
	PriceChangeRatio float64   `json:"price_change_ratio"`
	Vol              float64   `json:"vol"`
	HighestPrice     float64   `json:"highest_price"`
	LowestPrice      float64   `json:"lowest_price"`
	OpenPrice        float64   `json:"open_price"`
	ClosePrice       float64   `json:"close_price"`
	RefPrice         float64   `json:"ref_price"`
	CeilingPrice     float64   `json:"ceiling_price"`
	FloorPrice       float64   `json:"floor_price"`
	ForeignBuyVol    float64   `json:"foreign_buy_vol"`
	ForeignSellVol   float64   `json:"foreign_sell_vol"`
	TotalVal         float64   `json:"total_val"`
	MarketCap        float64   `json:"market_cap"`
	PE               float64   `json:"pe"`
	PB               float64   `json:"pb"`
	EPS              float64   `json:"eps"`
	BVPS             float64   `json:"bvps"`
	ROE              float64   `json:"roe"`
	ROA              float64   `json:"roa"`
	Beta             float64   `json:"beta"`
	Week52High       float64   `json:"week52_high"`
	Week52Low        float64   `json:"week52_low"`
	SharesOutstanding float64  `json:"shares_outstanding"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// StockPriceListResponse contains paginated stock price results
type StockPriceListResponse struct {
	Prices     []StockPrice `json:"prices"`
	Total      int64        `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"page_size"`
	TotalPages int          `json:"total_pages"`
}

// PriceSyncResult contains the result of price sync operation
type PriceSyncResult struct {
	TotalTickers int      `json:"total_tickers"`
	TotalChunks  int      `json:"total_chunks"`
	ChunkSize    int      `json:"chunk_size"`
	Fetched      int      `json:"fetched"`
	Failed       int      `json:"failed"`
	Errors       []string `json:"errors"`
	SyncedAt     string   `json:"synced_at"`
	Duration     string   `json:"duration"`
}

// InMemoryPriceStore stores stock prices in memory
type InMemoryPriceStore struct {
	mu         sync.RWMutex
	prices     map[string]*StockPrice // key = ticker
	lastSyncAt *time.Time
	isSyncing  bool
}

// Global in-memory price store
var GlobalPriceStore = NewInMemoryPriceStore()

// NewInMemoryPriceStore creates a new in-memory price store and loads from file if exists
func NewInMemoryPriceStore() *InMemoryPriceStore {
	store := &InMemoryPriceStore{
		prices: make(map[string]*StockPrice),
	}
	// Try to load from file on startup
	if err := store.LoadFromFile(); err != nil {
		log.Printf("No existing price data file or error loading: %v", err)
	} else {
		log.Printf("Loaded %d prices from file", len(store.prices))
	}
	return store
}

// SaveToFile saves all prices to a JSON file
func (s *InMemoryPriceStore) SaveToFile() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create data directory if not exists
	dir := filepath.Dir(PriceDataFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Convert map to slice
	prices := make([]*StockPrice, 0, len(s.prices))
	for _, price := range s.prices {
		prices = append(prices, price)
	}

	data := PriceDataStore{
		LastSyncAt: s.lastSyncAt,
		Prices:     prices,
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal price data: %w", err)
	}

	// Write to file
	if err := os.WriteFile(PriceDataFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write price file: %w", err)
	}

	log.Printf("Saved %d prices to %s", len(prices), PriceDataFile)
	return nil
}

// LoadFromFile loads prices from a JSON file
func (s *InMemoryPriceStore) LoadFromFile() error {
	// Check if file exists
	if _, err := os.Stat(PriceDataFile); os.IsNotExist(err) {
		return fmt.Errorf("price data file not found: %s", PriceDataFile)
	}

	// Read file
	jsonData, err := os.ReadFile(PriceDataFile)
	if err != nil {
		return fmt.Errorf("failed to read price file: %w", err)
	}

	// Unmarshal JSON
	var data PriceDataStore
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("failed to unmarshal price data: %w", err)
	}

	// Load into memory
	s.mu.Lock()
	defer s.mu.Unlock()

	s.prices = make(map[string]*StockPrice)
	for _, price := range data.Prices {
		s.prices[price.Ticker] = price
	}
	s.lastSyncAt = data.LastSyncAt

	return nil
}

// chunkSlice splits a slice into chunks of specified size
func chunkSlice(slice []string, chunkSize int) [][]string {
	var chunks [][]string
	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}
	return chunks
}

// FetchPricesFromSSI fetches all prices from SSI API by exchange
func FetchPricesFromSSI(exchanges []string) ([]SSIPriceData, error) {
	if len(exchanges) == 0 {
		exchanges = []string{"hose", "hnx", "upcom"}
	}

	transport := &http.Transport{
		DisableCompression: true,
	}
	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	url := SSIPriceAPIURL + strings.Join(exchanges, ",")
	log.Printf("SSI API request: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("SSI API request failed: %v", err)
		return nil, fmt.Errorf("failed to fetch from SSI: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("SSI API response: status=%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("SSI API error: status=%d, body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("SSI API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response SSIPriceResponse
	if err := json.Unmarshal(body, &response); err != nil {
		// Try parsing as array directly
		var dataArray []SSIPriceData
		if err2 := json.Unmarshal(body, &dataArray); err2 != nil {
			log.Printf("SSI API parse error: %v, body preview: %s", err, string(body)[:min(200, len(body))])
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return dataArray, nil
	}

	log.Printf("SSI API fetched %d records", len(response.Data))
	return response.Data, nil
}

// min returns the smaller of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetAllTickers returns all ticker codes from stock store
func GetAllTickers() []string {
	stocks := GlobalStockStore.GetAll()
	tickers := make([]string, 0, len(stocks))
	for _, stock := range stocks {
		tickers = append(tickers, stock.Code)
	}
	sort.Strings(tickers)
	return tickers
}

// IsSyncing returns whether a sync is in progress
func (s *InMemoryPriceStore) IsSyncing() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isSyncing
}

// SyncFromTCBS syncs all stock prices from SSI API (renamed for compatibility)
func (s *InMemoryPriceStore) SyncFromTCBS(chunkSize int, delayMs int) (*PriceSyncResult, error) {
	// Check if already syncing
	s.mu.Lock()
	if s.isSyncing {
		s.mu.Unlock()
		return nil, fmt.Errorf("sync already in progress")
	}
	s.isSyncing = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isSyncing = false
		s.mu.Unlock()
	}()

	startTime := time.Now()
	result := &PriceSyncResult{
		ChunkSize: chunkSize,
		Errors:    []string{},
		SyncedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	// Fetch all prices from SSI API (one call for all exchanges)
	exchanges := []string{"hose", "hnx", "upcom"}
	result.TotalChunks = len(exchanges)

	allPrices, err := FetchPricesFromSSI(exchanges)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		result.Duration = time.Since(startTime).String()
		return result, err
	}

	result.TotalTickers = len(allPrices)

	// Store all prices
	for _, priceData := range allPrices {
		s.UpsertSSIPrice(&priceData)
		result.Fetched++
	}

	// Update last sync time
	now := time.Now()
	s.mu.Lock()
	s.lastSyncAt = &now
	s.mu.Unlock()

	// Save to file for persistence
	if err := s.SaveToFile(); err != nil {
		log.Printf("Warning: failed to save prices to file: %v", err)
		result.Errors = append(result.Errors, fmt.Sprintf("failed to save to file: %v", err))
	}

	result.Duration = time.Since(startTime).String()
	log.Printf("SSI sync completed: fetched=%d, duration=%s", result.Fetched, result.Duration)
	return result, nil
}

// UpsertPrice adds or updates a price (for TCBS data - kept for compatibility)
func (s *InMemoryPriceStore) UpsertPrice(data *TCBSPriceData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ticker := strings.ToUpper(data.Ticker)
	s.prices[ticker] = &StockPrice{
		Ticker:           ticker,
		Exchange:         data.Exchange,
		Price:            data.Price,
		PriceChange:      data.PriceChange,
		PriceChangeRatio: data.PriceChangeRatio,
		Vol:              data.Vol,
		HighestPrice:     data.HighestPrice,
		LowestPrice:      data.LowestPrice,
		OpenPrice:        data.OpenPrice,
		ClosePrice:       data.ClosePrice,
		RefPrice:         data.RefPrice,
		CeilingPrice:     data.CeilingPrice,
		FloorPrice:       data.FloorPrice,
		ForeignBuyVol:    data.ForeignBuyVol,
		ForeignSellVol:   data.ForeignSellVol,
		TotalVal:         data.TotalVal,
		MarketCap:        data.MarketCap,
		PE:               data.PE,
		PB:               data.PB,
		EPS:              data.EPS,
		BVPS:             data.BVPS,
		ROE:              data.ROE,
		ROA:              data.ROA,
		Beta:             data.Beta,
		Week52High:       data.Week52High,
		Week52Low:        data.Week52Low,
		SharesOutstanding: data.SharesOutstanding,
		UpdatedAt:        time.Now(),
	}
}

// UpsertSSIPrice adds or updates a price from SSI API data
func (s *InMemoryPriceStore) UpsertSSIPrice(data *SSIPriceData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ticker := strings.ToUpper(data.SS)
	exchange := strings.ToUpper(data.ST)

	s.prices[ticker] = &StockPrice{
		Ticker:           ticker,
		Exchange:         exchange,
		Price:            data.MP,                    // Match price
		PriceChange:      data.CG,                    // Change
		PriceChangeRatio: data.PCT,                   // Percent change
		Vol:              data.TVOL,                  // Total volume
		HighestPrice:     data.HP,                    // Highest price
		LowestPrice:      data.LP,                    // Lowest price
		OpenPrice:        data.OP,                    // Open price
		ClosePrice:       data.MP,                    // Using match price as close
		RefPrice:         data.RP,                    // Reference price
		CeilingPrice:     data.CP,                    // Ceiling price
		FloorPrice:       data.FP,                    // Floor price
		ForeignBuyVol:    data.FBQ,                   // Foreign buy quantity
		ForeignSellVol:   data.FSQ,                   // Foreign sell quantity
		TotalVal:         data.TVAL,                  // Total value
		MarketCap:        data.MC,                    // Market cap
		UpdatedAt:        time.Now(),
	}
}

// GetPrices returns paginated prices with search and filter
func (s *InMemoryPriceStore) GetPrices(page, pageSize int, search, exchange, sortBy, sortOrder string) *StockPriceListResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	// Filter prices
	var filtered []StockPrice
	searchLower := strings.ToLower(search)

	for _, price := range s.prices {
		// Exchange filter
		if exchange != "" && exchange != "all" && price.Exchange != exchange {
			continue
		}

		// Search filter by ticker
		if search != "" {
			tickerLower := strings.ToLower(price.Ticker)
			if !strings.Contains(tickerLower, searchLower) {
				continue
			}
		}

		filtered = append(filtered, *price)
	}

	// Sort prices
	sort.Slice(filtered, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "ticker":
			less = filtered[i].Ticker < filtered[j].Ticker
		case "price":
			less = filtered[i].Price < filtered[j].Price
		case "price_change":
			less = filtered[i].PriceChange < filtered[j].PriceChange
		case "price_change_ratio":
			less = filtered[i].PriceChangeRatio < filtered[j].PriceChangeRatio
		case "vol":
			less = filtered[i].Vol < filtered[j].Vol
		case "market_cap":
			less = filtered[i].MarketCap < filtered[j].MarketCap
		case "pe":
			less = filtered[i].PE < filtered[j].PE
		default:
			less = filtered[i].Ticker < filtered[j].Ticker
		}

		if sortOrder == "desc" {
			return !less
		}
		return less
	})

	// Pagination
	total := int64(len(filtered))
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}

	return &StockPriceListResponse{
		Prices:     filtered[start:end],
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// GetByTicker returns a price by ticker
func (s *InMemoryPriceStore) GetByTicker(ticker string) (*StockPrice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	price, exists := s.prices[strings.ToUpper(ticker)]
	if !exists {
		return nil, fmt.Errorf("price not found for ticker: %s", ticker)
	}
	return price, nil
}

// GetAll returns all prices
func (s *InMemoryPriceStore) GetAll() []StockPrice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prices := make([]StockPrice, 0, len(s.prices))
	for _, price := range s.prices {
		prices = append(prices, *price)
	}
	return prices
}

// Count returns the number of prices stored
func (s *InMemoryPriceStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.prices)
}

// GetLastSyncTime returns the last sync time
func (s *InMemoryPriceStore) GetLastSyncTime() *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncAt
}

// GetStats returns statistics about prices
func (s *InMemoryPriceStore) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total"] = len(s.prices)

	exchangeCounts := map[string]int64{}
	gainers := 0
	losers := 0
	unchanged := 0

	for _, price := range s.prices {
		// Count by exchange
		exchangeCounts[price.Exchange]++

		// Count gainers/losers
		if price.PriceChange > 0 {
			gainers++
		} else if price.PriceChange < 0 {
			losers++
		} else {
			unchanged++
		}
	}

	stats["by_exchange"] = exchangeCounts
	stats["gainers"] = gainers
	stats["losers"] = losers
	stats["unchanged"] = unchanged

	if s.lastSyncAt != nil {
		stats["last_sync"] = s.lastSyncAt.Format(time.RFC3339)
	}

	return stats
}

// Clear clears all prices
func (s *InMemoryPriceStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prices = make(map[string]*StockPrice)
	s.lastSyncAt = nil
}

// GetTopGainers returns top N gainers by price change ratio
func (s *InMemoryPriceStore) GetTopGainers(limit int) []StockPrice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prices := make([]StockPrice, 0, len(s.prices))
	for _, p := range s.prices {
		if p.PriceChangeRatio > 0 {
			prices = append(prices, *p)
		}
	}

	sort.Slice(prices, func(i, j int) bool {
		return prices[i].PriceChangeRatio > prices[j].PriceChangeRatio
	})

	if limit > 0 && len(prices) > limit {
		prices = prices[:limit]
	}

	return prices
}

// GetTopLosers returns top N losers by price change ratio
func (s *InMemoryPriceStore) GetTopLosers(limit int) []StockPrice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prices := make([]StockPrice, 0, len(s.prices))
	for _, p := range s.prices {
		if p.PriceChangeRatio < 0 {
			prices = append(prices, *p)
		}
	}

	sort.Slice(prices, func(i, j int) bool {
		return prices[i].PriceChangeRatio < prices[j].PriceChangeRatio
	})

	if limit > 0 && len(prices) > limit {
		prices = prices[:limit]
	}

	return prices
}

// GetTopVolume returns top N stocks by volume
func (s *InMemoryPriceStore) GetTopVolume(limit int) []StockPrice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prices := make([]StockPrice, 0, len(s.prices))
	for _, p := range s.prices {
		prices = append(prices, *p)
	}

	sort.Slice(prices, func(i, j int) bool {
		return prices[i].Vol > prices[j].Vol
	})

	if limit > 0 && len(prices) > limit {
		prices = prices[:limit]
	}

	return prices
}

// === Wrapper methods for SupabaseDBClient compatibility ===

// GetStockPrices fetches stock prices (wrapper for compatibility)
func (c *SupabaseDBClient) GetStockPrices(page, pageSize int, search, exchange, sortBy, sortOrder string) (*StockPriceListResponse, error) {
	return GlobalPriceStore.GetPrices(page, pageSize, search, exchange, sortBy, sortOrder), nil
}

// GetStockPrice fetches a single stock price by ticker
func (c *SupabaseDBClient) GetStockPrice(ticker string) (*StockPrice, error) {
	return GlobalPriceStore.GetByTicker(ticker)
}

// SyncStockPricesFromTCBS syncs stock prices from TCBS API
func (c *SupabaseDBClient) SyncStockPricesFromTCBS(chunkSize, delayMs int) (*PriceSyncResult, error) {
	return GlobalPriceStore.SyncFromTCBS(chunkSize, delayMs)
}

// GetPriceStats returns statistics about stock prices
func (c *SupabaseDBClient) GetPriceStats() (map[string]interface{}, error) {
	return GlobalPriceStore.GetStats(), nil
}

// GetPriceLastSyncTime returns the last sync time for prices
func (c *SupabaseDBClient) GetPriceLastSyncTime() (*time.Time, error) {
	return GlobalPriceStore.GetLastSyncTime(), nil
}

// IsPriceSyncing returns whether a price sync is in progress
func (c *SupabaseDBClient) IsPriceSyncing() bool {
	return GlobalPriceStore.IsSyncing()
}
