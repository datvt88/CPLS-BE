package datafetcher

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go_backend_project/models"
	"go_backend_project/services"
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

// VNDirectPriceResponse represents VNDirect price API response
type VNDirectPriceResponse struct {
	Data []VNDirectPriceData `json:"data"`
}

const vnDirectDateLayout = "2006-01-02"

// VNDirectPriceData represents VNDirect daily price data
type VNDirectPriceData struct {
	Code      string  `json:"code"`
	Date      string  `json:"date"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	Value     float64 `json:"value"`
	Change    float64 `json:"change"`
	PctChange float64 `json:"pctChange"`
}

// FetchStockList fetches list of all stocks from Vietnamese exchanges
func (df *DataFetcher) FetchStockList() error {
	if df.db == nil {
		return fmt.Errorf("database not initialized")
	}

	vnStocks, err := services.FetchStocksFromVNDirect()
	if err != nil {
		return fmt.Errorf("failed to fetch VNDirect stock list: %w", err)
	}
	if len(vnStocks) == 0 {
		return fmt.Errorf("VNDirect stock list is empty")
	}

	for _, vnStock := range vnStocks {
		symbol := strings.ToUpper(strings.TrimSpace(vnStock.Code))
		if symbol == "" {
			continue
		}

		name := strings.TrimSpace(vnStock.CompanyName)
		if name == "" {
			name = strings.TrimSpace(vnStock.ShortName)
		}
		if name == "" {
			name = symbol
		}

		exchange := strings.ToUpper(strings.TrimSpace(vnStock.Floor))
		status := strings.ToLower(strings.TrimSpace(vnStock.Status))
		if status == "listed" {
			status = "active"
		} else if status == "" {
			status = "active"
		}

		var listingDate *time.Time
		if vnStock.ListedDate != "" {
			if parsedDate, err := time.Parse(vnDirectDateLayout, vnStock.ListedDate); err == nil {
				listingDate = &parsedDate
			}
		}

		var existing models.Stock
		if err := df.db.Where("symbol = ?", symbol).First(&existing).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				stock := models.Stock{
					Symbol:      symbol,
					Name:        name,
					Exchange:    exchange,
					Status:      status,
					ListingDate: listingDate,
				}
				if err := df.db.Create(&stock).Error; err != nil {
					return fmt.Errorf("failed to create stock %s: %w", symbol, err)
				}
			} else {
				return err
			}
		} else {
			updates := map[string]interface{}{
				"name":     name,
				"exchange": exchange,
				"status":   status,
			}
			if listingDate != nil {
				updates["listing_date"] = listingDate
			}
			if err := df.db.Model(&existing).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update stock %s: %w", symbol, err)
			}
		}
	}

	return nil
}

// FetchHistoricalData fetches historical price data for a stock
func (df *DataFetcher) FetchHistoricalData(symbol string, startDate, endDate time.Time) error {
	if df.db == nil {
		return fmt.Errorf("database not initialized")
	}
	var stock models.Stock
	if err := df.db.Where("symbol = ?", symbol).First(&stock).Error; err != nil {
		return fmt.Errorf("stock not found: %w", err)
	}

	priceData, err := df.fetchVNDirectPrices(symbol, startDate, endDate)
	if err != nil {
		return err
	}

	for _, data := range priceData {
		priceDate, err := time.Parse(vnDirectDateLayout, data.Date)
		if err != nil {
			continue
		}

		change := data.Change
		if change == 0 {
			change = data.Close - data.Open
		}
		changePercent := data.PctChange
		if changePercent == 0 && data.Open != 0 {
			changePercent = (data.Close - data.Open) / data.Open * 100
		}

		volume := data.Volume
		if volume < 0 {
			volume = 0
		} else if volume > float64(math.MaxInt64) {
			volume = float64(math.MaxInt64)
		}
		volume = math.Round(volume)

		price := models.StockPrice{
			StockID:       stock.ID,
			Date:          priceDate,
			Open:          decimal.NewFromFloat(data.Open),
			High:          decimal.NewFromFloat(data.High),
			Low:           decimal.NewFromFloat(data.Low),
			Close:         decimal.NewFromFloat(data.Close),
			Volume:        int64(volume),
			Value:         decimal.NewFromFloat(data.Value),
			AdjClose:      decimal.NewFromFloat(data.Close),
			Change:        decimal.NewFromFloat(change),
			ChangePercent: decimal.NewFromFloat(changePercent),
		}

		var existing models.StockPrice
		err = df.db.Where("stock_id = ? AND date = ?", stock.ID, priceDate).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := df.db.Create(&price).Error; err != nil {
				return fmt.Errorf("failed to create price for %s on %s: %w", symbol, priceDate, err)
			}
			continue
		}
		if err != nil {
			return err
		}

		if err := df.db.Model(&existing).Updates(map[string]interface{}{
			"open":           price.Open,
			"high":           price.High,
			"low":            price.Low,
			"close":          price.Close,
			"volume":         price.Volume,
			"value":          price.Value,
			"adj_close":      price.AdjClose,
			"change":         price.Change,
			"change_percent": price.ChangePercent,
		}).Error; err != nil {
			return fmt.Errorf("failed to update price for %s on %s: %w", symbol, priceDate, err)
		}
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
	escapedSymbol := url.QueryEscape(strings.ToUpper(strings.TrimSpace(symbol)))
	url := fmt.Sprintf("https://finfo-api.vndirect.com.vn/v4/stock_prices?symbols=%s&sort=date", escapedSymbol)

	resp, err := df.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch from VNDirect: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var vnResponse VNDirectPriceResponse
	if err := json.Unmarshal(body, &vnResponse); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Process and store data...
	// Implementation depends on actual API structure

	return nil
}

func (df *DataFetcher) fetchVNDirectPrices(symbol string, startDate, endDate time.Time) ([]VNDirectPriceData, error) {
	escapedSymbol := url.QueryEscape(strings.ToUpper(strings.TrimSpace(symbol)))
	fromDate := startDate.Format(vnDirectDateLayout)
	toDate := endDate.Format(vnDirectDateLayout)
	apiURL := fmt.Sprintf("https://finfo-api.vndirect.com.vn/v4/stock_prices?symbols=%s&from=%s&to=%s&sort=date",
		escapedSymbol, fromDate, toDate)

	resp, err := df.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch VNDirect prices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("VNDirect API error (status %d) and failed to read body: %w", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("VNDirect API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response VNDirectPriceResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("no price data returned for %s (%s to %s) - stock may not exist or no trading data available for this period", symbol, fromDate, toDate)
	}

	return response.Data, nil
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
