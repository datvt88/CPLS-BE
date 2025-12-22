package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// VNDirectAPIURL is the endpoint for fetching stock list
const VNDirectAPIURL = "https://api-finfo.vndirect.com.vn/v4/stocks?q=type:stock~status:listed~floor:HOSE,HNX,UPCOM&size=9999"

// StockListFile is the local file for stocks data
const StockListFile = "data/stocks_list.json"

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
		log.Printf("VNDirect API error: %v, trying local file...", err)
		return LoadStocksFromFile()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("VNDirect API error (status %d): %s, trying local file...", resp.StatusCode, string(body))
		return LoadStocksFromFile()
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

	// Save to local file for future offline use
	go SaveStocksToFile(response.Data)

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

	if GlobalDuckDB == nil {
		return nil, errors.New("DuckDB not initialized")
	}

	var stocks []VNDirectStock
	if err := json.Unmarshal(jsonData, &stocks); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	result.TotalFetched = len(stocks)

	// Upsert each stock to DuckDB
	for _, stock := range stocks {
		duckStock := &DuckDBStock{
			Code:           strings.ToUpper(stock.Code),
			Type:           stock.Type,
			Floor:          stock.Floor,
			ISIN:           stock.ISIN,
			Status:         stock.Status,
			CompanyName:    stock.CompanyName,
			CompanyNameEng: stock.CompanyNameEng,
			ShortName:      stock.ShortName,
			ShortNameEng:   stock.ShortNameEng,
			ListedDate:     stock.ListedDate,
			DelistedDate:   stock.DelistedDate,
			CompanyID:      stock.CompanyID,
			TaxCode:        stock.TaxCode,
			IsActive:       stock.Status == "listed",
		}

		if err := GlobalDuckDB.UpsertStock(duckStock); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to upsert %s: %v", stock.Code, err))
			continue
		}
		result.Created++
	}

	// Save to local file for future use
	SaveStocksToFile(stocks)

	// Save sync history
	errStr := ""
	if len(result.Errors) > 0 {
		errStr = strings.Join(result.Errors, "; ")
	}
	GlobalDuckDB.SaveSyncHistory("stocks_import", result.TotalFetched, result.Created, 0, errStr)

	log.Printf("Stock import completed: imported=%d, errors=%d", result.TotalFetched, len(result.Errors))
	return result, nil
}

// SyncStocksFromVNDirectToDuckDB syncs stocks from VNDirect API to DuckDB
func SyncStocksFromVNDirectToDuckDB() (*StockSyncResult, error) {
	result := &StockSyncResult{
		Errors:   []string{},
		SyncedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if GlobalDuckDB == nil {
		return nil, errors.New("DuckDB not initialized")
	}

	// Fetch stocks from VNDirect
	stocks, err := FetchStocksFromVNDirect()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stocks from VNDirect: %w", err)
	}

	result.TotalFetched = len(stocks)

	// Get existing count
	existingCount, _ := GlobalDuckDB.GetStockCount()

	// Upsert each stock to DuckDB
	for _, stock := range stocks {
		duckStock := &DuckDBStock{
			Code:           strings.ToUpper(stock.Code),
			Type:           stock.Type,
			Floor:          stock.Floor,
			ISIN:           stock.ISIN,
			Status:         stock.Status,
			CompanyName:    stock.CompanyName,
			CompanyNameEng: stock.CompanyNameEng,
			ShortName:      stock.ShortName,
			ShortNameEng:   stock.ShortNameEng,
			ListedDate:     stock.ListedDate,
			DelistedDate:   stock.DelistedDate,
			CompanyID:      stock.CompanyID,
			TaxCode:        stock.TaxCode,
			IsActive:       stock.Status == "listed",
		}

		if err := GlobalDuckDB.UpsertStock(duckStock); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to upsert %s: %v", stock.Code, err))
			continue
		}

		// Count created vs updated
		if existingCount == 0 {
			result.Created++
		} else {
			result.Updated++
		}
	}

	// Save sync history
	errStr := ""
	if len(result.Errors) > 0 {
		errStr = strings.Join(result.Errors, "; ")
	}
	GlobalDuckDB.SaveSyncHistory("stocks", result.TotalFetched, result.Created, result.Updated, errStr)

	log.Printf("Stock sync completed: fetched=%d, created=%d, updated=%d, errors=%d",
		result.TotalFetched, result.Created, result.Updated, len(result.Errors))

	return result, nil
}

// === Wrapper methods for SupabaseDBClient compatibility ===

// GetStocks fetches stocks from DuckDB (wrapper for compatibility)
func (c *SupabaseDBClient) GetStocks(page, pageSize int, search, floor, sortBy, sortOrder string) (*StockListResponse, error) {
	if GlobalDuckDB == nil {
		return &StockListResponse{Stocks: []Stock{}, Total: 0, Page: page, PageSize: pageSize, TotalPages: 0}, nil
	}

	stocks, total, err := GlobalDuckDB.GetStocksPaginated(page, pageSize, search, floor, sortBy, sortOrder)
	if err != nil {
		return nil, err
	}

	// Convert DuckDBStock to Stock
	result := make([]Stock, len(stocks))
	for i, s := range stocks {
		result[i] = Stock{
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
			IsActive:       s.IsActive,
			CreatedAt:      s.CreatedAt,
			UpdatedAt:      s.UpdatedAt,
			LastSyncAt:     s.LastSyncAt,
		}
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &StockListResponse{
		Stocks:     result,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetStockByCode fetches a single stock by code from DuckDB
func (c *SupabaseDBClient) GetStockByCode(code string) (*Stock, error) {
	if GlobalDuckDB == nil {
		return nil, errors.New("DuckDB not initialized")
	}

	s, err := GlobalDuckDB.GetStockByCode(code)
	if err != nil {
		return nil, err
	}

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
		IsActive:       s.IsActive,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
		LastSyncAt:     s.LastSyncAt,
	}, nil
}

// GetStockCount returns the total count of stocks from DuckDB
func (c *SupabaseDBClient) GetStockCount() (int64, error) {
	if GlobalDuckDB == nil {
		return 0, nil
	}
	return GlobalDuckDB.GetStockCount()
}

// GetStockStats returns statistics about stocks from DuckDB
func (c *SupabaseDBClient) GetStockStats() (map[string]interface{}, error) {
	if GlobalDuckDB == nil {
		return map[string]interface{}{"total": 0, "by_floor": map[string]int64{}}, nil
	}

	total, err := GlobalDuckDB.GetStockCount()
	if err != nil {
		return nil, err
	}

	byFloor, err := GlobalDuckDB.GetStockCountByFloor()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total":    total,
		"by_floor": byFloor,
	}, nil
}

// SyncStocksFromVNDirect syncs stocks from VNDirect API to DuckDB
func (c *SupabaseDBClient) SyncStocksFromVNDirect() (*StockSyncResult, error) {
	return SyncStocksFromVNDirectToDuckDB()
}

// DeleteStock deletes a stock by code from DuckDB
func (c *SupabaseDBClient) DeleteStock(code string) error {
	if GlobalDuckDB == nil {
		return errors.New("DuckDB not initialized")
	}
	return GlobalDuckDB.DeleteStock(code)
}

// GetLastSyncTime returns the last sync time for stocks from DuckDB
func (c *SupabaseDBClient) GetLastSyncTime() (*time.Time, error) {
	if GlobalDuckDB == nil {
		return nil, nil
	}
	return GlobalDuckDB.GetLastSyncTime("stocks")
}
