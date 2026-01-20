package datafetcher

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go_backend_project/models"
	"gorm.io/gorm"
)

// DataFetcher handles fetching stock data from Vietnamese exchanges
type DataFetcher struct {
	db         *gorm.DB
	httpClient *http.Client
}

// NewDataFetcher creates a new data fetcher instance
func NewDataFetcher(db *gorm.DB) *DataFetcher {
	return &DataFetcher{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SSIQuoteResponse represents SSI API response structure
type SSIQuoteResponse struct {
	Data []struct {
		Symbol        string  `json:"stockSymbol"`
		Open          float64 `json:"open"`
		High          float64 `json:"high"`
		Low           float64 `json:"low"`
		Close         float64 `json:"close"`
		Volume        int64   `json:"volume"`
		Value         float64 `json:"value"`
		Change        float64 `json:"change"`
		ChangePercent float64 `json:"percentChange"`
	} `json:"data"`
}

// VNDirectResponse represents VNDirect API response
type VNDirectResponse struct {
	Data []struct {
		Code   string  `json:"code"`
		Date   string  `json:"date"`
		Open   float64 `json:"open"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Close  float64 `json:"close"`
		Volume int64   `json:"nmVolume"`
		Value  float64 `json:"nmValue"`
	} `json:"data"`
}

// VNDirectStockListResponse represents VNDirect stock list response
type VNDirectStockListResponse struct {
	Data []struct {
		Code         string `json:"code"`
		Name         string `json:"name"`
		CompanyName  string `json:"companyName"`
		ShortName    string `json:"shortName"`
		Exchange     string `json:"exchange"`
		IndustryName string `json:"industryName"`
		ListingDate  string `json:"listingDate"`
		Status       string `json:"status"`
	} `json:"data"`
}

const (
	vndirectStockListURL   = "https://finfo-api.vndirect.com.vn/v4/stocks?size=2000"
	vndirectPriceBatchSize = 400
)

// FetchStockList fetches list of all stocks from Vietnamese exchanges
func (df *DataFetcher) FetchStockList() error {
	// Sample stocks - in production, fetch from actual API
	stocks := []models.Stock{
		{Symbol: "VNM", Name: "Vinamilk", Exchange: "HOSE", Industry: "Consumer Goods", Status: "active"},
		{Symbol: "VIC", Name: "Vingroup", Exchange: "HOSE", Industry: "Real Estate", Status: "active"},
		{Symbol: "HPG", Name: "Hoa Phat Group", Exchange: "HOSE", Industry: "Steel", Status: "active"},
		{Symbol: "VHM", Name: "Vinhomes", Exchange: "HOSE", Industry: "Real Estate", Status: "active"},
		{Symbol: "VCB", Name: "Vietcombank", Exchange: "HOSE", Industry: "Banking", Status: "active"},
		{Symbol: "TCB", Name: "Techcombank", Exchange: "HOSE", Industry: "Banking", Status: "active"},
		{Symbol: "MSN", Name: "Masan Group", Exchange: "HOSE", Industry: "Consumer Goods", Status: "active"},
		{Symbol: "FPT", Name: "FPT Corporation", Exchange: "HOSE", Industry: "Technology", Status: "active"},
		{Symbol: "ACB", Name: "Asia Commercial Bank", Exchange: "HOSE", Industry: "Banking", Status: "active"},
		{Symbol: "GAS", Name: "PetroVietnam Gas", Exchange: "HOSE", Industry: "Oil & Gas", Status: "active"},
	}

	for _, stock := range stocks {
		// Check if stock already exists
		var existing models.Stock
		if err := df.db.Where("symbol = ?", stock.Symbol).First(&existing).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Create new stock
				if err := df.db.Create(&stock).Error; err != nil {
					return fmt.Errorf("failed to create stock %s: %w", stock.Symbol, err)
				}
			} else {
				return err
			}
		}
	}

	return nil
}

// FetchHistoricalData fetches historical price data for a stock
func (df *DataFetcher) FetchHistoricalData(symbol string, startDate, endDate time.Time) error {
	// In production, this would call actual APIs like:
	// - SSI iBoard API
	// - VNDirect API
	// - TCBS API
	// For now, we'll generate sample data

	var stock models.Stock
	if err := df.db.Where("symbol = ?", symbol).First(&stock).Error; err != nil {
		return fmt.Errorf("stock not found: %w", err)
	}

	// Generate sample historical data
	currentDate := startDate
	basePrice := 50000.0 // Starting price

	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		// Skip weekends
		if currentDate.Weekday() == time.Saturday || currentDate.Weekday() == time.Sunday {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		// Generate realistic price movements
		change := (float64(currentDate.Unix()%100) - 50) / 100.0 // Random change -0.5 to +0.5
		openPrice := basePrice * (1 + change*0.02)
		highPrice := openPrice * (1 + float64(currentDate.Unix()%50)/1000.0)
		lowPrice := openPrice * (1 - float64(currentDate.Unix()%50)/1000.0)
		closePrice := openPrice * (1 + change*0.01)
		volume := int64(1000000 + (currentDate.Unix() % 5000000))

		price := models.StockPrice{
			StockID:       stock.ID,
			Date:          currentDate,
			Open:          decimal.NewFromFloat(openPrice),
			High:          decimal.NewFromFloat(highPrice),
			Low:           decimal.NewFromFloat(lowPrice),
			Close:         decimal.NewFromFloat(closePrice),
			Volume:        volume,
			Value:         decimal.NewFromFloat(closePrice * float64(volume)),
			AdjClose:      decimal.NewFromFloat(closePrice),
			Change:        decimal.NewFromFloat(closePrice - openPrice),
			ChangePercent: decimal.NewFromFloat((closePrice - openPrice) / openPrice * 100),
		}

		// Check if price already exists for this date
		var existing models.StockPrice
		err := df.db.Where("stock_id = ? AND date = ?", stock.ID, currentDate).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := df.db.Create(&price).Error; err != nil {
				return fmt.Errorf("failed to create price for %s on %s: %w", symbol, currentDate, err)
			}
		}

		basePrice = closePrice
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	return nil
}

// FetchVNDirectStockList fetches stock symbols from VNDirect API
func (df *DataFetcher) FetchVNDirectStockList() error {
	resp, err := df.httpClient.Get(vndirectStockListURL)
	if err != nil {
		return fmt.Errorf("failed to fetch VNDirect stock list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("VNDirect stock list request failed: %s", strings.TrimSpace(string(body)))
	}

	var stockResponse VNDirectStockListResponse
	if err := json.NewDecoder(resp.Body).Decode(&stockResponse); err != nil {
		return fmt.Errorf("failed to decode VNDirect stock list: %w", err)
	}

	for _, item := range stockResponse.Data {
		if item.Code == "" {
			continue
		}

		name := item.Name
		if name == "" {
			name = item.CompanyName
		}
		if name == "" {
			name = item.ShortName
		}
		if name == "" {
			name = item.Code
		}

		status := normalizeVNDirectStatus(item.Status)
		if status == "" {
			status = "active"
		}

		var listingDate *time.Time
		if item.ListingDate != "" {
			if parsed, err := parseVNDirectDate(item.ListingDate); err == nil {
				normalized := normalizeVNDirectDate(parsed)
				listingDate = &normalized
			}
		}

		var existing models.Stock
		if err := df.db.Where("symbol = ?", item.Code).First(&existing).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				stock := models.Stock{
					Symbol:      item.Code,
					Name:        name,
					Exchange:    item.Exchange,
					Industry:    item.IndustryName,
					Status:      status,
					ListingDate: listingDate,
				}
				if err := df.db.Create(&stock).Error; err != nil {
					return fmt.Errorf("failed to create stock %s: %w", item.Code, err)
				}
				continue
			}
			return err
		}

		updates := map[string]interface{}{
			"status": status,
		}
		if name != "" {
			updates["name"] = name
		}
		if item.Exchange != "" {
			updates["exchange"] = item.Exchange
		}
		if item.IndustryName != "" {
			updates["industry"] = item.IndustryName
		}
		if listingDate != nil {
			updates["listing_date"] = listingDate
		}

		if err := df.db.Model(&existing).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update stock %s: %w", item.Code, err)
		}
	}

	return nil
}

// FetchVNDirectDailyCloseHistory fetches historical daily prices from VNDirect API
func (df *DataFetcher) FetchVNDirectDailyCloseHistory(symbol string, startDate, endDate time.Time) error {
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}

	var stock models.Stock
	if err := df.db.Where("symbol = ?", symbol).First(&stock).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("failed to load stock %s: %w", symbol, err)
		}
		stock = models.Stock{
			Symbol: symbol,
			Name:   symbol,
			Status: "active",
		}
		if err := df.db.Create(&stock).Error; err != nil {
			return fmt.Errorf("failed to create stock %s: %w", symbol, err)
		}
	}

	normalizedStart := normalizeVNDirectDate(startDate)
	normalizedEnd := normalizeVNDirectDate(endDate)
	queryStart := normalizedStart.Format("2006-01-02")
	queryEnd := normalizedEnd.Format("2006-01-02")
	url := fmt.Sprintf("https://finfo-api.vndirect.com.vn/v4/stock_prices?q=code:%s~date:gte:%s~date:lte:%s&sort=date:asc&size=%d",
		symbol, queryStart, queryEnd, vndirectPriceBatchSize)

	resp, err := df.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch VNDirect prices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("VNDirect price request failed: %s", strings.TrimSpace(string(body)))
	}

	var response VNDirectResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode VNDirect prices: %w", err)
	}

	previousClose := decimal.Zero
	var lastPrice models.StockPrice
	if err := df.db.Where("stock_id = ? AND date < ?", stock.ID, normalizedStart).
		Order("date DESC").
		First(&lastPrice).Error; err == nil {
		previousClose = lastPrice.Close
	}

	for _, item := range response.Data {
		parsedDate, err := parseVNDirectDate(item.Date)
		if err != nil {
			continue
		}
		normalizedDate := normalizeVNDirectDate(parsedDate)

		open := decimal.NewFromFloat(item.Open)
		high := decimal.NewFromFloat(item.High)
		low := decimal.NewFromFloat(item.Low)
		closePrice := decimal.NewFromFloat(item.Close)
		// VNDirect may omit value data; avoid estimating to keep calculations explicit.
		value := decimal.NewFromFloat(item.Value)
		changeBase := open
		if !previousClose.Equal(decimal.Zero) {
			// Prefer previous close for day-over-day change calculation.
			changeBase = previousClose
		}
		change := closePrice.Sub(changeBase)
		changePercent := decimal.Zero
		if !changeBase.Equal(decimal.Zero) {
			changePercent = change.Div(changeBase).Mul(decimal.NewFromFloat(100))
		}

		price := models.StockPrice{
			StockID:       stock.ID,
			Date:          normalizedDate,
			Open:          open,
			High:          high,
			Low:           low,
			Close:         closePrice,
			Volume:        item.Volume,
			Value:         value,
			AdjClose:      closePrice,
			Change:        change,
			ChangePercent: changePercent,
		}

		var existing models.StockPrice
		err = df.db.Where("stock_id = ? AND date = ?", stock.ID, normalizedDate).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := df.db.Create(&price).Error; err != nil {
				return fmt.Errorf("failed to create price for %s on %s: %w", symbol, normalizedDate, err)
			}
			previousClose = closePrice
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to load existing price: %w", err)
		}

		updates := map[string]interface{}{
			"open":           price.Open,
			"high":           price.High,
			"low":            price.Low,
			"close":          price.Close,
			"volume":         price.Volume,
			"value":          price.Value,
			"adj_close":      price.AdjClose,
			"change":         price.Change,
			"change_percent": price.ChangePercent,
		}

		if err := df.db.Model(&existing).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update price for %s on %s: %w", symbol, normalizedDate, err)
		}

		previousClose = closePrice
	}

	return nil
}

// FetchRealtimeQuote fetches real-time quote for a stock
func (df *DataFetcher) FetchRealtimeQuote(symbol string) (*models.StockPrice, error) {
	// In production, call real-time API
	// For now, return latest price from database

	var stock models.Stock
	if err := df.db.Where("symbol = ?", symbol).First(&stock).Error; err != nil {
		return nil, fmt.Errorf("stock not found: %w", err)
	}

	var price models.StockPrice
	if err := df.db.Where("stock_id = ?", stock.ID).Order("date DESC").First(&price).Error; err != nil {
		return nil, fmt.Errorf("no price data found: %w", err)
	}

	return &price, nil
}

// FetchVNDirectData fetches data from VNDirect API (placeholder)
func (df *DataFetcher) FetchVNDirectData(symbol string) error {
	// Example VNDirect API endpoint
	url := fmt.Sprintf("https://finfo-api.vndirect.com.vn/v4/stock_prices?q=code:%s~date:gte:2024-01-01", symbol)

	resp, err := df.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch from VNDirect: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var vnResponse VNDirectResponse
	if err := json.Unmarshal(body, &vnResponse); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Process and store data...
	// Implementation depends on actual API structure

	return nil
}

// FetchMarketIndices fetches market index data
func (df *DataFetcher) FetchMarketIndices() error {
	// Fetch VN-Index, HNX-Index, UPCOM-Index
	indices := []string{"VNINDEX", "HNXINDEX", "UPCOMINDEX"}

	for _, indexCode := range indices {
		// In production, fetch from actual API
		// For now, create sample data

		index := models.MarketIndex{
			Name:          indexCode,
			Code:          indexCode,
			Date:          time.Now(),
			Open:          decimal.NewFromFloat(1200.0),
			High:          decimal.NewFromFloat(1220.0),
			Low:           decimal.NewFromFloat(1195.0),
			Close:         decimal.NewFromFloat(1210.0),
			Volume:        500000000,
			Value:         decimal.NewFromFloat(15000000000000.0),
			Change:        decimal.NewFromFloat(10.0),
			ChangePercent: decimal.NewFromFloat(0.83),
		}

		// Check if index data for today already exists
		var existing models.MarketIndex
		today := time.Now().Format("2006-01-02")
		err := df.db.Where("code = ? AND DATE(date) = ?", indexCode, today).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			if err := df.db.Create(&index).Error; err != nil {
				return fmt.Errorf("failed to create index %s: %w", indexCode, err)
			}
		}
	}

	return nil
}

func normalizeVNDirectStatus(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "", "listed", "trading", "active":
		return "active"
	case "delisted":
		return "delisted"
	case "suspended":
		return "suspended"
	default:
		return normalized
	}
}

func parseVNDirectDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("date is empty")
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse("2006-01-02T15:04:05", value); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("unsupported VNDirect date format: %s", value)
}

func normalizeVNDirectDate(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
