package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CrawlerService handles stock data crawling and technical indicator calculation
type CrawlerService struct {
	mongoClient *mongo.Client
	mongoDB     *mongo.Database
	httpClient  *http.Client
}

// StockData represents a stock from VNDirect API
type StockData struct {
	Code       string  `json:"code" bson:"code"`
	Exchange   string  `json:"exchange" bson:"exchange"`
	Name       string  `json:"name" bson:"name"`
	MarketCap  float64 `json:"market_cap" bson:"market_cap"`
	Industry   string  `json:"industry" bson:"industry"`
	Sector     string  `json:"sector" bson:"sector"`
	UpdatedAt  time.Time `json:"updated_at" bson:"updated_at"`
}

// PriceData represents OHLC price data
type PriceData struct {
	Date      time.Time `json:"date" bson:"date"`
	Open      float64   `json:"open" bson:"open"`
	High      float64   `json:"high" bson:"high"`
	Low       float64   `json:"low" bson:"low"`
	Close     float64   `json:"close" bson:"close"`
	Volume    int64     `json:"volume" bson:"volume"`
	Value     float64   `json:"value" bson:"value"`
}

// TechnicalIndicators represents calculated technical indicators
type TechnicalIndicators struct {
	RSI14          float64   `json:"rsi_14" bson:"rsi_14"`
	MACD           float64   `json:"macd" bson:"macd"`
	MACDSignal     float64   `json:"macd_signal" bson:"macd_signal"`
	MACDHistogram  float64   `json:"macd_histogram" bson:"macd_histogram"`
	MA20           float64   `json:"ma_20" bson:"ma_20"`
	MA50           float64   `json:"ma_50" bson:"ma_50"`
	MA200          float64   `json:"ma_200" bson:"ma_200"`
	BollingerUpper float64   `json:"bollinger_upper" bson:"bollinger_upper"`
	BollingerMiddle float64  `json:"bollinger_middle" bson:"bollinger_middle"`
	BollingerLower float64   `json:"bollinger_lower" bson:"bollinger_lower"`
	UpdatedAt      time.Time `json:"updated_at" bson:"updated_at"`
}

// StockIndicator combines stock info with indicators
type StockIndicator struct {
	Code       string               `bson:"_id"`
	Exchange   string               `bson:"exchange"`
	Name       string               `bson:"name"`
	LatestPrice float64             `bson:"latest_price"`
	Volume     int64                `bson:"volume"`
	Indicators TechnicalIndicators  `bson:"indicators"`
	UpdatedAt  time.Time            `bson:"updated_at"`
}

// NewCrawlerService creates a new crawler service
func NewCrawlerService(mongoClient *mongo.Client, dbName string) *CrawlerService {
	var mongoDB *mongo.Database
	if mongoClient != nil {
		mongoDB = mongoClient.Database(dbName)
	}

	return &CrawlerService{
		mongoClient: mongoClient,
		mongoDB:     mongoDB,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CrawlStocks fetches stock list from VNDirect and upserts to MongoDB
func (cs *CrawlerService) CrawlStocks() (int, error) {
	if cs.mongoDB == nil {
		return 0, fmt.Errorf("MongoDB not configured")
	}

	// VNDirect API endpoint for stock list
	url := "https://finfo-api.vndirect.com.vn/v4/stocks"
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := cs.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch stocks: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResponse struct {
		Data []struct {
			Code      string  `json:"code"`
			Exchange  string  `json:"exchange"`
			Name      string  `json:"name"`
			Type      string  `json:"type"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	// Prepare bulk upsert operations
	collection := cs.mongoDB.Collection("stocks")
	var operations []mongo.WriteModel
	now := time.Now()

	for _, stock := range apiResponse.Data {
		// Only process stocks (not indices)
		if stock.Type != "s" {
			continue
		}

		stockData := StockData{
			Code:      stock.Code,
			Exchange:  stock.Exchange,
			Name:      stock.Name,
			UpdatedAt: now,
		}

		filter := bson.M{"code": stock.Code}
		update := bson.M{"$set": stockData}
		operation := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true)

		operations = append(operations, operation)
	}

	if len(operations) == 0 {
		return 0, nil
	}

	// Execute bulk write
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := collection.BulkWrite(ctx, operations)
	if err != nil {
		return 0, fmt.Errorf("failed to bulk upsert stocks: %w", err)
	}

	log.Printf("Crawled %d stocks: %d upserted, %d modified",
		len(operations), result.UpsertedCount, result.ModifiedCount)

	return len(operations), nil
}

// CrawlAndCalculatePrices fetches price data and calculates indicators with worker pool
func (cs *CrawlerService) CrawlAndCalculatePrices(stockCodes []string, workers int) error {
	if cs.mongoDB == nil {
		return fmt.Errorf("MongoDB not configured")
	}

	if workers <= 0 {
		workers = 5
	}

	log.Printf("Starting price crawl with %d workers for %d stocks", workers, len(stockCodes))

	// Create worker pool
	jobs := make(chan string, len(stockCodes))
	results := make(chan error, len(stockCodes))
	var wg sync.WaitGroup

	// Start workers
	for w := 1; w <= workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for code := range jobs {
				log.Printf("Worker %d processing %s", workerID, code)
				err := cs.processStockPriceAndIndicators(code)
				results <- err
				time.Sleep(500 * time.Millisecond) // Rate limiting
			}
		}(w)
	}

	// Send jobs to workers
	go func() {
		for _, code := range stockCodes {
			jobs <- code
		}
		close(jobs)
	}()

	// Wait for workers to finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var errors []error
	successCount := 0
	for err := range results {
		if err != nil {
			errors = append(errors, err)
		} else {
			successCount++
		}
	}

	log.Printf("Completed: %d successful, %d errors", successCount, len(errors))

	if len(errors) > 0 && successCount == 0 {
		return fmt.Errorf("all stocks failed to process")
	}

	return nil
}

// processStockPriceAndIndicators processes a single stock: fetch prices and calculate indicators
func (cs *CrawlerService) processStockPriceAndIndicators(code string) error {
	// Fetch price data from VNDirect
	prices, err := cs.fetchPriceData(code, 365) // Last 1 year
	if err != nil {
		return fmt.Errorf("failed to fetch prices for %s: %w", code, err)
	}

	if len(prices) < 20 {
		return fmt.Errorf("insufficient data for %s: only %d days", code, len(prices))
	}

	// Calculate technical indicators
	indicators := cs.calculateIndicators(prices)

	// Get latest price and volume
	latestPrice := prices[len(prices)-1]

	// Upsert to MongoDB stock_indicators collection
	collection := cs.mongoDB.Collection("stock_indicators")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stockIndicator := StockIndicator{
		Code:        code,
		LatestPrice: latestPrice.Close,
		Volume:      latestPrice.Volume,
		Indicators:  indicators,
		UpdatedAt:   time.Now(),
	}

	filter := bson.M{"_id": code}
	update := bson.M{"$set": stockIndicator}
	opts := options.Update().SetUpsert(true)

	_, err = collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save indicators for %s: %w", code, err)
	}

	return nil
}

// fetchPriceData fetches historical price data from VNDirect API
func (cs *CrawlerService) fetchPriceData(code string, days int) ([]PriceData, error) {
	// Calculate date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	url := fmt.Sprintf("https://finfo-api.vndirect.com.vn/v4/stock_prices?symbols=%s&from=%s&to=%s&sort=date",
		code,
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := cs.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResponse struct {
		Data []struct {
			Date   string  `json:"date"`
			Open   float64 `json:"open"`
			High   float64 `json:"high"`
			Low    float64 `json:"low"`
			Close  float64 `json:"close"`
			Volume int64   `json:"volume"`
			Value  float64 `json:"value"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, err
	}

	var prices []PriceData
	for _, p := range apiResponse.Data {
		date, _ := time.Parse("2006-01-02", p.Date)
		prices = append(prices, PriceData{
			Date:   date,
			Open:   p.Open,
			High:   p.High,
			Low:    p.Low,
			Close:  p.Close,
			Volume: p.Volume,
			Value:  p.Value,
		})
	}

	return prices, nil
}

// calculateIndicators calculates all technical indicators
func (cs *CrawlerService) calculateIndicators(prices []PriceData) TechnicalIndicators {
	indicators := TechnicalIndicators{
		UpdatedAt: time.Now(),
	}

	if len(prices) < 200 {
		// Calculate what we can with available data
		if len(prices) >= 20 {
			indicators.MA20 = cs.calculateSMA(prices, 20)
		}
		if len(prices) >= 50 {
			indicators.MA50 = cs.calculateSMA(prices, 50)
		}
		if len(prices) >= 14 {
			indicators.RSI14 = cs.calculateRSI(prices, 14)
		}
	} else {
		// Calculate all indicators
		indicators.RSI14 = cs.calculateRSI(prices, 14)
		indicators.MA20 = cs.calculateSMA(prices, 20)
		indicators.MA50 = cs.calculateSMA(prices, 50)
		indicators.MA200 = cs.calculateSMA(prices, 200)

		// Calculate MACD
		macd, signal, histogram := cs.calculateMACD(prices)
		indicators.MACD = macd
		indicators.MACDSignal = signal
		indicators.MACDHistogram = histogram

		// Calculate Bollinger Bands
		upper, middle, lower := cs.calculateBollingerBands(prices, 20, 2.0)
		indicators.BollingerUpper = upper
		indicators.BollingerMiddle = middle
		indicators.BollingerLower = lower
	}

	return indicators
}

// calculateSMA calculates Simple Moving Average
func (cs *CrawlerService) calculateSMA(prices []PriceData, period int) float64 {
	if len(prices) < period {
		return 0
	}

	sum := 0.0
	for i := len(prices) - period; i < len(prices); i++ {
		sum += prices[i].Close
	}

	return sum / float64(period)
}

// calculateEMA calculates Exponential Moving Average
func (cs *CrawlerService) calculateEMA(prices []PriceData, period int) float64 {
	if len(prices) < period {
		return 0
	}

	multiplier := 2.0 / float64(period+1)
	
	// Start with SMA
	ema := cs.calculateSMA(prices[:period], period)

	// Calculate EMA for remaining prices
	for i := period; i < len(prices); i++ {
		ema = (prices[i].Close-ema)*multiplier + ema
	}

	return ema
}

// calculateRSI calculates Relative Strength Index
func (cs *CrawlerService) calculateRSI(prices []PriceData, period int) float64 {
	if len(prices) < period+1 {
		return 0
	}

	var gains, losses []float64

	for i := 1; i < len(prices); i++ {
		change := prices[i].Close - prices[i-1].Close
		if change > 0 {
			gains = append(gains, change)
			losses = append(losses, 0)
		} else {
			gains = append(gains, 0)
			losses = append(losses, -change)
		}
	}

	if len(gains) < period {
		return 0
	}

	// Calculate average gain and loss
	avgGain := 0.0
	avgLoss := 0.0

	for i := len(gains) - period; i < len(gains); i++ {
		avgGain += gains[i]
		avgLoss += losses[i]
	}

	avgGain /= float64(period)
	avgLoss /= float64(period)

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateMACD calculates MACD indicator
// Note: This is a simplified implementation. For production use, consider implementing
// proper MACD calculation with historical values or using a TA library.
func (cs *CrawlerService) calculateMACD(prices []PriceData) (float64, float64, float64) {
	if len(prices) < 35 { // Need at least 26 + 9 days for proper MACD
		return 0, 0, 0
	}

	// Calculate MACD line (12-day EMA - 26-day EMA)
	ema12 := cs.calculateEMA(prices, 12)
	ema26 := cs.calculateEMA(prices, 26)
	macd := ema12 - ema26

	// Calculate signal line (9-day EMA of MACD)
	// For a proper implementation, we would need to calculate MACD for all historical days
	// and then calculate EMA of those MACD values. This is a simplified version.
	// TODO: Store MACD historical values and calculate proper EMA(9) of MACD
	
	// Simplified signal calculation using a weighted average approach
	// This gives an approximation of the signal line direction
	signal := macd * 0.85 // Dampened MACD as signal approximation
	
	histogram := macd - signal

	return macd, signal, histogram
}

// calculateBollingerBands calculates Bollinger Bands
func (cs *CrawlerService) calculateBollingerBands(prices []PriceData, period int, stdDev float64) (float64, float64, float64) {
	if len(prices) < period {
		return 0, 0, 0
	}

	// Calculate middle band (SMA)
	middle := cs.calculateSMA(prices, period)

	// Calculate standard deviation
	variance := 0.0
	for i := len(prices) - period; i < len(prices); i++ {
		diff := prices[i].Close - middle
		variance += diff * diff
	}
	variance /= float64(period)
	stdDeviation := 0.0
	if variance > 0 {
		// Simple square root approximation
		stdDeviation = variance / 2.0
		for i := 0; i < 10; i++ { // Newton's method iterations
			stdDeviation = (stdDeviation + variance/stdDeviation) / 2.0
		}
	}

	upper := middle + (stdDeviation * stdDev)
	lower := middle - (stdDeviation * stdDev)

	return upper, middle, lower
}

// CreateIndexes creates MongoDB indexes for optimal query performance
func (cs *CrawlerService) CreateIndexes() error {
	if cs.mongoDB == nil {
		return fmt.Errorf("MongoDB not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create indexes on stock_indicators collection
	collection := cs.mongoDB.Collection("stock_indicators")

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "indicators.rsi_14", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "volume", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "latest_price", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "updated_at", Value: -1}},
		},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	log.Println("MongoDB indexes created successfully")
	return nil
}
