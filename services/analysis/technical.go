package analysis

import (
	"fmt"
	"math"
	"time"

	"go_backend_project/models"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TechnicalAnalysis provides technical indicator calculations
type TechnicalAnalysis struct {
	db *gorm.DB
}

// NewTechnicalAnalysis creates a new technical analysis instance
func NewTechnicalAnalysis(db *gorm.DB) *TechnicalAnalysis {
	return &TechnicalAnalysis{db: db}
}

// CalculateSMA calculates Simple Moving Average
func (ta *TechnicalAnalysis) CalculateSMA(stockID uint, period int, date time.Time) (decimal.Decimal, error) {
	var prices []models.StockPrice
	err := ta.db.Where("stock_id = ? AND date <= ?", stockID, date).
		Order("date DESC").
		Limit(period).
		Find(&prices).Error

	if err != nil {
		return decimal.Zero, err
	}

	if len(prices) < period {
		return decimal.Zero, fmt.Errorf("insufficient data for SMA%d calculation", period)
	}

	sum := decimal.Zero
	for _, price := range prices {
		sum = sum.Add(price.Close)
	}

	return sum.Div(decimal.NewFromInt(int64(period))), nil
}

// CalculateEMA calculates Exponential Moving Average
func (ta *TechnicalAnalysis) CalculateEMA(stockID uint, period int, date time.Time) (decimal.Decimal, error) {
	var prices []models.StockPrice
	err := ta.db.Where("stock_id = ? AND date <= ?", stockID, date).
		Order("date DESC").
		Limit(period * 3). // Get more data for accurate EMA
		Find(&prices).Error

	if err != nil {
		return decimal.Zero, err
	}

	if len(prices) < period {
		return decimal.Zero, fmt.Errorf("insufficient data for EMA%d calculation", period)
	}

	// Reverse to get chronological order
	for i := 0; i < len(prices)/2; i++ {
		prices[i], prices[len(prices)-1-i] = prices[len(prices)-1-i], prices[i]
	}

	multiplier := decimal.NewFromFloat(2.0 / float64(period+1))
	ema := prices[0].Close // Start with first price

	for i := 1; i < len(prices); i++ {
		ema = prices[i].Close.Sub(ema).Mul(multiplier).Add(ema)
	}

	return ema, nil
}

// CalculateRSI calculates Relative Strength Index
func (ta *TechnicalAnalysis) CalculateRSI(stockID uint, period int, date time.Time) (decimal.Decimal, error) {
	var prices []models.StockPrice
	err := ta.db.Where("stock_id = ? AND date <= ?", stockID, date).
		Order("date DESC").
		Limit(period + 1).
		Find(&prices).Error

	if err != nil {
		return decimal.Zero, err
	}

	if len(prices) < period+1 {
		return decimal.Zero, fmt.Errorf("insufficient data for RSI%d calculation", period)
	}

	// Reverse for chronological order
	for i := 0; i < len(prices)/2; i++ {
		prices[i], prices[len(prices)-1-i] = prices[len(prices)-1-i], prices[i]
	}

	gains := decimal.Zero
	losses := decimal.Zero

	for i := 1; i < len(prices); i++ {
		change := prices[i].Close.Sub(prices[i-1].Close)
		if change.GreaterThan(decimal.Zero) {
			gains = gains.Add(change)
		} else {
			losses = losses.Add(change.Abs())
		}
	}

	avgGain := gains.Div(decimal.NewFromInt(int64(period)))
	avgLoss := losses.Div(decimal.NewFromInt(int64(period)))

	if avgLoss.IsZero() {
		return decimal.NewFromInt(100), nil
	}

	rs := avgGain.Div(avgLoss)
	rsi := decimal.NewFromInt(100).Sub(
		decimal.NewFromInt(100).Div(decimal.NewFromInt(1).Add(rs)),
	)

	return rsi, nil
}

// MACDResult holds MACD calculation results
type MACDResult struct {
	MACD      decimal.Decimal
	Signal    decimal.Decimal
	Histogram decimal.Decimal
}

// CalculateMACD calculates MACD indicator
func (ta *TechnicalAnalysis) CalculateMACD(stockID uint, date time.Time) (*MACDResult, error) {
	// Standard MACD: 12, 26, 9
	ema12, err := ta.CalculateEMA(stockID, 12, date)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate EMA12: %w", err)
	}

	ema26, err := ta.CalculateEMA(stockID, 26, date)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate EMA26: %w", err)
	}

	macd := ema12.Sub(ema26)

	// Calculate signal line (9-day EMA of MACD)
	// In production, you'd calculate this from historical MACD values
	// For now, we'll use a simplified version
	signal := macd.Mul(decimal.NewFromFloat(0.9)) // Simplified

	histogram := macd.Sub(signal)

	return &MACDResult{
		MACD:      macd,
		Signal:    signal,
		Histogram: histogram,
	}, nil
}

// CalculateBollingerBands calculates Bollinger Bands
type BollingerBands struct {
	Upper  decimal.Decimal
	Middle decimal.Decimal
	Lower  decimal.Decimal
}

func (ta *TechnicalAnalysis) CalculateBollingerBands(stockID uint, period int, date time.Time) (*BollingerBands, error) {
	sma, err := ta.CalculateSMA(stockID, period, date)
	if err != nil {
		return nil, err
	}

	var prices []models.StockPrice
	err = ta.db.Where("stock_id = ? AND date <= ?", stockID, date).
		Order("date DESC").
		Limit(period).
		Find(&prices).Error

	if err != nil {
		return nil, err
	}

	// Calculate standard deviation
	var variance float64
	smaFloat, _ := sma.Float64()

	for _, price := range prices {
		closeFloat, _ := price.Close.Float64()
		diff := closeFloat - smaFloat
		variance += diff * diff
	}

	stdDev := math.Sqrt(variance / float64(period))
	stdDevDecimal := decimal.NewFromFloat(stdDev)

	return &BollingerBands{
		Upper:  sma.Add(stdDevDecimal.Mul(decimal.NewFromInt(2))),
		Middle: sma,
		Lower:  sma.Sub(stdDevDecimal.Mul(decimal.NewFromInt(2))),
	}, nil
}

// CalculateStochastic calculates Stochastic Oscillator
type Stochastic struct {
	K decimal.Decimal
	D decimal.Decimal
}

func (ta *TechnicalAnalysis) CalculateStochastic(stockID uint, period int, date time.Time) (*Stochastic, error) {
	var prices []models.StockPrice
	err := ta.db.Where("stock_id = ? AND date <= ?", stockID, date).
		Order("date DESC").
		Limit(period).
		Find(&prices).Error

	if err != nil {
		return nil, err
	}

	if len(prices) < period {
		return nil, fmt.Errorf("insufficient data for Stochastic calculation")
	}

	// Find highest high and lowest low
	highestHigh := prices[0].High
	lowestLow := prices[0].Low
	currentClose := prices[0].Close

	for _, price := range prices {
		if price.High.GreaterThan(highestHigh) {
			highestHigh = price.High
		}
		if price.Low.LessThan(lowestLow) {
			lowestLow = price.Low
		}
	}

	// Calculate %K
	k := currentClose.Sub(lowestLow).Div(highestHigh.Sub(lowestLow)).Mul(decimal.NewFromInt(100))

	// %D is typically a 3-day SMA of %K (simplified here)
	d := k.Mul(decimal.NewFromFloat(0.85)) // Simplified

	return &Stochastic{
		K: k,
		D: d,
	}, nil
}

// SaveIndicator saves calculated indicator to database
func (ta *TechnicalAnalysis) SaveIndicator(stockID uint, date time.Time, indicatorType string, period int, value, signal, histogram decimal.Decimal) error {
	indicator := models.TechnicalIndicator{
		StockID:   stockID,
		Date:      date,
		Type:      indicatorType,
		Period:    period,
		Value:     value,
		Signal:    signal,
		Histogram: histogram,
	}

	// Check if indicator already exists
	var existing models.TechnicalIndicator
	err := ta.db.Where("stock_id = ? AND date = ? AND type = ? AND period = ?",
		stockID, date, indicatorType, period).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		return ta.db.Create(&indicator).Error
	} else if err != nil {
		return err
	}

	// Update existing
	return ta.db.Model(&existing).Updates(indicator).Error
}

// CalculateAllIndicators calculates all indicators for a stock on a given date
func (ta *TechnicalAnalysis) CalculateAllIndicators(stockID uint, date time.Time) error {
	// Calculate and save SMA
	for _, period := range []int{10, 20, 50, 200} {
		sma, err := ta.CalculateSMA(stockID, period, date)
		if err == nil {
			ta.SaveIndicator(stockID, date, "SMA", period, sma, decimal.Zero, decimal.Zero)
		}
	}

	// Calculate and save EMA
	for _, period := range []int{12, 26, 50} {
		ema, err := ta.CalculateEMA(stockID, period, date)
		if err == nil {
			ta.SaveIndicator(stockID, date, "EMA", period, ema, decimal.Zero, decimal.Zero)
		}
	}

	// Calculate and save RSI
	rsi, err := ta.CalculateRSI(stockID, 14, date)
	if err == nil {
		ta.SaveIndicator(stockID, date, "RSI", 14, rsi, decimal.Zero, decimal.Zero)
	}

	// Calculate and save MACD
	macd, err := ta.CalculateMACD(stockID, date)
	if err == nil {
		ta.SaveIndicator(stockID, date, "MACD", 0, macd.MACD, macd.Signal, macd.Histogram)
	}

	return nil
}
