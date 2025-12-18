package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// VNDirectAPIURL is the endpoint for fetching stock list
const VNDirectAPIURL = "https://api-finfo.vndirect.com.vn/v4/stocks?q=type:stock~status:listed~floor:HOSE,HNX,UPCOM&size=9999"

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

// Stock represents a stock in the in-memory storage
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

// InMemoryStockStore stores stocks in memory
type InMemoryStockStore struct {
	mu         sync.RWMutex
	stocks     map[string]*Stock // key = stock code
	lastSyncAt *time.Time
}

// Global in-memory stock store
var GlobalStockStore = NewInMemoryStockStore()

// NewInMemoryStockStore creates a new in-memory stock store
func NewInMemoryStockStore() *InMemoryStockStore {
	return &InMemoryStockStore{
		stocks: make(map[string]*Stock),
	}
}

// FetchStocksFromVNDirect fetches stock list from VNDirect API
func FetchStocksFromVNDirect() ([]VNDirectStock, error) {
	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest("GET", VNDirectAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from VNDirect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("VNDirect API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response VNDirectResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Data, nil
}

// GetAll returns all stocks
func (s *InMemoryStockStore) GetAll() []Stock {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stocks := make([]Stock, 0, len(s.stocks))
	for _, stock := range s.stocks {
		stocks = append(stocks, *stock)
	}
	return stocks
}

// GetStocks returns paginated stocks with search and filter
func (s *InMemoryStockStore) GetStocks(page, pageSize int, search, floor, sortBy, sortOrder string) *StockListResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	// Filter stocks
	var filtered []Stock
	searchLower := strings.ToLower(search)

	for _, stock := range s.stocks {
		// Floor filter
		if floor != "" && floor != "all" && stock.Floor != floor {
			continue
		}

		// Search filter
		if search != "" {
			codeLower := strings.ToLower(stock.Code)
			nameLower := strings.ToLower(stock.CompanyName)
			shortNameLower := strings.ToLower(stock.ShortName)

			if !strings.Contains(codeLower, searchLower) &&
				!strings.Contains(nameLower, searchLower) &&
				!strings.Contains(shortNameLower, searchLower) {
				continue
			}
		}

		filtered = append(filtered, *stock)
	}

	// Sort stocks
	sort.Slice(filtered, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "code":
			less = filtered[i].Code < filtered[j].Code
		case "floor":
			less = filtered[i].Floor < filtered[j].Floor
		case "company_name":
			less = filtered[i].CompanyName < filtered[j].CompanyName
		case "listed_date":
			less = filtered[i].ListedDate < filtered[j].ListedDate
		default:
			less = filtered[i].Code < filtered[j].Code
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

	return &StockListResponse{
		Stocks:     filtered[start:end],
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// GetByCode returns a stock by code
func (s *InMemoryStockStore) GetByCode(code string) (*Stock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stock, exists := s.stocks[strings.ToUpper(code)]
	if !exists {
		return nil, errors.New("stock not found")
	}
	return stock, nil
}

// Upsert adds or updates a stock
func (s *InMemoryStockStore) Upsert(vnStock *VNDirectStock) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	code := strings.ToUpper(vnStock.Code)

	existing, exists := s.stocks[code]
	if exists {
		// Update existing
		existing.Type = vnStock.Type
		existing.Floor = vnStock.Floor
		existing.ISIN = vnStock.ISIN
		existing.Status = vnStock.Status
		existing.CompanyName = vnStock.CompanyName
		existing.CompanyNameEng = vnStock.CompanyNameEng
		existing.ShortName = vnStock.ShortName
		existing.ShortNameEng = vnStock.ShortNameEng
		existing.ListedDate = vnStock.ListedDate
		existing.DelistedDate = vnStock.DelistedDate
		existing.CompanyID = vnStock.CompanyID
		existing.TaxCode = vnStock.TaxCode
		existing.IsActive = vnStock.Status == "listed"
		existing.LastSyncAt = &now
		existing.UpdatedAt = now
	} else {
		// Create new
		s.stocks[code] = &Stock{
			ID:             code,
			Code:           code,
			Type:           vnStock.Type,
			Floor:          vnStock.Floor,
			ISIN:           vnStock.ISIN,
			Status:         vnStock.Status,
			CompanyName:    vnStock.CompanyName,
			CompanyNameEng: vnStock.CompanyNameEng,
			ShortName:      vnStock.ShortName,
			ShortNameEng:   vnStock.ShortNameEng,
			ListedDate:     vnStock.ListedDate,
			DelistedDate:   vnStock.DelistedDate,
			CompanyID:      vnStock.CompanyID,
			TaxCode:        vnStock.TaxCode,
			IsActive:       vnStock.Status == "listed",
			LastSyncAt:     &now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}
}

// Delete removes a stock by code
func (s *InMemoryStockStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	code = strings.ToUpper(code)
	if _, exists := s.stocks[code]; !exists {
		return errors.New("stock not found")
	}
	delete(s.stocks, code)
	return nil
}

// GetStats returns statistics about stocks
func (s *InMemoryStockStore) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total"] = len(s.stocks)

	floorCounts := map[string]int64{
		"HOSE":  0,
		"HNX":   0,
		"UPCOM": 0,
	}

	for _, stock := range s.stocks {
		if count, ok := floorCounts[stock.Floor]; ok {
			floorCounts[stock.Floor] = count + 1
		}
	}

	stats["by_floor"] = floorCounts
	return stats
}

// GetLastSyncTime returns the last sync time
func (s *InMemoryStockStore) GetLastSyncTime() *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncAt
}

// SetLastSyncTime sets the last sync time
func (s *InMemoryStockStore) SetLastSyncTime(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSyncAt = &t
}

// Count returns the number of stocks
func (s *InMemoryStockStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.stocks)
}

// SyncFromVNDirect syncs stocks from VNDirect API
func (s *InMemoryStockStore) SyncFromVNDirect() (*StockSyncResult, error) {
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

	// Get existing count before sync
	existingCount := s.Count()

	// Upsert each stock
	for _, stock := range stocks {
		_, exists := s.stocks[strings.ToUpper(stock.Code)]
		s.Upsert(&stock)

		if !exists {
			result.Created++
		} else {
			result.Updated++
		}
	}

	// Update last sync time
	s.SetLastSyncTime(time.Now())

	// If this was first sync, all are created
	if existingCount == 0 {
		result.Created = result.TotalFetched
		result.Updated = 0
	}

	return result, nil
}

// === Wrapper methods for SupabaseDBClient compatibility ===

// GetStocks fetches stocks (wrapper for compatibility)
func (c *SupabaseDBClient) GetStocks(page, pageSize int, search, floor, sortBy, sortOrder string) (*StockListResponse, error) {
	return GlobalStockStore.GetStocks(page, pageSize, search, floor, sortBy, sortOrder), nil
}

// GetStockByCode fetches a single stock by code
func (c *SupabaseDBClient) GetStockByCode(code string) (*Stock, error) {
	return GlobalStockStore.GetByCode(code)
}

// GetStockCount returns the total count of stocks
func (c *SupabaseDBClient) GetStockCount() (int64, error) {
	return int64(GlobalStockStore.Count()), nil
}

// GetStockStats returns statistics about stocks
func (c *SupabaseDBClient) GetStockStats() (map[string]interface{}, error) {
	return GlobalStockStore.GetStats(), nil
}

// SyncStocksFromVNDirect syncs stocks from VNDirect API
func (c *SupabaseDBClient) SyncStocksFromVNDirect() (*StockSyncResult, error) {
	return GlobalStockStore.SyncFromVNDirect()
}

// DeleteStock deletes a stock by code
func (c *SupabaseDBClient) DeleteStock(code string) error {
	return GlobalStockStore.Delete(code)
}

// GetLastSyncTime returns the last sync time for stocks
func (c *SupabaseDBClient) GetLastSyncTime() (*time.Time, error) {
	return GlobalStockStore.GetLastSyncTime(), nil
}
