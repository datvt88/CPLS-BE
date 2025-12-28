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
	"time"
)

// VNDirectAPIURL is the endpoint for fetching stock list
const VNDirectAPIURL = "https://api-finfo.vndirect.com.vn/v4/stocks?q=type:stock~status:listed~floor:HOSE,HNX,UPCOM&size=9999"

// StockListFile is the local file for stocks data
const StockListFile = "data/stocks_list.json"

// publicAPIEnabled controls whether the public API is enabled
var publicAPIEnabled = true

// SetPublicAPIEnabled sets the public API enabled state
func SetPublicAPIEnabled(enabled bool) {
	publicAPIEnabled = enabled
	if enabled {
		log.Println("Public API enabled")
	} else {
		log.Println("Public API disabled")
	}
}

// IsPublicAPIEnabled returns whether the public API is enabled
func IsPublicAPIEnabled() bool {
	return publicAPIEnabled
}

// VNDirectResponse represents the response from VNDirect API
type VNDirectResponse struct {
	Data []VNDirectStock `json:"data"`
}

// VNDirectStock represents a stock from VNDirect API
type VNDirectStock struct {
	Code           string `json:"code"`
	Type           string `json:"type"`
	Floor          string `json:"floor"`
	ISIN           string `json:"isin"`
	Status         string `json:"status"`
	CompanyName    string `json:"companyName"`
	CompanyNameEng string `json:"companyNameEng"`
	ShortName      string `json:"shortName"`
	ShortNameEng   string `json:"shortNameEng"`
	ListedDate     string `json:"listedDate"`
	DelistedDate   string `json:"delistedDate"`
	CompanyID      string `json:"companyId"`
	TaxCode        string `json:"taxCode"`
}

// Stock represents a stock (compatible with existing code)
type Stock struct {
	ID             string     `json:"id"`
	Code           string     `json:"code"`
	Type           string     `json:"type"`
	Floor          string     `json:"floor"`
	ISIN           string     `json:"isin"`
	Status         string     `json:"status"`
	CompanyName    string     `json:"company_name"`
	CompanyNameEng string     `json:"company_name_eng"`
	ShortName      string     `json:"short_name"`
	ShortNameEng   string     `json:"short_name_eng"`
	ListedDate     string     `json:"listed_date"`
	DelistedDate   string     `json:"delisted_date"`
	CompanyID      string     `json:"company_id"`
	TaxCode        string     `json:"tax_code"`
	IsActive       bool       `json:"is_active"`
	LastSyncAt     *time.Time `json:"last_sync_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// StockListResponse contains paginated stock results
type StockListResponse struct {
	Stocks     []Stock `json:"stocks"`
	Total      int64   `json:"total"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	TotalPages int     `json:"total_pages"`
}

// StockSyncResult contains the result of stock sync operation
type StockSyncResult struct {
	TotalFetched int      `json:"total_fetched"`
	Created      int      `json:"created"`
	Updated      int      `json:"updated"`
	Errors       []string `json:"errors"`
	SyncedAt     string   `json:"synced_at"`
}

// FetchStocksFromVNDirect fetches stock list from VNDirect API
func FetchStocksFromVNDirect() ([]VNDirectStock, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", VNDirectAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("VNDirect API error: %v, trying fallback storage...", err)
		return LoadStocksWithFallback()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("VNDirect API error (status %d): %s, trying fallback storage...", resp.StatusCode, string(body))
		return LoadStocksWithFallback()
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response VNDirectResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	log.Printf("VNDirect API fetched %d stocks", len(response.Data))

	// Save to local file and MongoDB for persistence
	go func() {
		SaveStocksToFile(response.Data)
		// Save to MongoDB Atlas for persistence across deploys
		if GlobalMongoClient != nil && GlobalMongoClient.IsConfigured() {
			if err := GlobalMongoClient.SaveStockList(response.Data); err != nil {
				log.Printf("Warning: failed to save stock list to MongoDB: %v", err)
			} else {
				log.Println("Stock list saved to MongoDB Atlas")
			}
		}
	}()

	return response.Data, nil
}

// LoadStocksFromFile loads stocks from local JSON file
func LoadStocksFromFile() ([]VNDirectStock, error) {
	data, err := os.ReadFile(StockListFile)
	if err != nil {
		return nil, fmt.Errorf("stock list file not found: %w. Please provide data/stocks_list.json or import via admin", err)
	}

	var stocks []VNDirectStock
	if err := json.Unmarshal(data, &stocks); err != nil {
		return nil, fmt.Errorf("failed to parse stock list file: %w", err)
	}

	log.Printf("Loaded %d stocks from file: %s", len(stocks), StockListFile)
	return stocks, nil
}

// LoadStocksWithFallback loads stocks from local file first, then MongoDB Atlas as fallback
func LoadStocksWithFallback() ([]VNDirectStock, error) {
	// Try local file first (fastest)
	stocks, err := LoadStocksFromFile()
	if err == nil && len(stocks) > 0 {
		return stocks, nil
	}

	// Fallback to MongoDB Atlas (persists across deploys)
	if GlobalMongoClient != nil && GlobalMongoClient.IsConfigured() {
		log.Println("Local stock list not found, loading from MongoDB Atlas...")
		stocks, err := GlobalMongoClient.LoadStockList()
		if err == nil && len(stocks) > 0 {
			// Cache to local file for faster future reads
			go SaveStocksToFile(stocks)
			return stocks, nil
		}
		if err != nil {
			log.Printf("MongoDB stock list load failed: %v", err)
		}
	}

	return nil, fmt.Errorf("stock list not found in local storage or MongoDB Atlas")
}

// SaveStocksToFile saves stocks to local JSON file
func SaveStocksToFile(stocks []VNDirectStock) error {
	dir := filepath.Dir(StockListFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	data, err := json.MarshalIndent(stocks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stocks: %w", err)
	}

	if err := os.WriteFile(StockListFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write stock list file: %w", err)
	}

	log.Printf("Saved %d stocks to file: %s", len(stocks), StockListFile)
	return nil
}

// ImportStocksFromJSON imports stocks from provided JSON data
func ImportStocksFromJSON(jsonData []byte) (*StockSyncResult, error) {
	result := &StockSyncResult{
		Errors:   []string{},
		SyncedAt: time.Now().UTC().Format(time.RFC3339),
	}

	var stocks []VNDirectStock
	if err := json.Unmarshal(jsonData, &stocks); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	result.TotalFetched = len(stocks)

	// Save to local file
	if err := SaveStocksToFile(stocks); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to save to local file: %v", err))
	}

	// Save to MongoDB Atlas
	if GlobalMongoClient != nil && GlobalMongoClient.IsConfigured() {
		if err := GlobalMongoClient.SaveStockList(stocks); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to save to MongoDB: %v", err))
		} else {
			result.Created = len(stocks)
			log.Println("Stock list saved to MongoDB Atlas")
		}
	} else {
		result.Errors = append(result.Errors, "MongoDB not configured")
	}

	log.Printf("Stock import completed: imported=%d, errors=%d", result.TotalFetched, len(result.Errors))
	return result, nil
}

// syncStocksFromVNDirectInternal syncs stocks from VNDirect API to MongoDB
func syncStocksFromVNDirectInternal() (*StockSyncResult, error) {
	result := &StockSyncResult{
		Errors:   []string{},
		SyncedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Fetch stocks from VNDirect
	stocks, err := FetchStocksFromVNDirect()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stocks from VNDirect: %w", err)
	}

	result.TotalFetched = len(stocks)

	// Save to MongoDB Atlas
	if GlobalMongoClient != nil && GlobalMongoClient.IsConfigured() {
		if err := GlobalMongoClient.SaveStockList(stocks); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to save to MongoDB: %v", err))
		} else {
			result.Created = len(stocks)
			log.Println("Stock list synced to MongoDB Atlas")
		}
	} else {
		result.Errors = append(result.Errors, "MongoDB not configured")
	}

	log.Printf("Stock sync completed: fetched=%d, created=%d, errors=%d",
		result.TotalFetched, result.Created, len(result.Errors))

	return result, nil
}

// === Wrapper methods for SupabaseDBClient compatibility ===

// GetStocks fetches stocks with pagination (using local file + MongoDB)
func (c *SupabaseDBClient) GetStocks(page, pageSize int, search, floor, sortBy, sortOrder string) (*StockListResponse, error) {
	// Load all stocks from fallback chain
	vnStocks, err := LoadStocksWithFallback()
	if err != nil {
		return &StockListResponse{Stocks: []Stock{}, Total: 0, Page: page, PageSize: pageSize, TotalPages: 0}, nil
	}

	// Convert to Stock and apply filters
	var filteredStocks []Stock
	for _, s := range vnStocks {
		stock := Stock{
			ID:             s.Code,
			Code:           s.Code,
			Type:           s.Type,
			Floor:          s.Floor,
			ISIN:           s.ISIN,
			Status:         s.Status,
			CompanyName:    s.CompanyName,
			CompanyNameEng: s.CompanyNameEng,
			ShortName:      s.ShortName,
			ShortNameEng:   s.ShortNameEng,
			ListedDate:     s.ListedDate,
			DelistedDate:   s.DelistedDate,
			CompanyID:      s.CompanyID,
			TaxCode:        s.TaxCode,
			IsActive:       s.Status == "listed",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		// Apply search filter
		if search != "" {
			searchLower := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(stock.Code), searchLower) &&
				!strings.Contains(strings.ToLower(stock.CompanyName), searchLower) &&
				!strings.Contains(strings.ToLower(stock.ShortName), searchLower) {
				continue
			}
		}

		// Apply floor filter
		if floor != "" && stock.Floor != floor {
			continue
		}

		filteredStocks = append(filteredStocks, stock)
	}

	// Sort
	if sortBy != "" {
		sort.Slice(filteredStocks, func(i, j int) bool {
			var less bool
			switch sortBy {
			case "code":
				less = filteredStocks[i].Code < filteredStocks[j].Code
			case "company_name":
				less = filteredStocks[i].CompanyName < filteredStocks[j].CompanyName
			case "floor":
				less = filteredStocks[i].Floor < filteredStocks[j].Floor
			default:
				less = filteredStocks[i].Code < filteredStocks[j].Code
			}
			if sortOrder == "desc" {
				return !less
			}
			return less
		})
	}

	total := int64(len(filteredStocks))

	// Paginate
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(filteredStocks) {
		start = len(filteredStocks)
	}
	if end > len(filteredStocks) {
		end = len(filteredStocks)
	}

	paginatedStocks := filteredStocks[start:end]

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &StockListResponse{
		Stocks:     paginatedStocks,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetStockByCode fetches a single stock by code
func (c *SupabaseDBClient) GetStockByCode(code string) (*Stock, error) {
	stocks, err := LoadStocksWithFallback()
	if err != nil {
		return nil, err
	}

	code = strings.ToUpper(code)
	for _, s := range stocks {
		if strings.ToUpper(s.Code) == code {
			return &Stock{
				ID:             s.Code,
				Code:           s.Code,
				Type:           s.Type,
				Floor:          s.Floor,
				ISIN:           s.ISIN,
				Status:         s.Status,
				CompanyName:    s.CompanyName,
				CompanyNameEng: s.CompanyNameEng,
				ShortName:      s.ShortName,
				ShortNameEng:   s.ShortNameEng,
				ListedDate:     s.ListedDate,
				DelistedDate:   s.DelistedDate,
				CompanyID:      s.CompanyID,
				TaxCode:        s.TaxCode,
				IsActive:       s.Status == "listed",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}, nil
		}
	}

	return nil, fmt.Errorf("stock %s not found", code)
}

// GetStockCount returns the total count of stocks
func (c *SupabaseDBClient) GetStockCount() (int64, error) {
	stocks, err := LoadStocksWithFallback()
	if err != nil {
		return 0, nil
	}
	return int64(len(stocks)), nil
}

// GetStockStats returns statistics about stocks
func (c *SupabaseDBClient) GetStockStats() (map[string]interface{}, error) {
	stocks, err := LoadStocksWithFallback()
	if err != nil {
		return map[string]interface{}{"total": 0, "by_floor": map[string]int64{}}, nil
	}

	byFloor := make(map[string]int64)
	for _, s := range stocks {
		byFloor[s.Floor]++
	}

	return map[string]interface{}{
		"total":    int64(len(stocks)),
		"by_floor": byFloor,
	}, nil
}

// SyncStocksFromVNDirect syncs stocks from VNDirect API to MongoDB (wrapper for SupabaseDBClient)
func (c *SupabaseDBClient) SyncStocksFromVNDirect() (*StockSyncResult, error) {
	return syncStocksFromVNDirectInternal()
}

// DeleteStock removes a stock (not supported with file-based storage)
func (c *SupabaseDBClient) DeleteStock(code string) error {
	log.Printf("DeleteStock called for %s - operation not supported in file-based storage", code)
	return nil
}

// GetLastSyncTime returns the last sync time for stocks
func (c *SupabaseDBClient) GetLastSyncTime() (*time.Time, error) {
	// Check file modification time
	info, err := os.Stat(StockListFile)
	if err != nil {
		return nil, nil
	}
	modTime := info.ModTime()
	return &modTime, nil
}
