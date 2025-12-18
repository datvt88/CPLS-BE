package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// Stock represents a stock in the database
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

// GetStocks fetches stocks from Supabase with pagination and search
func (c *SupabaseDBClient) GetStocks(page, pageSize int, search, floor, sortBy, sortOrder string) (*StockListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// Build query URL
	queryURL := fmt.Sprintf("%s/rest/v1/stocks?select=*&order=%s.%s&limit=%d&offset=%d",
		c.URL, sortBy, sortOrder, pageSize, offset)

	// Add filters
	filters := []string{}

	if search != "" {
		filters = append(filters, fmt.Sprintf("or=(code.ilike.%%%s%%,company_name.ilike.%%%s%%,short_name.ilike.%%%s%%)",
			url.QueryEscape(search), url.QueryEscape(search), url.QueryEscape(search)))
	}

	if floor != "" && floor != "all" {
		filters = append(filters, fmt.Sprintf("floor=eq.%s", url.QueryEscape(floor)))
	}

	for _, f := range filters {
		queryURL += "&" + f
	}

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "count=exact")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	var stocks []Stock
	if err := json.Unmarshal(body, &stocks); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Get total count from Content-Range header
	var total int64
	contentRange := resp.Header.Get("Content-Range")
	if contentRange != "" {
		fmt.Sscanf(contentRange, "*/%d", &total)
		if total == 0 {
			var start, end int64
			fmt.Sscanf(contentRange, "%d-%d/%d", &start, &end, &total)
		}
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &StockListResponse{
		Stocks:     stocks,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetStockByCode fetches a single stock by code
func (c *SupabaseDBClient) GetStockByCode(code string) (*Stock, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/stocks?code=eq.%s&limit=1", c.URL, url.QueryEscape(code))

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	var stocks []Stock
	if err := json.Unmarshal(body, &stocks); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(stocks) == 0 {
		return nil, errors.New("stock not found")
	}

	return &stocks[0], nil
}

// GetStockCount returns the total count of stocks
func (c *SupabaseDBClient) GetStockCount() (int64, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/stocks?select=count", c.URL)

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Prefer", "count=exact")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var total int64
	contentRange := resp.Header.Get("Content-Range")
	if contentRange != "" {
		fmt.Sscanf(contentRange, "*/%d", &total)
		if total == 0 {
			var start, end int64
			fmt.Sscanf(contentRange, "%d-%d/%d", &start, &end, &total)
		}
	}

	return total, nil
}

// GetStockStats returns statistics about stocks
func (c *SupabaseDBClient) GetStockStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get total count
	total, err := c.GetStockCount()
	if err != nil {
		return nil, err
	}
	stats["total"] = total

	// Get count by floor
	floors := []string{"HOSE", "HNX", "UPCOM"}
	floorCounts := make(map[string]int64)

	for _, floor := range floors {
		floorURL := fmt.Sprintf("%s/rest/v1/stocks?floor=eq.%s&select=count", c.URL, floor)
		floorReq, _ := http.NewRequest("GET", floorURL, nil)
		floorReq.Header.Set("apikey", c.getAPIKey())
		floorReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
		floorReq.Header.Set("Prefer", "count=exact")

		floorResp, err := c.httpClient.Do(floorReq)
		if err == nil {
			defer floorResp.Body.Close()
			contentRange := floorResp.Header.Get("Content-Range")
			if contentRange != "" {
				var count int64
				fmt.Sscanf(contentRange, "*/%d", &count)
				floorCounts[floor] = count
			}
		}
	}
	stats["by_floor"] = floorCounts

	return stats, nil
}

// UpsertStock creates or updates a stock in Supabase
func (c *SupabaseDBClient) UpsertStock(stock *VNDirectStock) error {
	queryURL := fmt.Sprintf("%s/rest/v1/stocks", c.URL)

	now := time.Now().UTC().Format(time.RFC3339)
	stockData := map[string]interface{}{
		"code":             stock.Code,
		"type":             stock.Type,
		"floor":            stock.Floor,
		"isin":             stock.ISIN,
		"status":           stock.Status,
		"company_name":     stock.CompanyName,
		"company_name_eng": stock.CompanyNameEng,
		"short_name":       stock.ShortName,
		"short_name_eng":   stock.ShortNameEng,
		"listed_date":      stock.ListedDate,
		"delisted_date":    stock.DelistedDate,
		"company_id":       stock.CompanyID,
		"tax_code":         stock.TaxCode,
		"is_active":        stock.Status == "listed",
		"last_sync_at":     now,
		"updated_at":       now,
	}

	payload, err := json.Marshal(stockData)
	if err != nil {
		return fmt.Errorf("failed to marshal stock data: %w", err)
	}

	req, err := http.NewRequest("POST", queryURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=merge-duplicates")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// SyncStocksFromVNDirect syncs stocks from VNDirect API to Supabase
func (c *SupabaseDBClient) SyncStocksFromVNDirect() (*StockSyncResult, error) {
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

	// Upsert each stock
	for _, stock := range stocks {
		// Check if stock exists
		existing, _ := c.GetStockByCode(stock.Code)

		err := c.UpsertStock(&stock)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", stock.Code, err))
			continue
		}

		if existing == nil {
			result.Created++
		} else {
			result.Updated++
		}
	}

	return result, nil
}

// DeleteStock deletes a stock by code
func (c *SupabaseDBClient) DeleteStock(code string) error {
	queryURL := fmt.Sprintf("%s/rest/v1/stocks?code=eq.%s", c.URL, url.QueryEscape(code))

	req, err := http.NewRequest("DELETE", queryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetLastSyncTime returns the last sync time for stocks
func (c *SupabaseDBClient) GetLastSyncTime() (*time.Time, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/stocks?select=last_sync_at&order=last_sync_at.desc&limit=1", c.URL)

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var results []struct {
		LastSyncAt *time.Time `json:"last_sync_at"`
	}
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(results) == 0 || results[0].LastSyncAt == nil {
		return nil, nil
	}

	return results[0].LastSyncAt, nil
}
