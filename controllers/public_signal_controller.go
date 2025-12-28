package controllers

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go_backend_project/services"
	"go_backend_project/services/signals"

	"github.com/gin-gonic/gin"
)

// PublicSignalController handles optimized public signal API endpoints
type PublicSignalController struct{}

// NewPublicSignalController creates a new public signal controller
func NewPublicSignalController() *PublicSignalController {
	return &PublicSignalController{}
}

// SignalResponse represents a standardized signal API response
type SignalResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data"`
	Meta      *MetaInfo   `json:"meta,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// MetaInfo contains pagination and metadata
type MetaInfo struct {
	Total       int    `json:"total"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
	TotalPages  int    `json:"total_pages"`
	Strategy    string `json:"strategy,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	CacheExpiry string `json:"cache_expiry,omitempty"`
}

// StockSignalSummary is an optimized signal summary for frontend
type StockSignalSummary struct {
	Code        string   `json:"code"`
	SignalType  string   `json:"signal_type"`
	Strength    int      `json:"strength"`
	Confidence  float64  `json:"confidence"`
	Price       float64  `json:"price"`
	PriceChange float64  `json:"price_change"`
	TargetPrice float64  `json:"target_price"`
	StopLoss    float64  `json:"stop_loss"`
	RSAvg       float64  `json:"rs_avg"`
	RSI         float64  `json:"rsi"`
	MACD        float64  `json:"macd"`
	AvgVol      float64  `json:"avg_vol"`
	Reasons     []string `json:"reasons"`
	Strategy    string   `json:"strategy"`
}

// SignalStats contains signal statistics
type SignalStats struct {
	TotalStocks int     `json:"total_stocks"`
	StrongBuy   int     `json:"strong_buy"`
	Buy         int     `json:"buy"`
	Hold        int     `json:"hold"`
	Sell        int     `json:"sell"`
	StrongSell  int     `json:"strong_sell"`
	AvgStrength float64 `json:"avg_strength"`
	UpdatedAt   string  `json:"updated_at"`
}

// RegisterPublicSignalRoutes registers optimized public signal routes
func (ctrl *PublicSignalController) RegisterPublicSignalRoutes(api *gin.RouterGroup) {
	signalRoutes := api.Group("/signals")
	{
		// Core signal endpoints
		signalRoutes.GET("", ctrl.GetSignals)
		signalRoutes.GET("/stock/:code", ctrl.GetStockSignal)
		signalRoutes.GET("/top", ctrl.GetTopSignals)
		signalRoutes.GET("/stats", ctrl.GetSignalStats)

		// Strategy endpoints
		signalRoutes.GET("/strategies", ctrl.GetStrategies)
		signalRoutes.GET("/strategy/:name", ctrl.GetStrategySignals)

		// Screener endpoints
		signalRoutes.GET("/screener/buy", ctrl.GetBuySignals)
		signalRoutes.GET("/screener/sell", ctrl.GetSellSignals)
		signalRoutes.GET("/screener/momentum", ctrl.GetMomentumStocks)
		signalRoutes.GET("/screener/oversold", ctrl.GetOversoldStocks)
		signalRoutes.GET("/screener/breakout", ctrl.GetBreakoutStocks)

		// Indicator-based endpoints
		signalRoutes.GET("/indicators/:code", ctrl.GetStockIndicators)
		signalRoutes.GET("/indicators", ctrl.GetAllIndicators)
	}
}

// GetSignals returns paginated signals with filtering
// GET /api/v1/signals?page=1&page_size=20&strategy=composite&signal_type=BUY&min_strength=60
func (ctrl *PublicSignalController) GetSignals(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Signal service not available")
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Parse filters
	strategy := c.DefaultQuery("strategy", "composite")
	signalType := c.Query("signal_type")
	minStrength, _ := strconv.Atoi(c.DefaultQuery("min_strength", "0"))
	minConfidence, _ := strconv.ParseFloat(c.DefaultQuery("min_confidence", "0"), 64)
	minTradingVal, _ := strconv.ParseFloat(c.DefaultQuery("min_trading_val", "1"), 64)

	// Build filter
	filter := &signals.SignalFilter{
		MinStrength:   minStrength,
		MinConfidence: minConfidence,
		MinTradingVal: minTradingVal,
	}

	if signalType != "" {
		filter.SignalTypes = []signals.SignalType{signals.SignalType(signalType)}
	}

	// Generate all signals
	allSignals, err := signals.GlobalSignalService.GenerateAllSignals(strategy, filter)
	if err != nil {
		ctrl.errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to summaries
	var filtered []StockSignalSummary
	for _, sig := range allSignals {
		summary := ctrl.convertToSummary(sig)
		filtered = append(filtered, summary)
	}

	// Sort by strength descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Strength > filtered[j].Strength
	})

	// Paginate
	total := len(filtered)
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginated := filtered[start:end]

	ctrl.successResponse(c, paginated, &MetaInfo{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		Strategy:   strategy,
		UpdatedAt:  time.Now().Format(time.RFC3339),
	})
}

// GetStockSignal returns signal for a specific stock
// GET /api/v1/signals/stock/VNM?strategy=composite
func (ctrl *PublicSignalController) GetStockSignal(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Signal service not available")
		return
	}

	code := strings.ToUpper(c.Param("code"))
	strategy := c.DefaultQuery("strategy", "composite")

	signal, err := signals.GlobalSignalService.GenerateSignal(code, strategy)
	if err != nil {
		ctrl.errorResponse(c, http.StatusNotFound, "Stock not found: "+code)
		return
	}

	// Get additional indicator data
	var indicators *services.ExtendedStockIndicators
	if services.GlobalIndicatorService != nil {
		indicators, _ = services.GlobalIndicatorService.GetStockIndicators(code)
	}

	response := gin.H{
		"code":         code,
		"signal":       signal,
		"indicators":   indicators,
		"strategy":     strategy,
		"generated_at": time.Now().Format(time.RFC3339),
	}

	ctrl.successResponse(c, response, nil)
}

// GetTopSignals returns top buy and sell signals
// GET /api/v1/signals/top?limit=10
func (ctrl *PublicSignalController) GetTopSignals(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Signal service not available")
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit < 1 || limit > 50 {
		limit = 10
	}
	minTradingVal, _ := strconv.ParseFloat(c.DefaultQuery("min_trading_val", "5"), 64)

	// Get buy signals
	buySignals, _ := signals.GlobalSignalService.GetBuySignals(50, limit*2)
	// Get sell signals
	sellSignals, _ := signals.GlobalSignalService.GetSellSignals(50, limit*2)

	var topBuy, topSell []StockSignalSummary

	for _, sig := range buySignals {
		if sig.Indicators != nil && sig.Indicators.AvgTradingVal >= minTradingVal {
			topBuy = append(topBuy, ctrl.convertToSummary(sig))
		}
	}

	for _, sig := range sellSignals {
		if sig.Indicators != nil && sig.Indicators.AvgTradingVal >= minTradingVal {
			topSell = append(topSell, ctrl.convertToSummary(sig))
		}
	}

	if len(topBuy) > limit {
		topBuy = topBuy[:limit]
	}
	if len(topSell) > limit {
		topSell = topSell[:limit]
	}

	ctrl.successResponse(c, gin.H{
		"top_buy":  topBuy,
		"top_sell": topSell,
	}, &MetaInfo{
		Total:     len(topBuy) + len(topSell),
		Strategy:  "composite",
		UpdatedAt: time.Now().Format(time.RFC3339),
	})
}

// GetSignalStats returns signal statistics
// GET /api/v1/signals/stats
func (ctrl *PublicSignalController) GetSignalStats(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Signal service not available")
		return
	}

	allSignals, err := signals.GlobalSignalService.GenerateAllSignals("composite", nil)
	if err != nil {
		ctrl.errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	stats := SignalStats{
		TotalStocks: len(allSignals),
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}

	var totalStrength int
	for _, sig := range allSignals {
		totalStrength += sig.Strength
		switch sig.Signal {
		case signals.SignalStrongBuy:
			stats.StrongBuy++
		case signals.SignalBuy:
			stats.Buy++
		case signals.SignalHold:
			stats.Hold++
		case signals.SignalSell:
			stats.Sell++
		case signals.SignalStrongSell:
			stats.StrongSell++
		}
	}

	if stats.TotalStocks > 0 {
		stats.AvgStrength = float64(totalStrength) / float64(stats.TotalStocks)
	}

	ctrl.successResponse(c, stats, nil)
}

// GetStrategies returns available strategies
// GET /api/v1/signals/strategies
func (ctrl *PublicSignalController) GetStrategies(c *gin.Context) {
	var strategyList []string
	if signals.GlobalSignalService != nil {
		strategyList = signals.GlobalSignalService.GetStrategies()
	}

	strategies := []gin.H{
		{
			"name":        "composite",
			"description": "Combines all strategies with weighted scoring",
			"weights": gin.H{
				"momentum":        0.30,
				"trend_following": 0.35,
				"mean_reversion":  0.15,
				"breakout":        0.20,
			},
			"recommended": true,
		},
		{
			"name":        "momentum",
			"description": "Focus on relative strength and momentum indicators",
			"indicators":  []string{"RS_AVG", "RS_1Y", "RS_3M", "RS_3D", "VOLUME"},
		},
		{
			"name":        "trend_following",
			"description": "Follow long-term trends using moving averages",
			"indicators":  []string{"MA10", "MA30", "MA50", "MA200", "MACD"},
		},
		{
			"name":        "mean_reversion",
			"description": "Find oversold/overbought stocks for reversal trades",
			"indicators":  []string{"RSI", "PRICE", "MA50"},
		},
		{
			"name":        "breakout",
			"description": "Identify volume breakouts with strong momentum",
			"indicators":  []string{"VOLUME", "RS_3D", "PRICE", "MACD"},
		},
	}

	ctrl.successResponse(c, gin.H{
		"strategies": strategies,
		"available":  strategyList,
	}, nil)
}

// GetStrategySignals returns signals for a specific strategy
// GET /api/v1/signals/strategy/momentum?limit=20
func (ctrl *PublicSignalController) GetStrategySignals(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Signal service not available")
		return
	}

	strategyName := c.Param("name")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	filter := &signals.SignalFilter{
		Limit: limit,
	}

	allSignals, err := signals.GlobalSignalService.GenerateAllSignals(strategyName, filter)
	if err != nil {
		ctrl.errorResponse(c, http.StatusBadRequest, "Invalid strategy: "+strategyName)
		return
	}

	var results []StockSignalSummary
	for _, sig := range allSignals {
		results = append(results, ctrl.convertToSummary(sig))
	}

	ctrl.successResponse(c, results, &MetaInfo{
		Total:    len(results),
		Strategy: strategyName,
	})
}

// GetBuySignals returns top buy signals
// GET /api/v1/signals/screener/buy?min_strength=70&min_trading_val=10&limit=20
func (ctrl *PublicSignalController) GetBuySignals(c *gin.Context) {
	ctrl.getFilteredSignals(c, []signals.SignalType{signals.SignalBuy, signals.SignalStrongBuy}, true)
}

// GetSellSignals returns top sell signals
// GET /api/v1/signals/screener/sell?min_strength=30&limit=20
func (ctrl *PublicSignalController) GetSellSignals(c *gin.Context) {
	ctrl.getFilteredSignals(c, []signals.SignalType{signals.SignalSell, signals.SignalStrongSell}, false)
}

// GetMomentumStocks returns stocks with high momentum
// GET /api/v1/signals/screener/momentum?min_rs=80&limit=20
func (ctrl *PublicSignalController) GetMomentumStocks(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Indicator service not available")
		return
	}

	minRS, _ := strconv.ParseFloat(c.DefaultQuery("min_rs", "80"), 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	summary, err := services.GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		ctrl.errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	var results []gin.H
	for code, ind := range summary.Stocks {
		if ind.RSAvg >= minRS {
			results = append(results, gin.H{
				"code":         code,
				"rs_avg":       ind.RSAvg,
				"rs_3d":        ind.RS3DRank,
				"rs_1m":        ind.RS1MRank,
				"rs_3m":        ind.RS3MRank,
				"rs_1y":        ind.RS1YRank,
				"price":        ind.CurrentPrice,
				"price_change": ind.PriceChange,
				"volume_ratio": ind.VolRatio,
			})
		}
	}

	// Sort by RS Avg
	sort.Slice(results, func(i, j int) bool {
		return results[i]["rs_avg"].(float64) > results[j]["rs_avg"].(float64)
	})

	if len(results) > limit {
		results = results[:limit]
	}

	ctrl.successResponse(c, results, &MetaInfo{
		Total:     len(results),
		UpdatedAt: summary.UpdatedAt,
	})
}

// GetOversoldStocks returns oversold stocks (RSI < 30)
// GET /api/v1/signals/screener/oversold?limit=20
func (ctrl *PublicSignalController) GetOversoldStocks(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Indicator service not available")
		return
	}

	maxRSI, _ := strconv.ParseFloat(c.DefaultQuery("max_rsi", "30"), 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	summary, err := services.GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		ctrl.errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	var results []gin.H
	for code, ind := range summary.Stocks {
		if ind.RSI <= maxRSI && ind.RSI > 0 {
			results = append(results, gin.H{
				"code":          code,
				"rsi":           ind.RSI,
				"price":         ind.CurrentPrice,
				"ma50":          ind.MA50,
				"price_vs_ma50": (ind.CurrentPrice - ind.MA50) / ind.MA50 * 100,
				"rs_avg":        ind.RSAvg,
			})
		}
	}

	// Sort by RSI ascending (most oversold first)
	sort.Slice(results, func(i, j int) bool {
		return results[i]["rsi"].(float64) < results[j]["rsi"].(float64)
	})

	if len(results) > limit {
		results = results[:limit]
	}

	ctrl.successResponse(c, results, &MetaInfo{
		Total:     len(results),
		UpdatedAt: summary.UpdatedAt,
	})
}

// GetBreakoutStocks returns stocks with volume breakout
// GET /api/v1/signals/screener/breakout?min_vol_ratio=2&limit=20
func (ctrl *PublicSignalController) GetBreakoutStocks(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Indicator service not available")
		return
	}

	minVolRatio, _ := strconv.ParseFloat(c.DefaultQuery("min_vol_ratio", "2"), 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	summary, err := services.GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		ctrl.errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	var results []gin.H
	for code, ind := range summary.Stocks {
		if ind.VolRatio >= minVolRatio && ind.RS3DRank >= 70 {
			results = append(results, gin.H{
				"code":         code,
				"vol_ratio":    ind.VolRatio,
				"rs_3d":        ind.RS3DRank,
				"price":        ind.CurrentPrice,
				"price_change": ind.PriceChange,
				"macd_hist":    ind.MACDHist,
				"above_ma10":   ind.CurrentPrice > ind.MA10,
				"above_ma50":   ind.CurrentPrice > ind.MA50,
			})
		}
	}

	// Sort by volume ratio
	sort.Slice(results, func(i, j int) bool {
		return results[i]["vol_ratio"].(float64) > results[j]["vol_ratio"].(float64)
	})

	if len(results) > limit {
		results = results[:limit]
	}

	ctrl.successResponse(c, results, &MetaInfo{
		Total:     len(results),
		UpdatedAt: summary.UpdatedAt,
	})
}

// GetStockIndicators returns all indicators for a stock
// GET /api/v1/signals/indicators/VNM
func (ctrl *PublicSignalController) GetStockIndicators(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Indicator service not available")
		return
	}

	code := strings.ToUpper(c.Param("code"))

	ind, err := services.GlobalIndicatorService.GetStockIndicators(code)
	if err != nil {
		ctrl.errorResponse(c, http.StatusNotFound, "Stock not found: "+code)
		return
	}

	ctrl.successResponse(c, ind, nil)
}

// GetAllIndicators returns paginated indicators for all stocks
// GET /api/v1/signals/indicators?page=1&page_size=50&sort_by=rs_avg
func (ctrl *PublicSignalController) GetAllIndicators(c *gin.Context) {
	if services.GlobalIndicatorService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Indicator service not available")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	sortBy := c.DefaultQuery("sort_by", "rs_avg")

	summary, err := services.GlobalIndicatorService.LoadIndicatorSummary()
	if err != nil {
		ctrl.errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to slice
	type stockInd struct {
		Code       string                            `json:"code"`
		Indicators *services.ExtendedStockIndicators `json:"indicators"`
	}

	var results []stockInd
	for code, ind := range summary.Stocks {
		results = append(results, stockInd{Code: code, Indicators: ind})
	}

	// Sort
	sort.Slice(results, func(i, j int) bool {
		switch sortBy {
		case "rs_avg":
			return results[i].Indicators.RSAvg > results[j].Indicators.RSAvg
		case "rs_1y":
			return results[i].Indicators.RS1YRank > results[j].Indicators.RS1YRank
		case "rsi":
			return results[i].Indicators.RSI > results[j].Indicators.RSI
		case "price":
			return results[i].Indicators.CurrentPrice > results[j].Indicators.CurrentPrice
		case "volume":
			return results[i].Indicators.AvgVol > results[j].Indicators.AvgVol
		default:
			return results[i].Indicators.RSAvg > results[j].Indicators.RSAvg
		}
	})

	// Paginate
	total := len(results)
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	ctrl.successResponse(c, results[start:end], &MetaInfo{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		UpdatedAt:  summary.UpdatedAt,
	})
}

// Helper methods

func (ctrl *PublicSignalController) getFilteredSignals(c *gin.Context, types []signals.SignalType, sortDesc bool) {
	if signals.GlobalSignalService == nil {
		ctrl.errorResponse(c, http.StatusServiceUnavailable, "Signal service not available")
		return
	}

	minStrength, _ := strconv.Atoi(c.DefaultQuery("min_strength", "0"))
	minTradingVal, _ := strconv.ParseFloat(c.DefaultQuery("min_trading_val", "1"), 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	filter := &signals.SignalFilter{
		MinStrength:   minStrength,
		MinTradingVal: minTradingVal,
		SignalTypes:   types,
		Limit:         limit * 2, // Get more to filter
	}

	allSignals, _ := signals.GlobalSignalService.GenerateAllSignals("composite", filter)

	var results []StockSignalSummary
	for _, sig := range allSignals {
		results = append(results, ctrl.convertToSummary(sig))
	}

	// Sort
	sort.Slice(results, func(i, j int) bool {
		if sortDesc {
			return results[i].Strength > results[j].Strength
		}
		return results[i].Strength < results[j].Strength
	})

	if len(results) > limit {
		results = results[:limit]
	}

	ctrl.successResponse(c, results, &MetaInfo{
		Total:    len(results),
		Strategy: "composite",
	})
}

func (ctrl *PublicSignalController) convertToSummary(sig *signals.TradingSignal) StockSignalSummary {
	summary := StockSignalSummary{
		Code:        sig.Code,
		SignalType:  string(sig.Signal),
		Strength:    sig.Strength,
		Confidence:  sig.Confidence,
		Price:       sig.Price,
		TargetPrice: sig.TargetPrice,
		StopLoss:    sig.StopLoss,
		Reasons:     sig.Reasons,
		Strategy:    sig.Strategy,
	}

	if sig.Indicators != nil {
		summary.RSAvg = sig.Indicators.RSAvg
		summary.RSI = sig.Indicators.RSI
		summary.MACD = sig.Indicators.MACDHist
		summary.AvgVol = sig.Indicators.AvgVol
		summary.PriceChange = sig.Indicators.PriceChange
	}

	return summary
}

func (ctrl *PublicSignalController) successResponse(c *gin.Context, data interface{}, meta *MetaInfo) {
	c.JSON(http.StatusOK, SignalResponse{
		Success:   true,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

func (ctrl *PublicSignalController) errorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, SignalResponse{
		Success:   false,
		Error:     message,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}
