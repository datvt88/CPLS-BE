package trading

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"go_backend_project/models"
	"go_backend_project/services/analysis"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TradingBot handles automated trading
type TradingBot struct {
	db                *gorm.DB
	technicalAnalysis *analysis.TechnicalAnalysis
	isRunning         bool
	stopChan          chan bool
	mutex             sync.RWMutex
	strategies        map[uint]*models.TradingStrategy
}

// NewTradingBot creates a new trading bot instance
func NewTradingBot(db *gorm.DB) *TradingBot {
	return &TradingBot{
		db:                db,
		technicalAnalysis: analysis.NewTechnicalAnalysis(db),
		stopChan:          make(chan bool),
		strategies:        make(map[uint]*models.TradingStrategy),
	}
}

// Start starts the trading bot
func (bot *TradingBot) Start() error {
	bot.mutex.Lock()
	defer bot.mutex.Unlock()

	if bot.isRunning {
		return fmt.Errorf("trading bot is already running")
	}

	// Load active strategies
	if err := bot.loadStrategies(); err != nil {
		return fmt.Errorf("failed to load strategies: %w", err)
	}

	bot.isRunning = true
	go bot.run()

	log.Println("Trading bot started successfully")
	return nil
}

// Stop stops the trading bot
func (bot *TradingBot) Stop() {
	bot.mutex.Lock()
	defer bot.mutex.Unlock()

	if !bot.isRunning {
		return
	}

	bot.stopChan <- true
	bot.isRunning = false
	log.Println("Trading bot stopped")
}

// IsRunning returns whether the bot is running
func (bot *TradingBot) IsRunning() bool {
	bot.mutex.RLock()
	defer bot.mutex.RUnlock()
	return bot.isRunning
}

// run is the main trading bot loop
func (bot *TradingBot) run() {
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()

	for {
		select {
		case <-bot.stopChan:
			return
		case <-ticker.C:
			bot.executeTradingCycle()
		}
	}
}

// loadStrategies loads all active trading strategies
func (bot *TradingBot) loadStrategies() error {
	var strategies []models.TradingStrategy
	if err := bot.db.Where("is_active = ?", true).Find(&strategies).Error; err != nil {
		return err
	}

	bot.strategies = make(map[uint]*models.TradingStrategy)
	for i := range strategies {
		bot.strategies[strategies[i].ID] = &strategies[i]
	}

	log.Printf("Loaded %d active strategies", len(bot.strategies))
	return nil
}

// executeTradingCycle executes one trading cycle
func (bot *TradingBot) executeTradingCycle() {
	// Check if market is open (Vietnamese stock market: 9:00-15:00)
	now := time.Now()
	hour := now.Hour()
	if hour < 9 || hour >= 15 {
		return // Market closed
	}

	// Skip weekends
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return
	}

	log.Println("Executing trading cycle...")

	// Get all stocks
	var stocks []models.Stock
	if err := bot.db.Where("status = ?", "active").Find(&stocks).Error; err != nil {
		log.Printf("Error loading stocks: %v", err)
		return
	}

	// Process each strategy
	for _, strategy := range bot.strategies {
		bot.executeStrategy(strategy, stocks)
	}
}

// executeStrategy executes a trading strategy for all stocks
func (bot *TradingBot) executeStrategy(strategy *models.TradingStrategy, stocks []models.Stock) {
	for _, stock := range stocks {
		// Generate signal
		signal := bot.generateSignal(strategy, stock.ID)
		if signal == nil {
			continue
		}

		// Execute trade based on signal
		if signal.Type == "BUY" && signal.Confidence.GreaterThan(decimal.NewFromInt(70)) {
			bot.executeBuyOrder(signal, &stock, strategy)
		} else if signal.Type == "SELL" && signal.Confidence.GreaterThan(decimal.NewFromInt(70)) {
			bot.executeSellOrder(signal, &stock, strategy)
		}
	}
}

// generateSignal generates a trading signal for a stock
func (bot *TradingBot) generateSignal(strategy *models.TradingStrategy, stockID uint) *models.Signal {
	now := time.Now()

	// Get latest price
	var latestPrice models.StockPrice
	if err := bot.db.Where("stock_id = ?", stockID).Order("date DESC").First(&latestPrice).Error; err != nil {
		return nil
	}

	// Parse strategy parameters
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(strategy.Parameters), &params); err != nil {
		log.Printf("Error parsing strategy parameters: %v", err)
		return nil
	}

	var signalType string
	var confidence decimal.Decimal
	var reason string
	var targetPrice, stopLoss decimal.Decimal

	// Generate signal based on strategy type
	switch strategy.Type {
	case "sma_crossover":
		signalType, confidence, reason = bot.smaCrossoverStrategy(stockID, params, latestPrice)
	case "rsi_strategy":
		signalType, confidence, reason = bot.rsiStrategy(stockID, params, latestPrice)
	case "macd_strategy":
		signalType, confidence, reason = bot.macdStrategy(stockID, params, latestPrice)
	case "breakout_strategy":
		signalType, confidence, reason = bot.breakoutStrategy(stockID, params, latestPrice)
	default:
		return nil
	}

	if signalType == "HOLD" {
		return nil
	}

	// Calculate target price and stop loss
	if signalType == "BUY" {
		targetPrice = latestPrice.Close.Mul(decimal.NewFromFloat(1.05)) // 5% target
		stopLoss = latestPrice.Close.Mul(decimal.NewFromFloat(0.97))    // 3% stop loss
	} else if signalType == "SELL" {
		targetPrice = latestPrice.Close.Mul(decimal.NewFromFloat(0.95))
		stopLoss = latestPrice.Close.Mul(decimal.NewFromFloat(1.03))
	}

	signal := &models.Signal{
		StockID:     stockID,
		StrategyID:  strategy.ID,
		Type:        signalType,
		Strength:    confidence,
		Price:       latestPrice.Close,
		TargetPrice: targetPrice,
		StopLoss:    stopLoss,
		Confidence:  confidence,
		Reason:      reason,
		IsActive:    true,
		CreatedAt:   now,
	}

	// Save signal
	if err := bot.db.Create(signal).Error; err != nil {
		log.Printf("Error saving signal: %v", err)
		return nil
	}

	return signal
}

// smaCrossoverStrategy implements SMA crossover logic
func (bot *TradingBot) smaCrossoverStrategy(stockID uint, params map[string]interface{}, latestPrice models.StockPrice) (string, decimal.Decimal, string) {
	shortPeriod := 20
	longPeriod := 50

	if p, ok := params["short_period"].(float64); ok {
		shortPeriod = int(p)
	}
	if p, ok := params["long_period"].(float64); ok {
		longPeriod = int(p)
	}

	smaShort, err := bot.technicalAnalysis.CalculateSMA(stockID, shortPeriod, time.Now())
	if err != nil {
		return "HOLD", decimal.Zero, ""
	}

	smaLong, err := bot.technicalAnalysis.CalculateSMA(stockID, longPeriod, time.Now())
	if err != nil {
		return "HOLD", decimal.Zero, ""
	}

	// Calculate distance between SMAs as confidence
	distance := smaShort.Sub(smaLong).Div(smaLong).Abs().Mul(decimal.NewFromInt(100))

	if smaShort.GreaterThan(smaLong) {
		confidence := decimal.NewFromInt(70).Add(distance.Mul(decimal.NewFromFloat(3)))
		if confidence.GreaterThan(decimal.NewFromInt(100)) {
			confidence = decimal.NewFromInt(100)
		}
		return "BUY", confidence, fmt.Sprintf("SMA%d crossed above SMA%d", shortPeriod, longPeriod)
	}

	if smaShort.LessThan(smaLong) {
		confidence := decimal.NewFromInt(70).Add(distance.Mul(decimal.NewFromFloat(3)))
		if confidence.GreaterThan(decimal.NewFromInt(100)) {
			confidence = decimal.NewFromInt(100)
		}
		return "SELL", confidence, fmt.Sprintf("SMA%d crossed below SMA%d", shortPeriod, longPeriod)
	}

	return "HOLD", decimal.Zero, ""
}

// rsiStrategy implements RSI-based logic
func (bot *TradingBot) rsiStrategy(stockID uint, params map[string]interface{}, latestPrice models.StockPrice) (string, decimal.Decimal, string) {
	rsi, err := bot.technicalAnalysis.CalculateRSI(stockID, 14, time.Now())
	if err != nil {
		return "HOLD", decimal.Zero, ""
	}

	oversold := decimal.NewFromInt(30)
	overbought := decimal.NewFromInt(70)

	if p, ok := params["oversold"].(float64); ok {
		oversold = decimal.NewFromFloat(p)
	}
	if p, ok := params["overbought"].(float64); ok {
		overbought = decimal.NewFromFloat(p)
	}

	if rsi.LessThan(oversold) {
		confidence := oversold.Sub(rsi).Mul(decimal.NewFromFloat(2)).Add(decimal.NewFromInt(70))
		if confidence.GreaterThan(decimal.NewFromInt(100)) {
			confidence = decimal.NewFromInt(100)
		}
		return "BUY", confidence, fmt.Sprintf("RSI oversold at %s", rsi.StringFixed(2))
	}

	if rsi.GreaterThan(overbought) {
		confidence := rsi.Sub(overbought).Mul(decimal.NewFromFloat(2)).Add(decimal.NewFromInt(70))
		if confidence.GreaterThan(decimal.NewFromInt(100)) {
			confidence = decimal.NewFromInt(100)
		}
		return "SELL", confidence, fmt.Sprintf("RSI overbought at %s", rsi.StringFixed(2))
	}

	return "HOLD", decimal.Zero, ""
}

// macdStrategy implements MACD logic
func (bot *TradingBot) macdStrategy(stockID uint, params map[string]interface{}, latestPrice models.StockPrice) (string, decimal.Decimal, string) {
	macd, err := bot.technicalAnalysis.CalculateMACD(stockID, time.Now())
	if err != nil {
		return "HOLD", decimal.Zero, ""
	}

	histogramAbs := macd.Histogram.Abs()
	confidence := decimal.NewFromInt(75).Add(histogramAbs.Mul(decimal.NewFromInt(5)))
	if confidence.GreaterThan(decimal.NewFromInt(100)) {
		confidence = decimal.NewFromInt(100)
	}

	if macd.Histogram.GreaterThan(decimal.Zero) && macd.MACD.GreaterThan(decimal.Zero) {
		return "BUY", confidence, "MACD bullish crossover"
	}

	if macd.Histogram.LessThan(decimal.Zero) && macd.MACD.LessThan(decimal.Zero) {
		return "SELL", confidence, "MACD bearish crossover"
	}

	return "HOLD", decimal.Zero, ""
}

// breakoutStrategy implements breakout logic
func (bot *TradingBot) breakoutStrategy(stockID uint, params map[string]interface{}, latestPrice models.StockPrice) (string, decimal.Decimal, string) {
	period := 20
	if p, ok := params["period"].(float64); ok {
		period = int(p)
	}

	// Get historical high/low
	var prices []models.StockPrice
	err := bot.db.Where("stock_id = ?", stockID).
		Order("date DESC").
		Limit(period).
		Find(&prices).Error

	if err != nil || len(prices) < period {
		return "HOLD", decimal.Zero, ""
	}

	highestHigh := prices[0].High
	lowestLow := prices[0].Low

	for _, price := range prices {
		if price.High.GreaterThan(highestHigh) {
			highestHigh = price.High
		}
		if price.Low.LessThan(lowestLow) {
			lowestLow = price.Low
		}
	}

	// Breakout above resistance
	if latestPrice.Close.GreaterThan(highestHigh) {
		distance := latestPrice.Close.Sub(highestHigh).Div(highestHigh).Mul(decimal.NewFromInt(100))
		confidence := decimal.NewFromInt(75).Add(distance.Mul(decimal.NewFromInt(10)))
		if confidence.GreaterThan(decimal.NewFromInt(100)) {
			confidence = decimal.NewFromInt(100)
		}
		return "BUY", confidence, fmt.Sprintf("Breakout above %d-day high", period)
	}

	// Breakdown below support
	if latestPrice.Close.LessThan(lowestLow) {
		distance := lowestLow.Sub(latestPrice.Close).Div(lowestLow).Mul(decimal.NewFromInt(100))
		confidence := decimal.NewFromInt(75).Add(distance.Mul(decimal.NewFromInt(10)))
		if confidence.GreaterThan(decimal.NewFromInt(100)) {
			confidence = decimal.NewFromInt(100)
		}
		return "SELL", confidence, fmt.Sprintf("Breakdown below %d-day low", period)
	}

	return "HOLD", decimal.Zero, ""
}

// executeBuyOrder executes a buy order
func (bot *TradingBot) executeBuyOrder(signal *models.Signal, stock *models.Stock, strategy *models.TradingStrategy) {
	// In production, this would place an actual order through broker API
	// For now, we'll create a pending trade record

	log.Printf("BUY signal for %s: %s (confidence: %s)", stock.Symbol, signal.Reason, signal.Confidence.StringFixed(2))

	// Calculate quantity based on risk management
	// This is simplified - in production, use proper position sizing
	quantity := int64(100) // Example: 100 shares

	trade := models.Trade{
		UserID:     1, // System user
		StockID:    stock.ID,
		StrategyID: strategy.ID,
		Type:       "BUY",
		Quantity:   quantity,
		Price:      signal.Price,
		Commission: signal.Price.Mul(decimal.NewFromInt(quantity)).Mul(decimal.NewFromFloat(0.0015)),
		Status:     "pending",
		OrderType:  "limit",
	}

	trade.TotalAmount = trade.Price.Mul(decimal.NewFromInt(quantity)).Add(trade.Commission)

	if err := bot.db.Create(&trade).Error; err != nil {
		log.Printf("Error creating buy order: %v", err)
		return
	}

	log.Printf("Buy order created for %s: %d shares at %s", stock.Symbol, quantity, signal.Price.StringFixed(2))
}

// executeSellOrder executes a sell order
func (bot *TradingBot) executeSellOrder(signal *models.Signal, stock *models.Stock, strategy *models.TradingStrategy) {
	// Check if we have a position in this stock
	var portfolio models.Portfolio
	err := bot.db.Where("stock_id = ? AND quantity > 0", stock.ID).First(&portfolio).Error
	if err != nil {
		return // No position to sell
	}

	log.Printf("SELL signal for %s: %s (confidence: %s)", stock.Symbol, signal.Reason, signal.Confidence.StringFixed(2))

	trade := models.Trade{
		UserID:     1, // System user
		StockID:    stock.ID,
		StrategyID: strategy.ID,
		Type:       "SELL",
		Quantity:   portfolio.Quantity,
		Price:      signal.Price,
		Commission: signal.Price.Mul(decimal.NewFromInt(portfolio.Quantity)).Mul(decimal.NewFromFloat(0.0015)),
		Status:     "pending",
		OrderType:  "limit",
	}

	trade.TotalAmount = trade.Price.Mul(decimal.NewFromInt(portfolio.Quantity)).Sub(trade.Commission)

	if err := bot.db.Create(&trade).Error; err != nil {
		log.Printf("Error creating sell order: %v", err)
		return
	}

	log.Printf("Sell order created for %s: %d shares at %s", stock.Symbol, portfolio.Quantity, signal.Price.StringFixed(2))
}

// ManualTrade allows manual trade execution
func (bot *TradingBot) ManualTrade(userID uint, stockID uint, tradeType string, quantity int64, price decimal.Decimal) error {
	var stock models.Stock
	if err := bot.db.First(&stock, stockID).Error; err != nil {
		return fmt.Errorf("stock not found: %w", err)
	}

	commission := price.Mul(decimal.NewFromInt(quantity)).Mul(decimal.NewFromFloat(0.0015))

	trade := models.Trade{
		UserID:     userID,
		StockID:    stockID,
		Type:       tradeType,
		Quantity:   quantity,
		Price:      price,
		Commission: commission,
		Status:     "pending",
		OrderType:  "market",
	}

	if tradeType == "BUY" {
		trade.TotalAmount = price.Mul(decimal.NewFromInt(quantity)).Add(commission)
	} else {
		trade.TotalAmount = price.Mul(decimal.NewFromInt(quantity)).Sub(commission)
	}

	if err := bot.db.Create(&trade).Error; err != nil {
		return fmt.Errorf("failed to create trade: %w", err)
	}

	log.Printf("Manual %s order created: %s %d shares at %s", tradeType, stock.Symbol, quantity, price.StringFixed(2))
	return nil
}
