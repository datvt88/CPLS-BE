package services

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ExtendedStockIndicators holds all calculated technical indicators
type ExtendedStockIndicators struct {
	// Relative Strength (change percentage)
	RS3D float64 `json:"rs_3d"` // 3 days change %
	RS1M float64 `json:"rs_1m"` // 1 month (~22 days) change %
	RS3M float64 `json:"rs_3m"` // 3 months (~66 days) change %
	RS1Y float64 `json:"rs_1y"` // 1 year (~252 days) change %

	// Relative Strength Ranks (1-100 percentile)
	RS3DRank float64 `json:"rs_3d_rank"`
	RS1MRank float64 `json:"rs_1m_rank"`
	RS3MRank float64 `json:"rs_3m_rank"`
	RS1YRank float64 `json:"rs_1y_rank"`
	RSAvg    float64 `json:"rs_avg"` // Average of all RS ranks

	// MACD
	MACD       float64 `json:"macd"`        // MACD line (12-26 EMA)
	MACDSignal float64 `json:"macd_signal"` // Signal line (9 EMA of MACD)
	MACDHist   float64 `json:"macd_hist"`   // MACD Histogram

	// Volume
	AvgVol   float64 `json:"avg_vol"`    // 20-day average volume
	VolRatio float64 `json:"vol_ratio"`  // Current vol / Avg vol

	// RSI
	RSI float64 `json:"rsi"` // 14-day RSI

	// Moving Averages
	MA10  float64 `json:"ma_10"`
	MA30  float64 `json:"ma_30"`
	MA50  float64 `json:"ma_50"`
	MA200 float64 `json:"ma_200"`

	// Price info
	CurrentPrice float64 `json:"current_price"`
	PriceChange  float64 `json:"price_change"` // Today's change %

	// Metadata
	UpdatedAt string `json:"updated_at"`
}

// StockIndicatorService handles indicator calculations
type StockIndicatorService struct {
	mu sync.RWMutex
}

// Global indicator service instance
var GlobalIndicatorService *StockIndicatorService

// InitIndicatorService initializes the indicator service
func InitIndicatorService() error {
	GlobalIndicatorService = &StockIndicatorService{}
	log.Println("Stock Indicator Service initialized")
	return nil
}

// CalculateMA calculates Simple Moving Average
func CalculateMA(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}

	sum := 0.0
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	return sum / float64(period)
}

// CalculateEMA calculates Exponential Moving Average
func CalculateEMA(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}

	// Start with SMA for first EMA value
	sma := CalculateMA(prices[len(prices)-period:], period)
	if sma == 0 {
		return 0
	}

	multiplier := 2.0 / float64(period+1)
	ema := sma

	// Calculate EMA from oldest to newest
	for i := len(prices) - period - 1; i >= 0; i-- {
		ema = (prices[i]-ema)*multiplier + ema
	}

	return ema
}

// CalculateEMASeries calculates EMA series for MACD
func CalculateEMASeries(prices []float64, period int) []float64 {
	if len(prices) < period {
		return nil
	}

	result := make([]float64, len(prices))

	// Start with SMA for first value
	sum := 0.0
	for i := len(prices) - period; i < len(prices); i++ {
		sum += prices[i]
	}
	result[len(prices)-period] = sum / float64(period)

	multiplier := 2.0 / float64(period+1)

	// Calculate EMA forward
	for i := len(prices) - period - 1; i >= 0; i-- {
		result[i] = (prices[i]-result[i+1])*multiplier + result[i+1]
	}

	return result
}

// CalculateRSI calculates Relative Strength Index
func CalculateRSI(prices []float64, period int) float64 {
	if len(prices) <= period {
		return 50 // Default neutral
	}

	gains := 0.0
	losses := 0.0

	// Calculate average gains and losses
	for i := 0; i < period; i++ {
		change := prices[i] - prices[i+1]
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return math.Round(rsi*100) / 100
}

// CalculateMACD calculates MACD, Signal, and Histogram
func CalculateMACD(prices []float64) (macd, signal, hist float64) {
	if len(prices) < 26 {
		return 0, 0, 0
	}

	// Calculate EMA 12 and EMA 26
	ema12 := CalculateEMA(prices, 12)
	ema26 := CalculateEMA(prices, 26)

	macd = ema12 - ema26

	// For signal line, we need MACD series
	// Simplified: calculate signal as EMA of recent MACD values
	if len(prices) < 35 { // Need enough data for signal
		return macd, 0, macd
	}

	// Calculate MACD series
	ema12Series := CalculateEMASeries(prices, 12)
	ema26Series := CalculateEMASeries(prices, 26)

	if ema12Series == nil || ema26Series == nil {
		return macd, 0, macd
	}

	macdSeries := make([]float64, len(prices)-25)
	for i := 0; i < len(macdSeries); i++ {
		macdSeries[i] = ema12Series[i] - ema26Series[i]
	}

	// Signal is 9-day EMA of MACD
	signal = CalculateEMA(macdSeries, 9)
	hist = macd - signal

	return math.Round(macd*100) / 100,
		math.Round(signal*100) / 100,
		math.Round(hist*100) / 100
}

// CalculatePriceChange calculates percentage change over period
func CalculatePriceChange(prices []float64, period int) float64 {
	if len(prices) <= period {
		return 0
	}

	currentPrice := prices[0]
	pastPrice := prices[period]

	if pastPrice == 0 {
		return 0
	}

	change := ((currentPrice - pastPrice) / pastPrice) * 100
	return math.Round(change*100) / 100
}

// CalculateAvgVolume calculates average volume over period
func CalculateAvgVolume(volumes []float64, period int) float64 {
	if len(volumes) < period {
		period = len(volumes)
	}
	if period == 0 {
		return 0
	}

	sum := 0.0
	for i := 0; i < period; i++ {
		sum += volumes[i]
	}

	return math.Round(sum / float64(period))
}

// CalculateIndicatorsForStock calculates all indicators for a single stock
func CalculateIndicatorsForStock(priceFile *StockPriceFile) *ExtendedStockIndicators {
	if priceFile == nil || len(priceFile.Prices) < 10 {
		return nil
	}

	prices := priceFile.Prices

	// Extract close prices and volumes (prices are sorted desc by date)
	closePrices := make([]float64, len(prices))
	volumes := make([]float64, len(prices))

	for i, p := range prices {
		closePrices[i] = p.Close
		volumes[i] = p.NmVolume
	}

	indicators := &ExtendedStockIndicators{
		CurrentPrice: closePrices[0],
		UpdatedAt:    time.Now().Format(time.RFC3339),
	}

	// Price changes (RS values)
	indicators.PriceChange = CalculatePriceChange(closePrices, 1)
	indicators.RS3D = CalculatePriceChange(closePrices, 3)
	indicators.RS1M = CalculatePriceChange(closePrices, 22)  // ~1 month
	indicators.RS3M = CalculatePriceChange(closePrices, 66)  // ~3 months
	indicators.RS1Y = CalculatePriceChange(closePrices, 252) // ~1 year

	// RSI (14-day)
	indicators.RSI = CalculateRSI(closePrices, 14)

	// MACD
	indicators.MACD, indicators.MACDSignal, indicators.MACDHist = CalculateMACD(closePrices)

	// Average Volume (20-day)
	indicators.AvgVol = CalculateAvgVolume(volumes, 20)
	if indicators.AvgVol > 0 && volumes[0] > 0 {
		indicators.VolRatio = math.Round((volumes[0]/indicators.AvgVol)*100) / 100
	}

	// Moving Averages
	indicators.MA10 = math.Round(CalculateMA(closePrices, 10)*100) / 100
	indicators.MA30 = math.Round(CalculateMA(closePrices, 30)*100) / 100
	indicators.MA50 = math.Round(CalculateMA(closePrices, 50)*100) / 100
	indicators.MA200 = math.Round(CalculateMA(closePrices, 200)*100) / 100

	return indicators
}

// StockRSData holds RS values for ranking
type StockRSData struct {
	Code string
	RS3D float64
	RS1M float64
	RS3M float64
	RS1Y float64
}

// CalculateRSRanks calculates percentile ranks for all stocks
func CalculateRSRanks(allIndicators map[string]*ExtendedStockIndicators) {
	if len(allIndicators) == 0 {
		return
	}

	// Collect all RS values
	rs3dValues := make([]StockRSData, 0, len(allIndicators))
	rs1mValues := make([]StockRSData, 0, len(allIndicators))
	rs3mValues := make([]StockRSData, 0, len(allIndicators))
	rs1yValues := make([]StockRSData, 0, len(allIndicators))

	for code, ind := range allIndicators {
		if ind == nil {
			continue
		}
		data := StockRSData{Code: code, RS3D: ind.RS3D, RS1M: ind.RS1M, RS3M: ind.RS3M, RS1Y: ind.RS1Y}
		rs3dValues = append(rs3dValues, data)
		rs1mValues = append(rs1mValues, data)
		rs3mValues = append(rs3mValues, data)
		rs1yValues = append(rs1yValues, data)
	}

	// Sort and assign ranks (higher RS = higher rank)
	// RS3D
	sort.Slice(rs3dValues, func(i, j int) bool {
		return rs3dValues[i].RS3D < rs3dValues[j].RS3D
	})
	for i, v := range rs3dValues {
		if ind, ok := allIndicators[v.Code]; ok {
			ind.RS3DRank = math.Round(float64(i+1) / float64(len(rs3dValues)) * 100)
		}
	}

	// RS1M
	sort.Slice(rs1mValues, func(i, j int) bool {
		return rs1mValues[i].RS1M < rs1mValues[j].RS1M
	})
	for i, v := range rs1mValues {
		if ind, ok := allIndicators[v.Code]; ok {
			ind.RS1MRank = math.Round(float64(i+1) / float64(len(rs1mValues)) * 100)
		}
	}

	// RS3M
	sort.Slice(rs3mValues, func(i, j int) bool {
		return rs3mValues[i].RS3M < rs3mValues[j].RS3M
	})
	for i, v := range rs3mValues {
		if ind, ok := allIndicators[v.Code]; ok {
			ind.RS3MRank = math.Round(float64(i+1) / float64(len(rs3mValues)) * 100)
		}
	}

	// RS1Y
	sort.Slice(rs1yValues, func(i, j int) bool {
		return rs1yValues[i].RS1Y < rs1yValues[j].RS1Y
	})
	for i, v := range rs1yValues {
		if ind, ok := allIndicators[v.Code]; ok {
			ind.RS1YRank = math.Round(float64(i+1) / float64(len(rs1yValues)) * 100)
		}
	}

	// Calculate RSAvg (average of all ranks)
	for _, ind := range allIndicators {
		if ind == nil {
			continue
		}
		ind.RSAvg = math.Round((ind.RS3DRank + ind.RS1MRank + ind.RS3MRank + ind.RS1YRank) / 4)
	}
}

// CalculateAllIndicators calculates indicators for all stocks with price data
func (s *StockIndicatorService) CalculateAllIndicators() (map[string]*ExtendedStockIndicators, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Read all price files
	files, err := os.ReadDir(StockPriceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read price directory: %w", err)
	}

	allIndicators := make(map[string]*ExtendedStockIndicators)

	// Calculate individual indicators
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		code := file.Name()[:len(file.Name())-5] // Remove .json

		priceFile, err := GlobalPriceService.LoadStockPrice(code)
		if err != nil {
			log.Printf("Failed to load price for %s: %v", code, err)
			continue
		}

		indicators := CalculateIndicatorsForStock(priceFile)
		if indicators != nil {
			allIndicators[code] = indicators
		}
	}

	// Calculate RS ranks across all stocks
	CalculateRSRanks(allIndicators)

	log.Printf("Calculated indicators for %d stocks", len(allIndicators))

	return allIndicators, nil
}

// SaveIndicatorsToFile saves indicators back to price files
func (s *StockIndicatorService) SaveIndicatorsToFile(allIndicators map[string]*ExtendedStockIndicators) error {
	savedCount := 0

	for code, indicators := range allIndicators {
		if indicators == nil {
			continue
		}

		// Load existing price file
		priceFile, err := GlobalPriceService.LoadStockPrice(code)
		if err != nil {
			continue
		}

		// Convert to StockIndicators for storage
		priceFile.Indicators = &StockIndicators{
			RS3D:      indicators.RS3DRank, // Store rank instead of raw value
			RS1M:      indicators.RS1MRank,
			RS3M:      indicators.RS3MRank,
			RS1Y:      indicators.RS1YRank,
			RSAvg:     indicators.RSAvg,
			MACDHist:  indicators.MACDHist,
			AvgVol:    indicators.AvgVol,
			RSI:       indicators.RSI,
			UpdatedAt: indicators.UpdatedAt,
		}

		// Save to file
		data, err := json.MarshalIndent(priceFile, "", "  ")
		if err != nil {
			continue
		}

		filePath := filepath.Join(StockPriceDir, fmt.Sprintf("%s.json", code))
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			continue
		}

		savedCount++
	}

	log.Printf("Saved indicators for %d stocks", savedCount)
	return nil
}

// IndicatorSummaryFile stores summary of all stock indicators
type IndicatorSummaryFile struct {
	UpdatedAt string                              `json:"updated_at"`
	Count     int                                 `json:"count"`
	Stocks    map[string]*ExtendedStockIndicators `json:"stocks"`
}

// SaveIndicatorSummary saves all indicators to a summary file
func (s *StockIndicatorService) SaveIndicatorSummary(indicators map[string]*ExtendedStockIndicators) error {
	summary := IndicatorSummaryFile{
		UpdatedAt: time.Now().Format(time.RFC3339),
		Count:     len(indicators),
		Stocks:    indicators,
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	summaryPath := filepath.Join("data", "indicators_summary.json")
	if err := os.WriteFile(summaryPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	log.Printf("Saved indicator summary to %s", summaryPath)
	return nil
}

// LoadIndicatorSummary loads the indicator summary file
func (s *StockIndicatorService) LoadIndicatorSummary() (*IndicatorSummaryFile, error) {
	summaryPath := filepath.Join("data", "indicators_summary.json")

	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return nil, err
	}

	var summary IndicatorSummaryFile
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}

	return &summary, nil
}

// CalculateAndSaveAllIndicators calculates and saves all indicators
func (s *StockIndicatorService) CalculateAndSaveAllIndicators() error {
	// Calculate all indicators
	indicators, err := s.CalculateAllIndicators()
	if err != nil {
		return err
	}

	// Save to individual files
	if err := s.SaveIndicatorsToFile(indicators); err != nil {
		return err
	}

	// Save summary file
	if err := s.SaveIndicatorSummary(indicators); err != nil {
		return err
	}

	return nil
}

// GetStockIndicators returns indicators for a specific stock
func (s *StockIndicatorService) GetStockIndicators(code string) (*ExtendedStockIndicators, error) {
	priceFile, err := GlobalPriceService.LoadStockPrice(code)
	if err != nil {
		return nil, err
	}

	return CalculateIndicatorsForStock(priceFile), nil
}

// FilterStocksByIndicators filters stocks by indicator criteria
type IndicatorFilter struct {
	RSAvgMin   float64 `json:"rs_avg_min"`
	RSAvgMax   float64 `json:"rs_avg_max"`
	RSIMin     float64 `json:"rsi_min"`
	RSIMax     float64 `json:"rsi_max"`
	MACDHistPositive *bool `json:"macd_hist_positive"`
	AboveMA50  *bool   `json:"above_ma50"`
	AboveMA200 *bool   `json:"above_ma200"`
	MinVolume  float64 `json:"min_volume"`
}

// FilterStocks filters stocks by indicator criteria
func (s *StockIndicatorService) FilterStocks(filter IndicatorFilter) ([]string, error) {
	summary, err := s.LoadIndicatorSummary()
	if err != nil {
		// Calculate if summary doesn't exist
		indicators, err := s.CalculateAllIndicators()
		if err != nil {
			return nil, err
		}
		summary = &IndicatorSummaryFile{Stocks: indicators}
	}

	var results []string

	for code, ind := range summary.Stocks {
		if ind == nil {
			continue
		}

		// Apply filters
		if filter.RSAvgMin > 0 && ind.RSAvg < filter.RSAvgMin {
			continue
		}
		if filter.RSAvgMax > 0 && ind.RSAvg > filter.RSAvgMax {
			continue
		}
		if filter.RSIMin > 0 && ind.RSI < filter.RSIMin {
			continue
		}
		if filter.RSIMax > 0 && ind.RSI > filter.RSIMax {
			continue
		}
		if filter.MACDHistPositive != nil {
			if *filter.MACDHistPositive && ind.MACDHist <= 0 {
				continue
			}
			if !*filter.MACDHistPositive && ind.MACDHist > 0 {
				continue
			}
		}
		if filter.AboveMA50 != nil && ind.MA50 > 0 {
			if *filter.AboveMA50 && ind.CurrentPrice <= ind.MA50 {
				continue
			}
			if !*filter.AboveMA50 && ind.CurrentPrice > ind.MA50 {
				continue
			}
		}
		if filter.AboveMA200 != nil && ind.MA200 > 0 {
			if *filter.AboveMA200 && ind.CurrentPrice <= ind.MA200 {
				continue
			}
			if !*filter.AboveMA200 && ind.CurrentPrice > ind.MA200 {
				continue
			}
		}
		if filter.MinVolume > 0 && ind.AvgVol < filter.MinVolume {
			continue
		}

		results = append(results, code)
	}

	return results, nil
}
