package signals

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"go_backend_project/models"
	"go_backend_project/services"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ConditionEvaluator evaluates signal conditions against stock indicators
type ConditionEvaluator struct {
	db    *gorm.DB
	cache sync.Map
}

// ConditionResult represents the result of evaluating a condition
type ConditionResult struct {
	Condition   *models.SignalCondition
	Passed      bool
	Score       int
	ActualValue float64
	Message     string
}

// GroupEvaluationResult represents the result of evaluating a condition group
type GroupEvaluationResult struct {
	Group      *models.SignalConditionGroup
	Passed     bool
	TotalScore int
	MaxScore   int
	Results    []ConditionResult
}

// RuleSignal represents a signal generated from a rule
type RuleSignal struct {
	Rule         *models.SignalRule
	StockCode    string
	SignalType   string
	Score        int
	MaxScore     int
	Confidence   float64
	Price        float64
	TargetPrice  float64
	StopLoss     float64
	Reasons      []string
	Indicators   map[string]float64
	GroupResults []GroupEvaluationResult
	GeneratedAt  time.Time
}

// ConditionJSON represents a condition in JSON format
type ConditionJSON struct {
	Indicator        string  `json:"indicator"`
	Operator         string  `json:"operator"`
	Value            float64 `json:"value"`
	Value2           float64 `json:"value2,omitempty"`
	CompareIndicator string  `json:"compare_indicator,omitempty"`
	Weight           int     `json:"weight"`
	Required         bool    `json:"required"`
}

// Global condition evaluator instance
var GlobalConditionEvaluator *ConditionEvaluator

// InitConditionEvaluator initializes the condition evaluator
func InitConditionEvaluator(db *gorm.DB) error {
	GlobalConditionEvaluator = &ConditionEvaluator{
		db: db,
	}
	log.Println("Signal Condition Evaluator initialized")
	return nil
}

// GetIndicatorValue gets the value of an indicator from stock data
func (e *ConditionEvaluator) GetIndicatorValue(ind *services.ExtendedStockIndicators, indicator models.IndicatorType) float64 {
	switch indicator {
	case models.IndicatorRSI:
		return ind.RSI
	case models.IndicatorMACD:
		return ind.MACD
	case models.IndicatorMACDSignal:
		return ind.MACDSignal
	case models.IndicatorMACDHistogram:
		return ind.MACDHist
	case models.IndicatorMA10:
		return ind.MA10
	case models.IndicatorMA30:
		return ind.MA30
	case models.IndicatorMA50:
		return ind.MA50
	case models.IndicatorMA200:
		return ind.MA200
	case models.IndicatorRS3D:
		return ind.RS3DRank
	case models.IndicatorRS1M:
		return ind.RS1MRank
	case models.IndicatorRS3M:
		return ind.RS3MRank
	case models.IndicatorRS1Y:
		return ind.RS1YRank
	case models.IndicatorRSAvg:
		return ind.RSAvg
	case models.IndicatorVolume:
		return ind.AvgVol
	case models.IndicatorVolRatio:
		return ind.VolRatio
	case models.IndicatorPrice:
		return ind.CurrentPrice
	case models.IndicatorPriceChange:
		return ind.RS3DChange // 3-day change as proxy for recent price change
	case models.IndicatorTradingValue:
		return ind.AvgTradingVal
	default:
		return 0
	}
}

// EvaluateCondition evaluates a single condition against stock indicators
func (e *ConditionEvaluator) EvaluateCondition(condition *models.SignalCondition, ind *services.ExtendedStockIndicators) *ConditionResult {
	result := &ConditionResult{
		Condition: condition,
		Passed:    false,
		Score:     0,
	}

	actualValue := e.GetIndicatorValue(ind, condition.Indicator)
	result.ActualValue = actualValue

	targetValue := condition.Value.InexactFloat64()
	targetValue2 := condition.Value2.InexactFloat64()

	// Get compare indicator value if specified
	var compareValue float64
	if condition.CompareIndicator != "" {
		compareValue = e.GetIndicatorValue(ind, condition.CompareIndicator)
		// For indicator comparisons, use compareValue as target
		targetValue = compareValue
	}

	// Evaluate based on operator
	switch condition.Operator {
	case models.OperatorEqual:
		result.Passed = math.Abs(actualValue-targetValue) < 0.001
	case models.OperatorNotEqual:
		result.Passed = math.Abs(actualValue-targetValue) >= 0.001
	case models.OperatorGreaterThan:
		result.Passed = actualValue > targetValue
	case models.OperatorGreaterThanEqual:
		result.Passed = actualValue >= targetValue
	case models.OperatorLessThan:
		result.Passed = actualValue < targetValue
	case models.OperatorLessThanEqual:
		result.Passed = actualValue <= targetValue
	case models.OperatorBetween:
		if condition.CompareIndicator != "" {
			// For indicator comparison with between: value represents ratio
			ratio := actualValue / compareValue
			result.Passed = ratio >= targetValue && ratio <= targetValue2
		} else {
			result.Passed = actualValue >= targetValue && actualValue <= targetValue2
		}
	case models.OperatorCrossAbove:
		// This needs historical data - simplified check for now
		// In practice, you'd compare current vs previous values
		result.Passed = actualValue > compareValue && actualValue-compareValue < compareValue*0.05
	case models.OperatorCrossBelow:
		result.Passed = actualValue < compareValue && compareValue-actualValue < compareValue*0.05
	}

	if result.Passed {
		result.Score = condition.Weight
	}

	// Build message
	result.Message = buildConditionMessage(condition, actualValue, targetValue, result.Passed)

	return result
}

// EvaluateConditionGroup evaluates all conditions in a group
func (e *ConditionEvaluator) EvaluateConditionGroup(group *models.SignalConditionGroup, ind *services.ExtendedStockIndicators) *GroupEvaluationResult {
	result := &GroupEvaluationResult{
		Group:   group,
		Passed:  true,
		Results: []ConditionResult{},
	}

	// Sort conditions by order index
	sort.Slice(group.Conditions, func(i, j int) bool {
		return group.Conditions[i].OrderIndex < group.Conditions[j].OrderIndex
	})

	for _, condition := range group.Conditions {
		condResult := e.EvaluateCondition(&condition, ind)
		result.Results = append(result.Results, *condResult)
		result.MaxScore += condition.Weight

		if condResult.Passed {
			result.TotalScore += condResult.Score
		}

		// Check required conditions
		if condition.IsRequired && !condResult.Passed {
			result.Passed = false
		}
	}

	// Apply logical operators between conditions
	if len(group.Conditions) > 0 {
		logicResult := result.Results[0].Passed
		for i := 1; i < len(group.Conditions); i++ {
			cond := group.Conditions[i]
			switch cond.LogicalOperator {
			case models.LogicalAnd:
				logicResult = logicResult && result.Results[i].Passed
			case models.LogicalOr:
				logicResult = logicResult || result.Results[i].Passed
			}
		}
		result.Passed = result.Passed && logicResult
	}

	return result
}

// EvaluateRule evaluates a complete signal rule
func (e *ConditionEvaluator) EvaluateRule(rule *models.SignalRule, ind *services.ExtendedStockIndicators) (*RuleSignal, error) {
	if !rule.IsActive {
		return nil, errors.New("rule is not active")
	}

	signal := &RuleSignal{
		Rule:         rule,
		StockCode:    ind.Code,
		SignalType:   rule.SignalType,
		Price:        ind.CurrentPrice,
		Reasons:      []string{},
		GroupResults: []GroupEvaluationResult{},
		Indicators:   make(map[string]float64),
		GeneratedAt:  time.Now(),
	}

	// Parse condition groups from JSON
	var groupConfigs []struct {
		GroupID  uint   `json:"group_id"`
		Logic    string `json:"logic"` // AND, OR
		Required bool   `json:"required"`
	}

	if rule.ConditionGroups != "" {
		if err := json.Unmarshal([]byte(rule.ConditionGroups), &groupConfigs); err != nil {
			return nil, err
		}
	}

	totalScore := 0
	maxScore := 0
	allPassed := true

	for _, groupConfig := range groupConfigs {
		// Load condition group
		var group models.SignalConditionGroup
		if err := e.db.Preload("Conditions").First(&group, groupConfig.GroupID).Error; err != nil {
			continue
		}

		groupResult := e.EvaluateConditionGroup(&group, ind)
		signal.GroupResults = append(signal.GroupResults, *groupResult)

		totalScore += groupResult.TotalScore
		maxScore += groupResult.MaxScore

		if groupConfig.Required && !groupResult.Passed {
			allPassed = false
		}

		// Add reasons from passed conditions
		for _, condResult := range groupResult.Results {
			if condResult.Passed && condResult.Message != "" {
				signal.Reasons = append(signal.Reasons, condResult.Message)
			}
		}
	}

	signal.Score = totalScore
	signal.MaxScore = maxScore

	if maxScore > 0 {
		signal.Confidence = float64(totalScore) / float64(maxScore)
	}

	// Check if minimum score is met
	scorePercent := 0
	if maxScore > 0 {
		scorePercent = (totalScore * 100) / maxScore
	}

	if !allPassed || scorePercent < rule.MinScore {
		return nil, nil // Signal not triggered
	}

	// Calculate target and stop loss
	targetPercent := rule.TargetPercent.InexactFloat64()
	stopLossPercent := rule.StopLossPercent.InexactFloat64()

	if rule.SignalType == "BUY" || rule.SignalType == "STRONG_BUY" {
		signal.TargetPrice = ind.CurrentPrice * (1 + targetPercent/100)
		signal.StopLoss = ind.CurrentPrice * (1 - stopLossPercent/100)
	} else if rule.SignalType == "SELL" || rule.SignalType == "STRONG_SELL" {
		signal.TargetPrice = ind.CurrentPrice * (1 - targetPercent/100)
		signal.StopLoss = ind.CurrentPrice * (1 + stopLossPercent/100)
	}

	// Store key indicators
	signal.Indicators["rsi"] = ind.RSI
	signal.Indicators["macd_hist"] = ind.MACDHist
	signal.Indicators["rs_avg"] = ind.RSAvg
	signal.Indicators["vol_ratio"] = ind.VolRatio
	signal.Indicators["ma50"] = ind.MA50
	signal.Indicators["ma200"] = ind.MA200

	return signal, nil
}

// EvaluateAllRules evaluates all active rules for a stock
func (e *ConditionEvaluator) EvaluateAllRules(ind *services.ExtendedStockIndicators) ([]*RuleSignal, error) {
	var rules []models.SignalRule
	if err := e.db.Where("is_active = ?", true).Order("priority DESC").Find(&rules).Error; err != nil {
		return nil, err
	}

	var signals []*RuleSignal
	for _, rule := range rules {
		signal, err := e.EvaluateRule(&rule, ind)
		if err == nil && signal != nil {
			signals = append(signals, signal)
		}
	}

	return signals, nil
}

// EvaluateTemplate evaluates a signal template against stock indicators
func (e *ConditionEvaluator) EvaluateTemplate(template *models.SignalTemplate, ind *services.ExtendedStockIndicators) (*RuleSignal, error) {
	var conditions []ConditionJSON
	if err := json.Unmarshal([]byte(template.Conditions), &conditions); err != nil {
		return nil, err
	}

	signal := &RuleSignal{
		StockCode:   ind.Code,
		Price:       ind.CurrentPrice,
		Reasons:     []string{},
		Indicators:  make(map[string]float64),
		GeneratedAt: time.Now(),
	}

	totalScore := 0
	maxScore := 0
	allRequiredPassed := true

	for _, cond := range conditions {
		condModel := &models.SignalCondition{
			Indicator:        models.IndicatorType(cond.Indicator),
			Operator:         models.ConditionOperator(cond.Operator),
			Value:            decimal.NewFromFloat(cond.Value),
			Value2:           decimal.NewFromFloat(cond.Value2),
			CompareIndicator: models.IndicatorType(cond.CompareIndicator),
			Weight:           cond.Weight,
			IsRequired:       cond.Required,
		}

		result := e.EvaluateCondition(condModel, ind)
		maxScore += cond.Weight

		if result.Passed {
			totalScore += result.Score
			if result.Message != "" {
				signal.Reasons = append(signal.Reasons, result.Message)
			}
		} else if cond.Required {
			allRequiredPassed = false
		}
	}

	signal.Score = totalScore
	signal.MaxScore = maxScore

	if maxScore > 0 {
		signal.Confidence = float64(totalScore) / float64(maxScore)
	}

	if !allRequiredPassed || signal.Confidence < 0.6 {
		return nil, nil
	}

	// Determine signal type based on template category
	switch template.Category {
	case "momentum", "breakout", "trend":
		if signal.Confidence >= 0.8 {
			signal.SignalType = "STRONG_BUY"
			signal.TargetPrice = ind.CurrentPrice * 1.15
			signal.StopLoss = ind.CurrentPrice * 0.95
		} else {
			signal.SignalType = "BUY"
			signal.TargetPrice = ind.CurrentPrice * 1.10
			signal.StopLoss = ind.CurrentPrice * 0.95
		}
	case "reversal":
		// For reversal, check if it's oversold or overbought
		if ind.RSI < 40 {
			signal.SignalType = "BUY"
			signal.TargetPrice = ind.MA50
			signal.StopLoss = ind.CurrentPrice * 0.93
		} else {
			signal.SignalType = "SELL"
			signal.TargetPrice = ind.MA50
			signal.StopLoss = ind.CurrentPrice * 1.07
		}
	default:
		signal.SignalType = "ALERT"
	}

	return signal, nil
}

// ScreenStocksWithRule screens all stocks with a specific rule
func (e *ConditionEvaluator) ScreenStocksWithRule(ruleID uint, minTradingVal float64, limit int) ([]*RuleSignal, error) {
	var rule models.SignalRule
	if err := e.db.First(&rule, ruleID).Error; err != nil {
		return nil, err
	}

	summary, err := services.GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		return nil, err
	}

	var signals []*RuleSignal
	var mu sync.Mutex
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, 10)

	for code, ind := range summary.Stocks {
		if ind == nil || ind.AvgTradingVal < minTradingVal {
			continue
		}

		wg.Add(1)
		go func(stockCode string, stockInd *services.ExtendedStockIndicators) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			signal, err := e.EvaluateRule(&rule, stockInd)
			if err == nil && signal != nil {
				mu.Lock()
				signals = append(signals, signal)
				mu.Unlock()
			}
		}(code, ind)
	}

	wg.Wait()

	// Sort by score descending
	sort.Slice(signals, func(i, j int) bool {
		return signals[i].Score > signals[j].Score
	})

	if limit > 0 && len(signals) > limit {
		signals = signals[:limit]
	}

	return signals, nil
}

// ScreenStocksWithTemplate screens all stocks with a template
func (e *ConditionEvaluator) ScreenStocksWithTemplate(templateID uint, minTradingVal float64, limit int) ([]*RuleSignal, error) {
	var template models.SignalTemplate
	if err := e.db.First(&template, templateID).Error; err != nil {
		return nil, err
	}

	summary, err := services.GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		return nil, err
	}

	var signals []*RuleSignal
	var mu sync.Mutex
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, 10)

	for code, ind := range summary.Stocks {
		if ind == nil || ind.AvgTradingVal < minTradingVal {
			continue
		}

		wg.Add(1)
		go func(stockCode string, stockInd *services.ExtendedStockIndicators) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			signal, err := e.EvaluateTemplate(&template, stockInd)
			if err == nil && signal != nil {
				mu.Lock()
				signals = append(signals, signal)
				mu.Unlock()
			}
		}(code, ind)
	}

	wg.Wait()

	// Sort by confidence descending
	sort.Slice(signals, func(i, j int) bool {
		return signals[i].Confidence > signals[j].Confidence
	})

	if limit > 0 && len(signals) > limit {
		signals = signals[:limit]
	}

	return signals, nil
}

// RecordSignalPerformance records a signal for performance tracking
func (e *ConditionEvaluator) RecordSignalPerformance(signal *RuleSignal) error {
	indicatorJSON, _ := json.Marshal(signal.Indicators)

	perf := &models.SignalPerformance{
		RuleID:            signal.Rule.ID,
		StockSymbol:       signal.StockCode,
		SignalDate:        signal.GeneratedAt,
		SignalType:        signal.SignalType,
		SignalScore:       signal.Score,
		EntryPrice:        decimal.NewFromFloat(signal.Price),
		TargetPrice:       decimal.NewFromFloat(signal.TargetPrice),
		StopLossPrice:     decimal.NewFromFloat(signal.StopLoss),
		IndicatorSnapshot: string(indicatorJSON),
	}

	return e.db.Create(perf).Error
}

// UpdateSignalPerformance updates signal performance with exit data
func (e *ConditionEvaluator) UpdateSignalPerformance(perfID uint, exitPrice float64, exitReason string) error {
	var perf models.SignalPerformance
	if err := e.db.First(&perf, perfID).Error; err != nil {
		return err
	}

	entryPrice := perf.EntryPrice.InexactFloat64()
	pnlPercent := (exitPrice - entryPrice) / entryPrice * 100

	if perf.SignalType == "SELL" || perf.SignalType == "STRONG_SELL" {
		pnlPercent = -pnlPercent // Reverse for sell signals
	}

	now := time.Now()
	holdingDays := int(now.Sub(perf.SignalDate).Hours() / 24)

	updates := map[string]interface{}{
		"exit_price":   decimal.NewFromFloat(exitPrice),
		"exit_date":    now,
		"exit_reason":  exitReason,
		"pnl_percent":  decimal.NewFromFloat(pnlPercent),
		"pnl_amount":   decimal.NewFromFloat((exitPrice - entryPrice) * 100), // Assuming 100 shares
		"holding_days": holdingDays,
		"is_win":       pnlPercent > 0,
	}

	return e.db.Model(&perf).Updates(updates).Error
}

// GetRuleStatistics returns performance statistics for a rule
func (e *ConditionEvaluator) GetRuleStatistics(ruleID uint) (map[string]interface{}, error) {
	var stats struct {
		TotalSignals  int64
		WinningTrades int64
		TotalPnL      float64
		AvgPnL        float64
		MaxGain       float64
		MaxLoss       float64
		AvgHoldDays   float64
	}

	err := e.db.Model(&models.SignalPerformance{}).
		Where("rule_id = ? AND exit_date IS NOT NULL", ruleID).
		Select(`
			COUNT(*) as total_signals,
			SUM(CASE WHEN is_win THEN 1 ELSE 0 END) as winning_trades,
			SUM(pnl_percent) as total_pnl,
			AVG(pnl_percent) as avg_pnl,
			MAX(pnl_percent) as max_gain,
			MIN(pnl_percent) as max_loss,
			AVG(holding_days) as avg_hold_days
		`).
		Scan(&stats).Error

	if err != nil {
		return nil, err
	}

	winRate := float64(0)
	if stats.TotalSignals > 0 {
		winRate = float64(stats.WinningTrades) / float64(stats.TotalSignals) * 100
	}

	return map[string]interface{}{
		"total_signals":    stats.TotalSignals,
		"winning_trades":   stats.WinningTrades,
		"losing_trades":    stats.TotalSignals - stats.WinningTrades,
		"win_rate":         winRate,
		"total_pnl":        stats.TotalPnL,
		"avg_pnl":          stats.AvgPnL,
		"max_gain":         stats.MaxGain,
		"max_loss":         stats.MaxLoss,
		"avg_holding_days": stats.AvgHoldDays,
	}, nil
}

// Helper function to build condition message
func buildConditionMessage(cond *models.SignalCondition, actual, target float64, passed bool) string {
	if !passed {
		return ""
	}

	indicator := string(cond.Indicator)
	operator := string(cond.Operator)

	switch cond.Operator {
	case models.OperatorGreaterThan, models.OperatorGreaterThanEqual:
		return indicator + " > " + formatFloat(target) + " (" + formatFloat(actual) + ")"
	case models.OperatorLessThan, models.OperatorLessThanEqual:
		return indicator + " < " + formatFloat(target) + " (" + formatFloat(actual) + ")"
	case models.OperatorBetween:
		return indicator + " in range (" + formatFloat(actual) + ")"
	case models.OperatorCrossAbove:
		return indicator + " crossed above " + string(cond.CompareIndicator)
	case models.OperatorCrossBelow:
		return indicator + " crossed below " + string(cond.CompareIndicator)
	default:
		return indicator + " " + operator + " " + formatFloat(target)
	}
}

func formatFloat(v float64) string {
	if v == float64(int(v)) {
		return decimal.NewFromFloat(v).StringFixed(0)
	}
	return decimal.NewFromFloat(v).StringFixed(2)
}
