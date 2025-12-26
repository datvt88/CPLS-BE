package signals

import (
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"go_backend_project/services"
)

// SignalType represents the type of trading signal
type SignalType string

const (
	SignalBuy       SignalType = "BUY"
	SignalSell      SignalType = "SELL"
	SignalHold      SignalType = "HOLD"
	SignalStrongBuy SignalType = "STRONG_BUY"
	SignalStrongSell SignalType = "STRONG_SELL"
)

// SignalStrength represents how strong a signal is (0-100)
type SignalStrength int

const (
	StrengthWeak   SignalStrength = 25
	StrengthMedium SignalStrength = 50
	StrengthStrong SignalStrength = 75
	StrengthVeryStrong SignalStrength = 100
)

// TradingSignal represents a trading signal for a stock
type TradingSignal struct {
	Code           string          `json:"code"`
	Signal         SignalType      `json:"signal"`
	Strength       int             `json:"strength"`        // 0-100
	Confidence     float64         `json:"confidence"`      // 0-1
	Price          float64         `json:"price"`
	TargetPrice    float64         `json:"target_price,omitempty"`
	StopLoss       float64         `json:"stop_loss,omitempty"`
	Reasons        []string        `json:"reasons"`
	Indicators     *SignalIndicators `json:"indicators"`
	Strategy       string          `json:"strategy"`
	GeneratedAt    string          `json:"generated_at"`
}

// SignalIndicators contains the indicator values used to generate the signal
type SignalIndicators struct {
	RSAvg          float64 `json:"rs_avg"`
	RS3D           float64 `json:"rs_3d"`
	RS1M           float64 `json:"rs_1m"`
	RS3M           float64 `json:"rs_3m"`
	RS1Y           float64 `json:"rs_1y"`
	MACD           float64 `json:"macd"`
	MACDSignal     float64 `json:"macd_signal"`
	MACDHist       float64 `json:"macd_hist"`
	RSI            float64 `json:"rsi"`
	MA10           float64 `json:"ma_10"`
	MA30           float64 `json:"ma_30"`
	MA50           float64 `json:"ma_50"`
	MA200          float64 `json:"ma_200"`
	VolRatio       float64 `json:"vol_ratio"`
	AvgTradingVal  float64 `json:"avg_trading_val"`
}

// SignalFilter defines criteria for filtering signals
type SignalFilter struct {
	MinStrength    int        `json:"min_strength"`
	MinConfidence  float64    `json:"min_confidence"`
	SignalTypes    []SignalType `json:"signal_types"`
	MinTradingVal  float64    `json:"min_trading_val"`
	Strategies     []string   `json:"strategies"`
	Limit          int        `json:"limit"`
}

// Strategy defines a trading strategy
type Strategy interface {
	Name() string
	Description() string
	Evaluate(ind *services.ExtendedStockIndicators) (*TradingSignal, error)
}

// SignalService manages trading signals
type SignalService struct {
	mu         sync.RWMutex
	strategies map[string]Strategy
	cache      map[string]*TradingSignal
	cacheTime  time.Time
	cacheTTL   time.Duration
}

// Global signal service instance
var GlobalSignalService *SignalService

// InitSignalService initializes the signal service
func InitSignalService() error {
	GlobalSignalService = &SignalService{
		strategies: make(map[string]Strategy),
		cache:      make(map[string]*TradingSignal),
		cacheTTL:   5 * time.Minute,
	}

	// Register built-in strategies
	GlobalSignalService.RegisterStrategy(&MomentumStrategy{})
	GlobalSignalService.RegisterStrategy(&TrendFollowingStrategy{})
	GlobalSignalService.RegisterStrategy(&MeanReversionStrategy{})
	GlobalSignalService.RegisterStrategy(&BreakoutStrategy{})
	GlobalSignalService.RegisterStrategy(&CompositeStrategy{})

	log.Println("Signal Service initialized with", len(GlobalSignalService.strategies), "strategies")
	return nil
}

// RegisterStrategy registers a new strategy
func (s *SignalService) RegisterStrategy(strategy Strategy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strategies[strategy.Name()] = strategy
}

// GetStrategies returns all registered strategies
func (s *SignalService) GetStrategies() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.strategies))
	for name := range s.strategies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GenerateSignal generates a signal for a single stock using specified strategy
func (s *SignalService) GenerateSignal(code string, strategyName string) (*TradingSignal, error) {
	s.mu.RLock()
	strategy, ok := s.strategies[strategyName]
	s.mu.RUnlock()

	if !ok {
		// Use composite strategy as default
		strategy = &CompositeStrategy{}
	}

	// Get indicators for the stock
	indicators, err := services.GlobalIndicatorService.GetStockIndicators(code)
	if err != nil {
		return nil, err
	}

	return strategy.Evaluate(indicators)
}

// GenerateAllSignals generates signals for all stocks
func (s *SignalService) GenerateAllSignals(strategyName string, filter *SignalFilter) ([]*TradingSignal, error) {
	s.mu.RLock()
	strategy, ok := s.strategies[strategyName]
	s.mu.RUnlock()

	if !ok {
		strategy = &CompositeStrategy{}
	}

	// Load indicator summary
	summary, err := services.GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		return nil, err
	}

	var signals []*TradingSignal
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Process stocks concurrently
	semaphore := make(chan struct{}, 10) // Limit concurrency

	for code, ind := range summary.Stocks {
		if ind == nil {
			continue
		}

		// Apply trading value filter
		if filter != nil && filter.MinTradingVal > 0 && ind.AvgTradingVal < filter.MinTradingVal {
			continue
		}

		wg.Add(1)
		go func(stockCode string, indicators *services.ExtendedStockIndicators) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			signal, err := strategy.Evaluate(indicators)
			if err != nil {
				return
			}

			signal.Code = stockCode

			// Apply filters
			if filter != nil {
				if filter.MinStrength > 0 && signal.Strength < filter.MinStrength {
					return
				}
				if filter.MinConfidence > 0 && signal.Confidence < filter.MinConfidence {
					return
				}
				if len(filter.SignalTypes) > 0 {
					found := false
					for _, st := range filter.SignalTypes {
						if signal.Signal == st {
							found = true
							break
						}
					}
					if !found {
						return
					}
				}
			}

			mu.Lock()
			signals = append(signals, signal)
			mu.Unlock()
		}(code, ind)
	}

	wg.Wait()

	// Sort by strength descending
	sort.Slice(signals, func(i, j int) bool {
		return signals[i].Strength > signals[j].Strength
	})

	// Apply limit
	if filter != nil && filter.Limit > 0 && len(signals) > filter.Limit {
		signals = signals[:filter.Limit]
	}

	return signals, nil
}

// GetBuySignals returns all BUY and STRONG_BUY signals
func (s *SignalService) GetBuySignals(minStrength int, limit int) ([]*TradingSignal, error) {
	filter := &SignalFilter{
		MinStrength:   minStrength,
		SignalTypes:   []SignalType{SignalBuy, SignalStrongBuy},
		MinTradingVal: services.MinTradingValForRS, // Only large cap stocks
		Limit:         limit,
	}
	return s.GenerateAllSignals("composite", filter)
}

// GetSellSignals returns all SELL and STRONG_SELL signals
func (s *SignalService) GetSellSignals(minStrength int, limit int) ([]*TradingSignal, error) {
	filter := &SignalFilter{
		MinStrength:   minStrength,
		SignalTypes:   []SignalType{SignalSell, SignalStrongSell},
		MinTradingVal: services.MinTradingValForRS,
		Limit:         limit,
	}
	return s.GenerateAllSignals("composite", filter)
}

// =============================================================================
// MOMENTUM STRATEGY
// Based on Relative Strength (RS) rankings
// =============================================================================

type MomentumStrategy struct{}

func (s *MomentumStrategy) Name() string { return "momentum" }
func (s *MomentumStrategy) Description() string {
	return "Momentum strategy based on Relative Strength rankings across multiple timeframes"
}

func (s *MomentumStrategy) Evaluate(ind *services.ExtendedStockIndicators) (*TradingSignal, error) {
	signal := &TradingSignal{
		Price:       ind.CurrentPrice,
		Strategy:    s.Name(),
		GeneratedAt: time.Now().Format(time.RFC3339),
		Reasons:     []string{},
		Indicators: &SignalIndicators{
			RSAvg:     ind.RSAvg,
			RS3D:      ind.RS3DRank,
			RS1M:      ind.RS1MRank,
			RS3M:      ind.RS3MRank,
			RS1Y:      ind.RS1YRank,
			MACDHist:  ind.MACDHist,
			RSI:       ind.RSI,
			VolRatio:  ind.VolRatio,
		},
	}

	score := 0.0
	maxScore := 0.0

	// RS Average score (weight: 30)
	maxScore += 30
	if ind.RSAvg >= 80 {
		score += 30
		signal.Reasons = append(signal.Reasons, "RS Avg >= 80 (Strong momentum)")
	} else if ind.RSAvg >= 60 {
		score += 20
		signal.Reasons = append(signal.Reasons, "RS Avg >= 60 (Good momentum)")
	} else if ind.RSAvg >= 40 {
		score += 10
	} else if ind.RSAvg < 20 {
		score -= 10
		signal.Reasons = append(signal.Reasons, "RS Avg < 20 (Weak momentum)")
	}

	// RS 1Y (long-term trend) score (weight: 25)
	maxScore += 25
	if ind.RS1YRank >= 80 {
		score += 25
		signal.Reasons = append(signal.Reasons, "RS 1Y >= 80 (Strong yearly performance)")
	} else if ind.RS1YRank >= 60 {
		score += 15
	} else if ind.RS1YRank < 30 {
		score -= 10
	}

	// RS 3M (medium-term) score (weight: 20)
	maxScore += 20
	if ind.RS3MRank >= 70 {
		score += 20
		signal.Reasons = append(signal.Reasons, "RS 3M >= 70 (Strong quarterly momentum)")
	} else if ind.RS3MRank >= 50 {
		score += 10
	} else if ind.RS3MRank < 30 {
		score -= 5
	}

	// RS 3D (short-term) score (weight: 15)
	maxScore += 15
	if ind.RS3DRank >= 80 {
		score += 15
		signal.Reasons = append(signal.Reasons, "RS 3D >= 80 (Recent strength)")
	} else if ind.RS3DRank >= 60 {
		score += 10
	}

	// Volume confirmation (weight: 10)
	maxScore += 10
	if ind.VolRatio >= 1.5 {
		score += 10
		signal.Reasons = append(signal.Reasons, "Volume 1.5x above average")
	} else if ind.VolRatio >= 1.0 {
		score += 5
	}

	// Calculate normalized strength (0-100)
	strength := int(math.Max(0, math.Min(100, (score/maxScore)*100)))
	signal.Strength = strength
	signal.Confidence = score / maxScore

	// Determine signal type
	if strength >= 80 {
		signal.Signal = SignalStrongBuy
		signal.TargetPrice = ind.CurrentPrice * 1.15 // 15% target
		signal.StopLoss = ind.CurrentPrice * 0.95    // 5% stop loss
	} else if strength >= 60 {
		signal.Signal = SignalBuy
		signal.TargetPrice = ind.CurrentPrice * 1.10
		signal.StopLoss = ind.CurrentPrice * 0.95
	} else if strength <= 20 {
		signal.Signal = SignalStrongSell
	} else if strength <= 40 {
		signal.Signal = SignalSell
	} else {
		signal.Signal = SignalHold
	}

	return signal, nil
}

// =============================================================================
// TREND FOLLOWING STRATEGY
// Based on Moving Average crossovers and MACD
// =============================================================================

type TrendFollowingStrategy struct{}

func (s *TrendFollowingStrategy) Name() string { return "trend_following" }
func (s *TrendFollowingStrategy) Description() string {
	return "Trend following strategy using MA crossovers, MACD, and price-MA relationship"
}

func (s *TrendFollowingStrategy) Evaluate(ind *services.ExtendedStockIndicators) (*TradingSignal, error) {
	signal := &TradingSignal{
		Price:       ind.CurrentPrice,
		Strategy:    s.Name(),
		GeneratedAt: time.Now().Format(time.RFC3339),
		Reasons:     []string{},
		Indicators: &SignalIndicators{
			MA10:       ind.MA10,
			MA30:       ind.MA30,
			MA50:       ind.MA50,
			MA200:      ind.MA200,
			MACD:       ind.MACD,
			MACDSignal: ind.MACDSignal,
			MACDHist:   ind.MACDHist,
		},
	}

	score := 0.0
	maxScore := 0.0

	// MA10 > MA30 (short-term trend) - weight: 20
	maxScore += 20
	if ind.MA10AboveMA30 {
		score += 20
		signal.Reasons = append(signal.Reasons, "MA10 > MA30 (Short-term uptrend)")
	} else {
		signal.Reasons = append(signal.Reasons, "MA10 < MA30 (Short-term downtrend)")
	}

	// MA50 > MA200 (Golden Cross / Death Cross) - weight: 25
	maxScore += 25
	if ind.MA50AboveMA200 {
		score += 25
		signal.Reasons = append(signal.Reasons, "MA50 > MA200 (Golden Cross - Bullish)")
	} else {
		signal.Reasons = append(signal.Reasons, "MA50 < MA200 (Death Cross - Bearish)")
	}

	// Price above MA50 - weight: 20
	maxScore += 20
	if ind.CurrentPrice > ind.MA50 && ind.MA50 > 0 {
		score += 20
		signal.Reasons = append(signal.Reasons, "Price above MA50")
	}

	// Price above MA200 - weight: 15
	maxScore += 15
	if ind.CurrentPrice > ind.MA200 && ind.MA200 > 0 {
		score += 15
		signal.Reasons = append(signal.Reasons, "Price above MA200 (Long-term uptrend)")
	}

	// MACD Histogram positive - weight: 20
	maxScore += 20
	if ind.MACDHist > 0 {
		score += 20
		signal.Reasons = append(signal.Reasons, "MACD Histogram positive (Bullish momentum)")
	} else if ind.MACDHist < -0.5 {
		signal.Reasons = append(signal.Reasons, "MACD Histogram negative (Bearish momentum)")
	}

	// Calculate strength
	strength := int(math.Max(0, math.Min(100, (score/maxScore)*100)))
	signal.Strength = strength
	signal.Confidence = score / maxScore

	// Determine signal
	if strength >= 80 {
		signal.Signal = SignalStrongBuy
		signal.TargetPrice = ind.CurrentPrice * 1.12
		signal.StopLoss = ind.MA50 * 0.98 // Below MA50
	} else if strength >= 60 {
		signal.Signal = SignalBuy
		signal.TargetPrice = ind.CurrentPrice * 1.08
		signal.StopLoss = ind.MA50 * 0.98
	} else if strength <= 20 {
		signal.Signal = SignalStrongSell
	} else if strength <= 40 {
		signal.Signal = SignalSell
	} else {
		signal.Signal = SignalHold
	}

	return signal, nil
}

// =============================================================================
// MEAN REVERSION STRATEGY
// Based on RSI and price deviation from moving averages
// =============================================================================

type MeanReversionStrategy struct{}

func (s *MeanReversionStrategy) Name() string { return "mean_reversion" }
func (s *MeanReversionStrategy) Description() string {
	return "Mean reversion strategy using RSI oversold/overbought conditions"
}

func (s *MeanReversionStrategy) Evaluate(ind *services.ExtendedStockIndicators) (*TradingSignal, error) {
	signal := &TradingSignal{
		Price:       ind.CurrentPrice,
		Strategy:    s.Name(),
		GeneratedAt: time.Now().Format(time.RFC3339),
		Reasons:     []string{},
		Indicators: &SignalIndicators{
			RSI:    ind.RSI,
			MA50:   ind.MA50,
			MA200:  ind.MA200,
		},
	}

	// Calculate price deviation from MA50
	var ma50Deviation float64
	if ind.MA50 > 0 {
		ma50Deviation = (ind.CurrentPrice - ind.MA50) / ind.MA50 * 100
	}

	score := 50.0 // Start neutral
	maxScore := 100.0

	// RSI conditions
	if ind.RSI < 30 {
		score += 30
		signal.Reasons = append(signal.Reasons, "RSI < 30 (Oversold - Potential bounce)")
	} else if ind.RSI < 40 {
		score += 15
		signal.Reasons = append(signal.Reasons, "RSI < 40 (Approaching oversold)")
	} else if ind.RSI > 70 {
		score -= 30
		signal.Reasons = append(signal.Reasons, "RSI > 70 (Overbought - Potential pullback)")
	} else if ind.RSI > 60 {
		score -= 15
		signal.Reasons = append(signal.Reasons, "RSI > 60 (Approaching overbought)")
	}

	// Price deviation from MA50
	if ma50Deviation < -10 {
		score += 20
		signal.Reasons = append(signal.Reasons, "Price >10% below MA50 (Potential reversion)")
	} else if ma50Deviation < -5 {
		score += 10
	} else if ma50Deviation > 10 {
		score -= 20
		signal.Reasons = append(signal.Reasons, "Price >10% above MA50 (Extended)")
	} else if ma50Deviation > 5 {
		score -= 10
	}

	// Long-term trend confirmation (for mean reversion, we prefer healthy trends)
	if ind.MA50AboveMA200 && ind.RSI < 40 {
		score += 10
		signal.Reasons = append(signal.Reasons, "Oversold in uptrend (High probability bounce)")
	}

	// Calculate strength
	strength := int(math.Max(0, math.Min(100, score)))
	signal.Strength = strength
	signal.Confidence = score / maxScore

	// Determine signal (inverted logic for mean reversion)
	if ind.RSI < 30 && ma50Deviation < -5 {
		signal.Signal = SignalStrongBuy
		signal.TargetPrice = ind.MA50 // Target return to MA50
		signal.StopLoss = ind.CurrentPrice * 0.93
	} else if ind.RSI < 40 {
		signal.Signal = SignalBuy
		signal.TargetPrice = ind.MA50 * 0.98
		signal.StopLoss = ind.CurrentPrice * 0.95
	} else if ind.RSI > 70 && ma50Deviation > 5 {
		signal.Signal = SignalStrongSell
	} else if ind.RSI > 60 {
		signal.Signal = SignalSell
	} else {
		signal.Signal = SignalHold
	}

	return signal, nil
}

// =============================================================================
// BREAKOUT STRATEGY
// Based on volume spikes and RS momentum
// =============================================================================

type BreakoutStrategy struct{}

func (s *BreakoutStrategy) Name() string { return "breakout" }
func (s *BreakoutStrategy) Description() string {
	return "Breakout strategy detecting volume spikes with strong momentum"
}

func (s *BreakoutStrategy) Evaluate(ind *services.ExtendedStockIndicators) (*TradingSignal, error) {
	signal := &TradingSignal{
		Price:       ind.CurrentPrice,
		Strategy:    s.Name(),
		GeneratedAt: time.Now().Format(time.RFC3339),
		Reasons:     []string{},
		Indicators: &SignalIndicators{
			RS3D:     ind.RS3DRank,
			RSAvg:    ind.RSAvg,
			VolRatio: ind.VolRatio,
			MACDHist: ind.MACDHist,
		},
	}

	score := 0.0
	maxScore := 0.0

	// Volume breakout (weight: 35)
	maxScore += 35
	if ind.VolRatio >= 2.0 {
		score += 35
		signal.Reasons = append(signal.Reasons, "Volume 2x+ above average (Strong breakout)")
	} else if ind.VolRatio >= 1.5 {
		score += 25
		signal.Reasons = append(signal.Reasons, "Volume 1.5x above average")
	} else if ind.VolRatio >= 1.2 {
		score += 15
	}

	// Short-term momentum (RS 3D) - weight: 30
	maxScore += 30
	if ind.RS3DRank >= 90 {
		score += 30
		signal.Reasons = append(signal.Reasons, "RS 3D >= 90 (Explosive short-term momentum)")
	} else if ind.RS3DRank >= 80 {
		score += 25
		signal.Reasons = append(signal.Reasons, "RS 3D >= 80 (Strong short-term momentum)")
	} else if ind.RS3DRank >= 70 {
		score += 15
	}

	// Price above all major MAs - weight: 20
	maxScore += 20
	aboveAllMA := ind.CurrentPrice > ind.MA10 &&
	             ind.CurrentPrice > ind.MA30 &&
	             ind.CurrentPrice > ind.MA50
	if aboveAllMA {
		score += 20
		signal.Reasons = append(signal.Reasons, "Price above all major MAs")
	}

	// MACD confirmation - weight: 15
	maxScore += 15
	if ind.MACDHist > 0.5 {
		score += 15
		signal.Reasons = append(signal.Reasons, "MACD strongly bullish")
	} else if ind.MACDHist > 0 {
		score += 10
	}

	// Calculate strength
	strength := int(math.Max(0, math.Min(100, (score/maxScore)*100)))
	signal.Strength = strength
	signal.Confidence = score / maxScore

	// Determine signal - breakout strategy is aggressive
	if strength >= 75 && ind.VolRatio >= 1.5 {
		signal.Signal = SignalStrongBuy
		signal.TargetPrice = ind.CurrentPrice * 1.20 // 20% target for breakouts
		signal.StopLoss = ind.CurrentPrice * 0.92    // 8% stop loss
	} else if strength >= 60 {
		signal.Signal = SignalBuy
		signal.TargetPrice = ind.CurrentPrice * 1.12
		signal.StopLoss = ind.CurrentPrice * 0.95
	} else if strength <= 30 {
		signal.Signal = SignalSell
	} else {
		signal.Signal = SignalHold
	}

	return signal, nil
}

// =============================================================================
// COMPOSITE STRATEGY
// Combines all strategies with weighted scoring
// =============================================================================

type CompositeStrategy struct{}

func (s *CompositeStrategy) Name() string { return "composite" }
func (s *CompositeStrategy) Description() string {
	return "Composite strategy combining momentum, trend, mean reversion, and breakout signals"
}

func (s *CompositeStrategy) Evaluate(ind *services.ExtendedStockIndicators) (*TradingSignal, error) {
	// Evaluate all strategies
	momentum := &MomentumStrategy{}
	trend := &TrendFollowingStrategy{}
	meanRev := &MeanReversionStrategy{}
	breakout := &BreakoutStrategy{}

	momSignal, _ := momentum.Evaluate(ind)
	trendSignal, _ := trend.Evaluate(ind)
	meanRevSignal, _ := meanRev.Evaluate(ind)
	breakoutSignal, _ := breakout.Evaluate(ind)

	// Weighted average of strengths
	// Weights: Momentum 30%, Trend 35%, Mean Reversion 15%, Breakout 20%
	compositeStrength := int(
		float64(momSignal.Strength)*0.30 +
		float64(trendSignal.Strength)*0.35 +
		float64(meanRevSignal.Strength)*0.15 +
		float64(breakoutSignal.Strength)*0.20,
	)

	compositeConfidence := momSignal.Confidence*0.30 +
		trendSignal.Confidence*0.35 +
		meanRevSignal.Confidence*0.15 +
		breakoutSignal.Confidence*0.20

	signal := &TradingSignal{
		Price:       ind.CurrentPrice,
		Strategy:    s.Name(),
		Strength:    compositeStrength,
		Confidence:  compositeConfidence,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Reasons:     []string{},
		Indicators: &SignalIndicators{
			RSAvg:         ind.RSAvg,
			RS3D:          ind.RS3DRank,
			RS1M:          ind.RS1MRank,
			RS3M:          ind.RS3MRank,
			RS1Y:          ind.RS1YRank,
			MACD:          ind.MACD,
			MACDSignal:    ind.MACDSignal,
			MACDHist:      ind.MACDHist,
			RSI:           ind.RSI,
			MA10:          ind.MA10,
			MA30:          ind.MA30,
			MA50:          ind.MA50,
			MA200:         ind.MA200,
			VolRatio:      ind.VolRatio,
			AvgTradingVal: ind.AvgTradingVal,
		},
	}

	// Aggregate reasons from strongest signals
	if momSignal.Strength >= 60 && len(momSignal.Reasons) > 0 {
		signal.Reasons = append(signal.Reasons, "[Momentum] "+momSignal.Reasons[0])
	}
	if trendSignal.Strength >= 60 && len(trendSignal.Reasons) > 0 {
		signal.Reasons = append(signal.Reasons, "[Trend] "+trendSignal.Reasons[0])
	}
	if breakoutSignal.Strength >= 70 && len(breakoutSignal.Reasons) > 0 {
		signal.Reasons = append(signal.Reasons, "[Breakout] "+breakoutSignal.Reasons[0])
	}

	// Count buy/sell votes
	buyVotes := 0
	sellVotes := 0

	signals := []*TradingSignal{momSignal, trendSignal, meanRevSignal, breakoutSignal}
	for _, sig := range signals {
		switch sig.Signal {
		case SignalStrongBuy:
			buyVotes += 2
		case SignalBuy:
			buyVotes += 1
		case SignalStrongSell:
			sellVotes += 2
		case SignalSell:
			sellVotes += 1
		}
	}

	// Determine final signal based on votes and strength
	if compositeStrength >= 75 && buyVotes >= 4 {
		signal.Signal = SignalStrongBuy
		signal.TargetPrice = ind.CurrentPrice * 1.15
		signal.StopLoss = ind.CurrentPrice * 0.95
	} else if compositeStrength >= 60 && buyVotes >= 2 {
		signal.Signal = SignalBuy
		signal.TargetPrice = ind.CurrentPrice * 1.10
		signal.StopLoss = ind.CurrentPrice * 0.95
	} else if compositeStrength <= 25 && sellVotes >= 4 {
		signal.Signal = SignalStrongSell
	} else if compositeStrength <= 40 && sellVotes >= 2 {
		signal.Signal = SignalSell
	} else {
		signal.Signal = SignalHold
	}

	return signal, nil
}
