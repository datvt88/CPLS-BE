package screener

import (
	"fmt"
	"sort"

	"go_backend_project/models"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// StockScreener provides stock filtering and screening capabilities
type StockScreener struct {
	db *gorm.DB
}

// NewStockScreener creates a new stock screener instance
func NewStockScreener(db *gorm.DB) *StockScreener {
	return &StockScreener{db: db}
}

// ScreenerFilter represents filter criteria for stock screening
type ScreenerFilter struct {
	Exchange          []string `json:"exchange"`           // HOSE, HNX, UPCOM
	Industry          []string `json:"industry"`           // Banking, Technology, etc.
	Sector            []string `json:"sector"`             // Finance, IT, etc.
	MinPrice          *float64 `json:"min_price"`          // Minimum price
	MaxPrice          *float64 `json:"max_price"`          // Maximum price
	MinVolume         *int64   `json:"min_volume"`         // Minimum volume
	MaxVolume         *int64   `json:"max_volume"`         // Maximum volume
	MinMarketCap      *float64 `json:"min_market_cap"`     // Minimum market cap
	MaxMarketCap      *float64 `json:"max_market_cap"`     // Maximum market cap
	MinChangePercent  *float64 `json:"min_change_percent"` // Min daily change %
	MaxChangePercent  *float64 `json:"max_change_percent"` // Max daily change %
	MinRSI            *float64 `json:"min_rsi"`            // Minimum RSI
	MaxRSI            *float64 `json:"max_rsi"`            // Maximum RSI
	AboveSMA20        *bool    `json:"above_sma20"`        // Price above SMA20
	AboveSMA50        *bool    `json:"above_sma50"`        // Price above SMA50
	AboveSMA200       *bool    `json:"above_sma200"`       // Price above SMA200
	MACDBullish       *bool    `json:"macd_bullish"`       // MACD bullish crossover
	GoldenCross       *bool    `json:"golden_cross"`       // SMA50 above SMA200
	DeathCross        *bool    `json:"death_cross"`        // SMA50 below SMA200
	VolumeSpike       *float64 `json:"volume_spike"`       // Volume N times above average
	NewHighDays       *int     `json:"new_high_days"`      // New N-day high
	NewLowDays        *int     `json:"new_low_days"`       // New N-day low
	SortBy            string   `json:"sort_by"`            // Sort field
	SortOrder         string   `json:"sort_order"`         // asc, desc
	Page              int      `json:"page"`
	Limit             int      `json:"limit"`
}

// ScreenerResult represents a stock screening result
type ScreenerResult struct {
	Stock         models.Stock      `json:"stock"`
	LatestPrice   models.StockPrice `json:"latest_price"`
	Indicators    map[string]decimal.Decimal `json:"indicators"`
	MatchedCriteria []string        `json:"matched_criteria"`
}

// Screen applies filters and returns matching stocks
func (ss *StockScreener) Screen(filter *ScreenerFilter) ([]ScreenerResult, int64, error) {
	// Set defaults
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "desc"
	}
	if filter.SortBy == "" {
		filter.SortBy = "volume"
	}

	offset := (filter.Page - 1) * filter.Limit

	// Base query for stocks
	query := ss.db.Model(&models.Stock{}).Where("status = ?", "active")

	// Apply exchange filter
	if len(filter.Exchange) > 0 {
		query = query.Where("exchange IN ?", filter.Exchange)
	}

	// Apply industry filter
	if len(filter.Industry) > 0 {
		query = query.Where("industry IN ?", filter.Industry)
	}

	// Apply sector filter
	if len(filter.Sector) > 0 {
		query = query.Where("sector IN ?", filter.Sector)
	}

	// Apply market cap filter
	if filter.MinMarketCap != nil {
		query = query.Where("market_cap >= ?", *filter.MinMarketCap)
	}
	if filter.MaxMarketCap != nil {
		query = query.Where("market_cap <= ?", *filter.MaxMarketCap)
	}

	// Get total count
	var total int64
	query.Count(&total)

	// Get stocks
	var stocks []models.Stock
	if err := query.Limit(filter.Limit).Offset(offset).Find(&stocks).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch stocks: %w", err)
	}

	// Process each stock and apply price-based filters
	var results []ScreenerResult
	for _, stock := range stocks {
		// Get latest price
		var latestPrice models.StockPrice
		if err := ss.db.Where("stock_id = ?", stock.ID).Order("date DESC").First(&latestPrice).Error; err != nil {
			continue
		}

		// Apply price filters
		priceFloat, _ := latestPrice.Close.Float64()
		if filter.MinPrice != nil && priceFloat < *filter.MinPrice {
			continue
		}
		if filter.MaxPrice != nil && priceFloat > *filter.MaxPrice {
			continue
		}

		// Apply volume filter
		if filter.MinVolume != nil && latestPrice.Volume < *filter.MinVolume {
			continue
		}
		if filter.MaxVolume != nil && latestPrice.Volume > *filter.MaxVolume {
			continue
		}

		// Apply change percent filter
		changeFloat, _ := latestPrice.ChangePercent.Float64()
		if filter.MinChangePercent != nil && changeFloat < *filter.MinChangePercent {
			continue
		}
		if filter.MaxChangePercent != nil && changeFloat > *filter.MaxChangePercent {
			continue
		}

		// Get indicators
		indicators := ss.getIndicators(stock.ID)
		matchedCriteria := []string{}

		// Apply RSI filter
		if rsi, ok := indicators["RSI14"]; ok {
			rsiFloat, _ := rsi.Float64()
			if filter.MinRSI != nil && rsiFloat < *filter.MinRSI {
				continue
			}
			if filter.MaxRSI != nil && rsiFloat > *filter.MaxRSI {
				continue
			}
			if rsiFloat < 30 {
				matchedCriteria = append(matchedCriteria, "RSI Oversold")
			} else if rsiFloat > 70 {
				matchedCriteria = append(matchedCriteria, "RSI Overbought")
			}
		}

		// Apply SMA filters
		if filter.AboveSMA20 != nil && *filter.AboveSMA20 {
			if sma, ok := indicators["SMA20"]; ok {
				if latestPrice.Close.LessThan(sma) {
					continue
				}
				matchedCriteria = append(matchedCriteria, "Above SMA20")
			}
		}

		if filter.AboveSMA50 != nil && *filter.AboveSMA50 {
			if sma, ok := indicators["SMA50"]; ok {
				if latestPrice.Close.LessThan(sma) {
					continue
				}
				matchedCriteria = append(matchedCriteria, "Above SMA50")
			}
		}

		if filter.AboveSMA200 != nil && *filter.AboveSMA200 {
			if sma, ok := indicators["SMA200"]; ok {
				if latestPrice.Close.LessThan(sma) {
					continue
				}
				matchedCriteria = append(matchedCriteria, "Above SMA200")
			}
		}

		// Apply Golden Cross / Death Cross filter
		sma50, hasSMA50 := indicators["SMA50"]
		sma200, hasSMA200 := indicators["SMA200"]
		if hasSMA50 && hasSMA200 {
			if filter.GoldenCross != nil && *filter.GoldenCross {
				if sma50.LessThan(sma200) {
					continue
				}
				matchedCriteria = append(matchedCriteria, "Golden Cross")
			}
			if filter.DeathCross != nil && *filter.DeathCross {
				if sma50.GreaterThan(sma200) {
					continue
				}
				matchedCriteria = append(matchedCriteria, "Death Cross")
			}
		}

		// Apply MACD filter
		if filter.MACDBullish != nil && *filter.MACDBullish {
			if macdHist, ok := indicators["MACD_Histogram"]; ok {
				if macdHist.LessThanOrEqual(decimal.Zero) {
					continue
				}
				matchedCriteria = append(matchedCriteria, "MACD Bullish")
			}
		}

		// Check for new high/low
		if filter.NewHighDays != nil {
			isNewHigh := ss.checkNewHigh(stock.ID, *filter.NewHighDays, latestPrice.Close)
			if !isNewHigh {
				continue
			}
			matchedCriteria = append(matchedCriteria, fmt.Sprintf("%d-Day High", *filter.NewHighDays))
		}

		if filter.NewLowDays != nil {
			isNewLow := ss.checkNewLow(stock.ID, *filter.NewLowDays, latestPrice.Close)
			if !isNewLow {
				continue
			}
			matchedCriteria = append(matchedCriteria, fmt.Sprintf("%d-Day Low", *filter.NewLowDays))
		}

		// Check for volume spike
		if filter.VolumeSpike != nil {
			avgVolume := ss.getAverageVolume(stock.ID, 20)
			if avgVolume > 0 {
				volumeRatio := float64(latestPrice.Volume) / float64(avgVolume)
				if volumeRatio < *filter.VolumeSpike {
					continue
				}
				matchedCriteria = append(matchedCriteria, fmt.Sprintf("Volume Spike %.1fx", volumeRatio))
			}
		}

		results = append(results, ScreenerResult{
			Stock:           stock,
			LatestPrice:     latestPrice,
			Indicators:      indicators,
			MatchedCriteria: matchedCriteria,
		})
	}

	// Apply sorting
	ss.sortResults(results, filter.SortBy, filter.SortOrder)

	return results, total, nil
}

// GetPresetScreeners returns predefined screener configurations
func (ss *StockScreener) GetPresetScreeners() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":          "oversold",
			"name":        "Oversold Stocks (RSI < 30)",
			"description": "Stocks with RSI below 30, potentially undervalued",
			"filter": ScreenerFilter{
				MaxRSI: func() *float64 { v := 30.0; return &v }(),
			},
		},
		{
			"id":          "overbought",
			"name":        "Overbought Stocks (RSI > 70)",
			"description": "Stocks with RSI above 70, potentially overvalued",
			"filter": ScreenerFilter{
				MinRSI: func() *float64 { v := 70.0; return &v }(),
			},
		},
		{
			"id":          "bullish_trend",
			"name":        "Bullish Trend",
			"description": "Stocks above SMA20, SMA50, and with bullish MACD",
			"filter": ScreenerFilter{
				AboveSMA20:  func() *bool { v := true; return &v }(),
				AboveSMA50:  func() *bool { v := true; return &v }(),
				MACDBullish: func() *bool { v := true; return &v }(),
			},
		},
		{
			"id":          "golden_cross",
			"name":        "Golden Cross",
			"description": "Stocks with SMA50 crossing above SMA200",
			"filter": ScreenerFilter{
				GoldenCross: func() *bool { v := true; return &v }(),
			},
		},
		{
			"id":          "high_volume",
			"name":        "High Volume Breakout",
			"description": "Stocks with volume 2x above average",
			"filter": ScreenerFilter{
				VolumeSpike: func() *float64 { v := 2.0; return &v }(),
			},
		},
		{
			"id":          "new_52_week_high",
			"name":        "New 52-Week High",
			"description": "Stocks at new 52-week highs",
			"filter": ScreenerFilter{
				NewHighDays: func() *int { v := 252; return &v }(),
			},
		},
		{
			"id":          "top_gainers",
			"name":        "Top Gainers Today",
			"description": "Stocks with highest daily gains",
			"filter": ScreenerFilter{
				MinChangePercent: func() *float64 { v := 3.0; return &v }(),
				SortBy:           "change_percent",
				SortOrder:        "desc",
			},
		},
		{
			"id":          "top_losers",
			"name":        "Top Losers Today",
			"description": "Stocks with highest daily losses",
			"filter": ScreenerFilter{
				MaxChangePercent: func() *float64 { v := -3.0; return &v }(),
				SortBy:           "change_percent",
				SortOrder:        "asc",
			},
		},
	}
}

// getIndicators retrieves technical indicators for a stock
func (ss *StockScreener) getIndicators(stockID uint) map[string]decimal.Decimal {
	indicators := make(map[string]decimal.Decimal)

	var dbIndicators []models.TechnicalIndicator
	ss.db.Where("stock_id = ?", stockID).
		Order("date DESC").
		Limit(10).
		Find(&dbIndicators)

	for _, ind := range dbIndicators {
		key := fmt.Sprintf("%s%d", ind.Type, ind.Period)
		if ind.Period == 0 {
			key = ind.Type
		}
		indicators[key] = ind.Value

		// Store MACD components separately
		if ind.Type == "MACD" {
			indicators["MACD_Signal"] = ind.Signal
			indicators["MACD_Histogram"] = ind.Histogram
		}
	}

	return indicators
}

// checkNewHigh checks if the current price is a new N-day high
func (ss *StockScreener) checkNewHigh(stockID uint, days int, currentPrice decimal.Decimal) bool {
	var maxHigh decimal.Decimal
	err := ss.db.Model(&models.StockPrice{}).
		Where("stock_id = ?", stockID).
		Order("date DESC").
		Limit(days).
		Select("MAX(high) as max_high").
		Row().Scan(&maxHigh)

	if err != nil {
		return false
	}

	return currentPrice.GreaterThanOrEqual(maxHigh)
}

// checkNewLow checks if the current price is a new N-day low
func (ss *StockScreener) checkNewLow(stockID uint, days int, currentPrice decimal.Decimal) bool {
	var minLow decimal.Decimal
	err := ss.db.Model(&models.StockPrice{}).
		Where("stock_id = ?", stockID).
		Order("date DESC").
		Limit(days).
		Select("MIN(low) as min_low").
		Row().Scan(&minLow)

	if err != nil {
		return false
	}

	return currentPrice.LessThanOrEqual(minLow)
}

// getAverageVolume calculates average volume for N days
func (ss *StockScreener) getAverageVolume(stockID uint, days int) int64 {
	var avgVolume float64
	err := ss.db.Model(&models.StockPrice{}).
		Where("stock_id = ?", stockID).
		Order("date DESC").
		Limit(days).
		Select("AVG(volume) as avg_volume").
		Row().Scan(&avgVolume)

	if err != nil {
		return 0
	}

	return int64(avgVolume)
}

// sortResults sorts screening results using Go's built-in sort
func (ss *StockScreener) sortResults(results []ScreenerResult, sortBy, sortOrder string) {
	sort.Slice(results, func(i, j int) bool {
		var compare bool

		switch sortBy {
		case "volume":
			compare = results[i].LatestPrice.Volume > results[j].LatestPrice.Volume
		case "change_percent":
			compare = results[i].LatestPrice.ChangePercent.GreaterThan(results[j].LatestPrice.ChangePercent)
		case "price":
			compare = results[i].LatestPrice.Close.GreaterThan(results[j].LatestPrice.Close)
		case "market_cap":
			compare = results[i].Stock.MarketCap.GreaterThan(results[j].Stock.MarketCap)
		default:
			compare = results[i].LatestPrice.Volume > results[j].LatestPrice.Volume
		}

		if sortOrder == "asc" {
			return !compare
		}
		return compare
	})
}
