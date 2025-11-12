package backtesting

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"go_backend_project/models"
	"go_backend_project/services/analysis"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// BacktestEngine handles backtesting of trading strategies
type BacktestEngine struct {
	db                *gorm.DB
	technicalAnalysis *analysis.TechnicalAnalysis
}

// NewBacktestEngine creates a new backtest engine
func NewBacktestEngine(db *gorm.DB) *BacktestEngine {
	return &BacktestEngine{
		db:                db,
		technicalAnalysis: analysis.NewTechnicalAnalysis(db),
	}
}

// BacktestConfig holds backtesting configuration
type BacktestConfig struct {
	StrategyID     uint
	StartDate      time.Time
	EndDate        time.Time
	InitialCapital decimal.Decimal
	Commission     decimal.Decimal // Commission rate (e.g., 0.15% = 0.0015)
	Symbols        []string        // Stocks to backtest
	RiskPerTrade   decimal.Decimal // Risk per trade as % of capital
}

// Position represents an open position
type Position struct {
	StockID       uint
	Symbol        string
	Quantity      int64
	EntryPrice    decimal.Decimal
	EntryDate     time.Time
	CurrentPrice  decimal.Decimal
	UnrealizedPnL decimal.Decimal
}

// BacktestState holds current backtest state
type BacktestState struct {
	Cash            decimal.Decimal
	Equity          decimal.Decimal
	Positions       map[uint]*Position
	ClosedTrades    []models.BacktestTrade
	DailyEquity     map[string]decimal.Decimal
	MaxEquity       decimal.Decimal
	MaxDrawdown     decimal.Decimal
}

// RunBacktest executes a backtest
func (be *BacktestEngine) RunBacktest(config *BacktestConfig) (*models.Backtest, error) {
	// Create backtest record
	backtest := &models.Backtest{
		Name:           fmt.Sprintf("Backtest %s", time.Now().Format("2006-01-02 15:04:05")),
		StrategyID:     config.StrategyID,
		StartDate:      config.StartDate,
		EndDate:        config.EndDate,
		InitialCapital: config.InitialCapital,
	}

	if err := be.db.Create(backtest).Error; err != nil {
		return nil, fmt.Errorf("failed to create backtest: %w", err)
	}

	// Initialize state
	state := &BacktestState{
		Cash:        config.InitialCapital,
		Equity:      config.InitialCapital,
		Positions:   make(map[uint]*Position),
		DailyEquity: make(map[string]decimal.Decimal),
		MaxEquity:   config.InitialCapital,
	}

	// Load strategy
	var strategy models.TradingStrategy
	if err := be.db.First(&strategy, config.StrategyID).Error; err != nil {
		return nil, fmt.Errorf("strategy not found: %w", err)
	}

	// Get stocks to backtest
	var stocks []models.Stock
	if err := be.db.Where("symbol IN ?", config.Symbols).Find(&stocks).Error; err != nil {
		return nil, fmt.Errorf("failed to load stocks: %w", err)
	}

	// Iterate through each trading day
	currentDate := config.StartDate
	for currentDate.Before(config.EndDate) || currentDate.Equal(config.EndDate) {
		// Skip weekends
		if currentDate.Weekday() == time.Saturday || currentDate.Weekday() == time.Sunday {
			currentDate = currentDate.AddDate(0, 0, 1)
			continue
		}

		// Process each stock
		for _, stock := range stocks {
			// Get price data for the day
			var price models.StockPrice
			err := be.db.Where("stock_id = ? AND DATE(date) = ?", stock.ID, currentDate.Format("2006-01-02")).
				First(&price).Error

			if err != nil {
				continue // No data for this day
			}

			// Update current positions
			if pos, exists := state.Positions[stock.ID]; exists {
				pos.CurrentPrice = price.Close
				pos.UnrealizedPnL = price.Close.Sub(pos.EntryPrice).Mul(decimal.NewFromInt(pos.Quantity))
			}

			// Generate signals based on strategy
			signal := be.generateSignal(&strategy, stock.ID, currentDate)

			// Execute trades based on signals
			if signal == "BUY" && state.Cash.GreaterThan(decimal.Zero) {
				be.executeBuy(backtest.ID, stock, &price, state, config)
			} else if signal == "SELL" {
				if _, hasPosition := state.Positions[stock.ID]; hasPosition {
					be.executeSell(backtest.ID, stock, &price, state, config)
				}
			}
		}

		// Calculate daily equity
		totalEquity := state.Cash
		for _, pos := range state.Positions {
			totalEquity = totalEquity.Add(pos.CurrentPrice.Mul(decimal.NewFromInt(pos.Quantity)))
		}
		state.Equity = totalEquity
		state.DailyEquity[currentDate.Format("2006-01-02")] = totalEquity

		// Track max drawdown
		if totalEquity.GreaterThan(state.MaxEquity) {
			state.MaxEquity = totalEquity
		}

		drawdown := state.MaxEquity.Sub(totalEquity).Div(state.MaxEquity)
		if drawdown.GreaterThan(state.MaxDrawdown) {
			state.MaxDrawdown = drawdown
		}

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Close all remaining positions at end date
	for stockID := range state.Positions {
		var stock models.Stock
		be.db.First(&stock, stockID)

		var price models.StockPrice
		err := be.db.Where("stock_id = ? AND date <= ?", stockID, config.EndDate).
			Order("date DESC").
			First(&price).Error

		if err == nil {
			be.executeSell(backtest.ID, &stock, &price, state, config)
		}
	}

	// Calculate metrics
	be.calculateMetrics(backtest, state, config)

	// Save results
	resultsJSON, _ := json.Marshal(state.DailyEquity)
	backtest.Results = string(resultsJSON)
	completedAt := time.Now()
	backtest.CompletedAt = &completedAt

	if err := be.db.Save(backtest).Error; err != nil {
		return nil, fmt.Errorf("failed to save backtest results: %w", err)
	}

	return backtest, nil
}

// generateSignal generates trading signal based on strategy
func (be *BacktestEngine) generateSignal(strategy *models.TradingStrategy, stockID uint, date time.Time) string {
	// Parse strategy parameters
	var params map[string]interface{}
	json.Unmarshal([]byte(strategy.Parameters), &params)

	// Implement different strategy types
	switch strategy.Type {
	case "sma_crossover":
		return be.smaCrossoverSignal(stockID, date, params)
	case "rsi_strategy":
		return be.rsiSignal(stockID, date, params)
	case "macd_strategy":
		return be.macdSignal(stockID, date, params)
	default:
		return "HOLD"
	}
}

// smaCrossoverSignal implements SMA crossover strategy
func (be *BacktestEngine) smaCrossoverSignal(stockID uint, date time.Time, params map[string]interface{}) string {
	shortPeriod := 20
	longPeriod := 50

	if p, ok := params["short_period"].(float64); ok {
		shortPeriod = int(p)
	}
	if p, ok := params["long_period"].(float64); ok {
		longPeriod = int(p)
	}

	smaShort, err := be.technicalAnalysis.CalculateSMA(stockID, shortPeriod, date)
	if err != nil {
		return "HOLD"
	}

	smaLong, err := be.technicalAnalysis.CalculateSMA(stockID, longPeriod, date)
	if err != nil {
		return "HOLD"
	}

	// Get previous day's SMAs
	prevDate := date.AddDate(0, 0, -1)
	prevSMAShort, err := be.technicalAnalysis.CalculateSMA(stockID, shortPeriod, prevDate)
	if err != nil {
		return "HOLD"
	}

	prevSMALong, err := be.technicalAnalysis.CalculateSMA(stockID, longPeriod, prevDate)
	if err != nil {
		return "HOLD"
	}

	// Bullish crossover: short SMA crosses above long SMA
	if prevSMAShort.LessThanOrEqual(prevSMALong) && smaShort.GreaterThan(smaLong) {
		return "BUY"
	}

	// Bearish crossover: short SMA crosses below long SMA
	if prevSMAShort.GreaterThanOrEqual(prevSMALong) && smaShort.LessThan(smaLong) {
		return "SELL"
	}

	return "HOLD"
}

// rsiSignal implements RSI-based strategy
func (be *BacktestEngine) rsiSignal(stockID uint, date time.Time, params map[string]interface{}) string {
	rsi, err := be.technicalAnalysis.CalculateRSI(stockID, 14, date)
	if err != nil {
		return "HOLD"
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
		return "BUY"
	}
	if rsi.GreaterThan(overbought) {
		return "SELL"
	}

	return "HOLD"
}

// macdSignal implements MACD strategy
func (be *BacktestEngine) macdSignal(stockID uint, date time.Time, params map[string]interface{}) string {
	macd, err := be.technicalAnalysis.CalculateMACD(stockID, date)
	if err != nil {
		return "HOLD"
	}

	// Buy when MACD crosses above signal line (positive histogram)
	if macd.Histogram.GreaterThan(decimal.Zero) {
		return "BUY"
	}

	// Sell when MACD crosses below signal line (negative histogram)
	if macd.Histogram.LessThan(decimal.Zero) {
		return "SELL"
	}

	return "HOLD"
}

// executeBuy executes a buy order
func (be *BacktestEngine) executeBuy(backtestID uint, stock *models.Stock, price *models.StockPrice, state *BacktestState, config *BacktestConfig) {
	// Calculate position size based on risk
	positionSize := state.Cash.Mul(config.RiskPerTrade)
	quantity := positionSize.Div(price.Close).IntPart()

	if quantity <= 0 {
		return
	}

	totalCost := price.Close.Mul(decimal.NewFromInt(quantity))
	commission := totalCost.Mul(config.Commission)
	totalAmount := totalCost.Add(commission)

	if totalAmount.GreaterThan(state.Cash) {
		return // Not enough cash
	}

	// Create position
	state.Positions[stock.ID] = &Position{
		StockID:      stock.ID,
		Symbol:       stock.Symbol,
		Quantity:     quantity,
		EntryPrice:   price.Close,
		EntryDate:    price.Date,
		CurrentPrice: price.Close,
	}

	state.Cash = state.Cash.Sub(totalAmount)

	// Record trade
	trade := models.BacktestTrade{
		BacktestID: backtestID,
		StockID:    stock.ID,
		Type:       "BUY",
		Date:       price.Date,
		Quantity:   quantity,
		Price:      price.Close,
		Commission: commission,
		Signal:     "Strategy signal",
	}
	be.db.Create(&trade)
}

// executeSell executes a sell order
func (be *BacktestEngine) executeSell(backtestID uint, stock *models.Stock, price *models.StockPrice, state *BacktestState, config *BacktestConfig) {
	pos, exists := state.Positions[stock.ID]
	if !exists {
		return
	}

	totalRevenue := price.Close.Mul(decimal.NewFromInt(pos.Quantity))
	commission := totalRevenue.Mul(config.Commission)
	netRevenue := totalRevenue.Sub(commission)

	pnl := netRevenue.Sub(pos.EntryPrice.Mul(decimal.NewFromInt(pos.Quantity)))

	state.Cash = state.Cash.Add(netRevenue)
	delete(state.Positions, stock.ID)

	// Record trade
	trade := models.BacktestTrade{
		BacktestID: backtestID,
		StockID:    stock.ID,
		Type:       "SELL",
		Date:       price.Date,
		Quantity:   pos.Quantity,
		Price:      price.Close,
		Commission: commission,
		PnL:        pnl,
		Signal:     "Strategy signal",
	}
	be.db.Create(&trade)
	state.ClosedTrades = append(state.ClosedTrades, trade)
}

// calculateMetrics calculates backtest performance metrics
func (be *BacktestEngine) calculateMetrics(backtest *models.Backtest, state *BacktestState, config *BacktestConfig) {
	backtest.FinalCapital = state.Equity

	// Total return
	totalReturn := state.Equity.Sub(config.InitialCapital).Div(config.InitialCapital)
	backtest.TotalReturn = totalReturn

	// Annual return
	days := config.EndDate.Sub(config.StartDate).Hours() / 24
	years := days / 365.0
	annualReturn := decimal.NewFromFloat(math.Pow(1+totalReturn.InexactFloat64(), 1/years) - 1)
	backtest.AnnualReturn = annualReturn

	// Max drawdown
	backtest.MaxDrawdown = state.MaxDrawdown

	// Trade statistics
	backtest.TotalTrades = len(state.ClosedTrades)
	winningTrades := 0
	losingTrades := 0
	totalWin := decimal.Zero
	totalLoss := decimal.Zero

	for _, trade := range state.ClosedTrades {
		if trade.PnL.GreaterThan(decimal.Zero) {
			winningTrades++
			totalWin = totalWin.Add(trade.PnL)
		} else {
			losingTrades++
			totalLoss = totalLoss.Add(trade.PnL.Abs())
		}
	}

	backtest.WinningTrades = winningTrades
	backtest.LosingTrades = losingTrades

	if backtest.TotalTrades > 0 {
		backtest.WinRate = decimal.NewFromInt(int64(winningTrades)).Div(
			decimal.NewFromInt(int64(backtest.TotalTrades)),
		)
	}

	if winningTrades > 0 {
		backtest.AvgWin = totalWin.Div(decimal.NewFromInt(int64(winningTrades)))
	}

	if losingTrades > 0 {
		backtest.AvgLoss = totalLoss.Div(decimal.NewFromInt(int64(losingTrades)))
	}

	// Profit factor
	if totalLoss.GreaterThan(decimal.Zero) {
		backtest.ProfitFactor = totalWin.Div(totalLoss)
	}

	// Simplified Sharpe ratio (would need risk-free rate and more data in production)
	backtest.SharpeRatio = totalReturn.Div(state.MaxDrawdown.Add(decimal.NewFromFloat(0.01)))
}
