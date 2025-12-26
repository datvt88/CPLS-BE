package controllers

import (
	"net/http"
	"strconv"

	"go_backend_project/services/signals"

	"github.com/gin-gonic/gin"
)

// SignalController handles trading signal endpoints
type SignalController struct{}

// NewSignalController creates a new signal controller
func NewSignalController() *SignalController {
	return &SignalController{}
}

// GetStrategies returns all available trading strategies
// GET /api/v1/signals/strategies
func (ctrl *SignalController) GetStrategies(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Signal service not initialized"})
		return
	}

	strategies := signals.GlobalSignalService.GetStrategies()
	c.JSON(http.StatusOK, gin.H{
		"strategies": strategies,
		"count":      len(strategies),
	})
}

// GetSignal generates a trading signal for a specific stock
// GET /api/v1/signals/:code
func (ctrl *SignalController) GetSignal(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Signal service not initialized"})
		return
	}

	code := c.Param("code")
	strategy := c.DefaultQuery("strategy", "composite")

	signal, err := signals.GlobalSignalService.GenerateSignal(code, strategy)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	signal.Code = code
	c.JSON(http.StatusOK, signal)
}

// GetAllSignals generates signals for all stocks with filtering
// GET /api/v1/signals
func (ctrl *SignalController) GetAllSignals(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Signal service not initialized"})
		return
	}

	// Parse query parameters
	strategy := c.DefaultQuery("strategy", "composite")
	minStrength, _ := strconv.Atoi(c.DefaultQuery("min_strength", "0"))
	minConfidence, _ := strconv.ParseFloat(c.DefaultQuery("min_confidence", "0"), 64)
	minTradingVal, _ := strconv.ParseFloat(c.DefaultQuery("min_trading_val", "1"), 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	signalType := c.Query("signal_type") // BUY, SELL, STRONG_BUY, STRONG_SELL, HOLD

	filter := &signals.SignalFilter{
		MinStrength:   minStrength,
		MinConfidence: minConfidence,
		MinTradingVal: minTradingVal,
		Limit:         limit,
	}

	// Parse signal types
	if signalType != "" {
		filter.SignalTypes = []signals.SignalType{signals.SignalType(signalType)}
	}

	signalList, err := signals.GlobalSignalService.GenerateAllSignals(strategy, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count":    len(signalList),
		"signals":  signalList,
		"strategy": strategy,
		"filter": gin.H{
			"min_strength":    minStrength,
			"min_confidence":  minConfidence,
			"min_trading_val": minTradingVal,
			"signal_type":     signalType,
			"limit":           limit,
		},
	})
}

// GetBuySignals returns all buy signals
// GET /api/v1/signals/buy
func (ctrl *SignalController) GetBuySignals(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Signal service not initialized"})
		return
	}

	minStrength, _ := strconv.Atoi(c.DefaultQuery("min_strength", "60"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	signalList, err := signals.GlobalSignalService.GetBuySignals(minStrength, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count":   len(signalList),
		"signals": signalList,
		"filter": gin.H{
			"min_strength": minStrength,
			"limit":        limit,
		},
	})
}

// GetSellSignals returns all sell signals
// GET /api/v1/signals/sell
func (ctrl *SignalController) GetSellSignals(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Signal service not initialized"})
		return
	}

	minStrength, _ := strconv.Atoi(c.DefaultQuery("min_strength", "60"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	signalList, err := signals.GlobalSignalService.GetSellSignals(minStrength, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count":   len(signalList),
		"signals": signalList,
		"filter": gin.H{
			"min_strength": minStrength,
			"limit":        limit,
		},
	})
}

// GetTopSignals returns top trading opportunities across all signal types
// GET /api/v1/signals/top
func (ctrl *SignalController) GetTopSignals(c *gin.Context) {
	if signals.GlobalSignalService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Signal service not initialized"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	// Get strong buy signals
	buyFilter := &signals.SignalFilter{
		MinStrength:   70,
		SignalTypes:   []signals.SignalType{signals.SignalStrongBuy, signals.SignalBuy},
		MinTradingVal: 1.0,
		Limit:         limit,
	}
	buySignals, _ := signals.GlobalSignalService.GenerateAllSignals("composite", buyFilter)

	// Get strong sell signals
	sellFilter := &signals.SignalFilter{
		MinStrength:   70,
		SignalTypes:   []signals.SignalType{signals.SignalStrongSell, signals.SignalSell},
		MinTradingVal: 1.0,
		Limit:         limit,
	}
	sellSignals, _ := signals.GlobalSignalService.GenerateAllSignals("composite", sellFilter)

	c.JSON(http.StatusOK, gin.H{
		"top_buy_signals":  buySignals,
		"top_sell_signals": sellSignals,
		"buy_count":        len(buySignals),
		"sell_count":       len(sellSignals),
	})
}

// RegisterSignalRoutes registers all signal routes
func RegisterSignalRoutes(router *gin.RouterGroup) {
	ctrl := NewSignalController()

	signalGroup := router.Group("/signals")
	{
		signalGroup.GET("/strategies", ctrl.GetStrategies)
		signalGroup.GET("/buy", ctrl.GetBuySignals)
		signalGroup.GET("/sell", ctrl.GetSellSignals)
		signalGroup.GET("/top", ctrl.GetTopSignals)
		signalGroup.GET("/:code", ctrl.GetSignal)
		signalGroup.GET("", ctrl.GetAllSignals)
	}
}
