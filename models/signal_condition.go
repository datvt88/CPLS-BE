package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ConditionOperator represents comparison operators for conditions
type ConditionOperator string

const (
	OperatorEqual            ConditionOperator = "eq"          // =
	OperatorNotEqual         ConditionOperator = "neq"         // !=
	OperatorGreaterThan      ConditionOperator = "gt"          // >
	OperatorGreaterThanEqual ConditionOperator = "gte"         // >=
	OperatorLessThan         ConditionOperator = "lt"          // <
	OperatorLessThanEqual    ConditionOperator = "lte"         // <=
	OperatorBetween          ConditionOperator = "between"     // between min and max
	OperatorCrossAbove       ConditionOperator = "cross_above" // crosses above
	OperatorCrossBelow       ConditionOperator = "cross_below" // crosses below
)

// String returns the string representation of ConditionOperator
func (c ConditionOperator) String() string {
	return string(c)
}

// IndicatorType represents the type of technical indicator
type IndicatorType string

const (
	IndicatorRSI           IndicatorType = "RSI"
	IndicatorMACD          IndicatorType = "MACD"
	IndicatorMACDSignal    IndicatorType = "MACD_SIGNAL"
	IndicatorMACDHistogram IndicatorType = "MACD_HISTOGRAM"
	IndicatorMA10          IndicatorType = "MA10"
	IndicatorMA30          IndicatorType = "MA30"
	IndicatorMA50          IndicatorType = "MA50"
	IndicatorMA200         IndicatorType = "MA200"
	IndicatorRS3D          IndicatorType = "RS_3D"
	IndicatorRS1M          IndicatorType = "RS_1M"
	IndicatorRS3M          IndicatorType = "RS_3M"
	IndicatorRS1Y          IndicatorType = "RS_1Y"
	IndicatorRSAvg         IndicatorType = "RS_AVG"
	IndicatorVolume        IndicatorType = "VOLUME"
	IndicatorVolRatio      IndicatorType = "VOL_RATIO"
	IndicatorPrice         IndicatorType = "PRICE"
	IndicatorPriceChange   IndicatorType = "PRICE_CHANGE"
	IndicatorTradingValue  IndicatorType = "TRADING_VALUE"
)

// String returns the string representation of IndicatorType
func (i IndicatorType) String() string {
	return string(i)
}

// LogicalOperator for combining conditions
type LogicalOperator string

const (
	LogicalAnd LogicalOperator = "AND"
	LogicalOr  LogicalOperator = "OR"
)

// String returns the string representation of LogicalOperator
func (l LogicalOperator) String() string {
	return string(l)
}

// SignalConditionGroup represents a group of conditions that can be reused
type SignalConditionGroup struct {
	ID          uint              `gorm:"primaryKey" json:"id"`
	Name        string            `gorm:"uniqueIndex;not null" json:"name"`
	Description string            `json:"description"`
	SignalType  string            `json:"signal_type"` // BUY, SELL, HOLD, ALERT
	IsActive    bool              `gorm:"default:true" json:"is_active"`
	Priority    int               `gorm:"default:0" json:"priority"` // Higher priority = evaluated first
	Conditions  []SignalCondition `gorm:"foreignKey:GroupID" json:"conditions,omitempty"`
	CreatedBy   uint              `json:"created_by"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// SignalCondition represents a single condition rule
type SignalCondition struct {
	ID               uint              `gorm:"primaryKey" json:"id"`
	GroupID          uint              `gorm:"index" json:"group_id"`
	Name             string            `json:"name"`
	Indicator        IndicatorType     `gorm:"type:varchar(50);not null" json:"indicator"`
	Operator         ConditionOperator `gorm:"type:varchar(20);not null" json:"operator"`
	Value            decimal.Decimal   `gorm:"type:decimal(15,4)" json:"value"`
	Value2           decimal.Decimal   `gorm:"type:decimal(15,4)" json:"value2"`          // For BETWEEN operator
	CompareIndicator IndicatorType     `gorm:"type:varchar(50)" json:"compare_indicator"` // For comparing two indicators
	LogicalOperator  LogicalOperator   `gorm:"type:varchar(10);default:'AND'" json:"logical_operator"`
	Weight           int               `gorm:"default:1" json:"weight"`          // Weight for scoring
	IsRequired       bool              `gorm:"default:false" json:"is_required"` // Must be true for signal
	Description      string            `json:"description"`
	OrderIndex       int               `gorm:"default:0" json:"order_index"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// SignalRule represents a complete trading rule that combines condition groups
type SignalRule struct {
	ID              uint            `gorm:"primaryKey" json:"id"`
	Name            string          `gorm:"uniqueIndex;not null" json:"name"`
	Description     string          `json:"description"`
	SignalType      string          `gorm:"not null" json:"signal_type"` // BUY, SELL, ALERT
	StrategyType    string          `json:"strategy_type"`               // momentum, trend, etc.
	MinScore        int             `gorm:"default:60" json:"min_score"` // Minimum score to trigger
	TargetPercent   decimal.Decimal `gorm:"type:decimal(5,2);default:10" json:"target_percent"`
	StopLossPercent decimal.Decimal `gorm:"type:decimal(5,2);default:5" json:"stop_loss_percent"`
	IsActive        bool            `gorm:"default:true" json:"is_active"`
	Priority        int             `gorm:"default:0" json:"priority"`
	ConditionGroups string          `gorm:"type:jsonb" json:"condition_groups"` // JSON array of group IDs with logic
	// Backtest Performance
	BacktestWinRate     decimal.Decimal `gorm:"type:decimal(5,2)" json:"backtest_win_rate"`
	BacktestAvgReturn   decimal.Decimal `gorm:"type:decimal(10,4)" json:"backtest_avg_return"`
	BacktestTotalTrades int             `json:"backtest_total_trades"`
	LastBacktestAt      *time.Time      `json:"last_backtest_at"`
	CreatedBy           uint            `json:"created_by"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// SignalAlert represents an alert configuration
type SignalAlert struct {
	ID              uint        `gorm:"primaryKey" json:"id"`
	Name            string      `gorm:"not null" json:"name"`
	RuleID          uint        `gorm:"index" json:"rule_id"`
	Rule            *SignalRule `gorm:"foreignKey:RuleID" json:"rule,omitempty"`
	StockSymbol     string      `gorm:"type:varchar(20)" json:"stock_symbol"` // Empty = all stocks
	AlertType       string      `gorm:"not null" json:"alert_type"`           // email, push, webhook
	WebhookURL      string      `json:"webhook_url,omitempty"`
	IsActive        bool        `gorm:"default:true" json:"is_active"`
	Cooldown        int         `gorm:"default:60" json:"cooldown"` // Minutes between alerts
	LastTriggeredAt *time.Time  `json:"last_triggered_at"`
	TriggerCount    int         `gorm:"default:0" json:"trigger_count"`
	CreatedBy       uint        `json:"created_by"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// SignalAlertHistory stores triggered alert history
type SignalAlertHistory struct {
	ID          uint            `gorm:"primaryKey" json:"id"`
	AlertID     uint            `gorm:"index" json:"alert_id"`
	Alert       *SignalAlert    `gorm:"foreignKey:AlertID" json:"alert,omitempty"`
	StockSymbol string          `gorm:"type:varchar(20);not null" json:"stock_symbol"`
	SignalType  string          `json:"signal_type"`
	Score       int             `json:"score"`
	Price       decimal.Decimal `gorm:"type:decimal(15,2)" json:"price"`
	Message     string          `json:"message"`
	Metadata    string          `gorm:"type:jsonb" json:"metadata"` // Additional signal data
	DeliveredAt *time.Time      `json:"delivered_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

// SignalPerformance tracks the performance of generated signals
type SignalPerformance struct {
	ID                uint            `gorm:"primaryKey" json:"id"`
	RuleID            uint            `gorm:"index" json:"rule_id"`
	Rule              *SignalRule     `gorm:"foreignKey:RuleID" json:"rule,omitempty"`
	StockSymbol       string          `gorm:"type:varchar(20);index:idx_perf_stock_date" json:"stock_symbol"`
	SignalDate        time.Time       `gorm:"index:idx_perf_stock_date" json:"signal_date"`
	SignalType        string          `json:"signal_type"`
	SignalScore       int             `json:"signal_score"`
	EntryPrice        decimal.Decimal `gorm:"type:decimal(15,2)" json:"entry_price"`
	TargetPrice       decimal.Decimal `gorm:"type:decimal(15,2)" json:"target_price"`
	StopLossPrice     decimal.Decimal `gorm:"type:decimal(15,2)" json:"stop_loss_price"`
	ExitPrice         decimal.Decimal `gorm:"type:decimal(15,2)" json:"exit_price"`
	ExitDate          *time.Time      `json:"exit_date"`
	ExitReason        string          `json:"exit_reason"` // target_hit, stop_loss, timeout, manual
	PnLPercent        decimal.Decimal `gorm:"type:decimal(10,4)" json:"pnl_percent"`
	PnLAmount         decimal.Decimal `gorm:"type:decimal(15,2)" json:"pnl_amount"`
	HoldingDays       int             `json:"holding_days"`
	IsWin             bool            `json:"is_win"`
	MaxDrawdown       decimal.Decimal `gorm:"type:decimal(10,4)" json:"max_drawdown"`
	MaxGain           decimal.Decimal `gorm:"type:decimal(10,4)" json:"max_gain"`
	IndicatorSnapshot string          `gorm:"type:jsonb" json:"indicator_snapshot"` // Indicators at signal time
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// SignalTemplate provides preset condition templates
type SignalTemplate struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex;not null" json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"`                              // momentum, trend, reversal, breakout, custom
	Conditions  string    `gorm:"type:jsonb;not null" json:"conditions"` // JSON template
	Popularity  int       `gorm:"default:0" json:"popularity"`
	IsBuiltIn   bool      `gorm:"default:false" json:"is_built_in"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BuiltInTemplates returns built-in signal templates
func BuiltInTemplates() []SignalTemplate {
	return []SignalTemplate{
		{
			Name:        "RSI Oversold Bounce",
			Description: "Buy when RSI < 30 and starts recovering",
			Category:    "reversal",
			IsBuiltIn:   true,
			Conditions: `[
				{"indicator": "RSI", "operator": "lt", "value": 30, "weight": 30, "required": true},
				{"indicator": "MA50", "operator": "gt", "compare_indicator": "MA200", "weight": 20},
				{"indicator": "VOL_RATIO", "operator": "gte", "value": 1.2, "weight": 15}
			]`,
		},
		{
			Name:        "Golden Cross",
			Description: "MA50 crosses above MA200 with volume confirmation",
			Category:    "trend",
			IsBuiltIn:   true,
			Conditions: `[
				{"indicator": "MA50", "operator": "cross_above", "compare_indicator": "MA200", "weight": 40, "required": true},
				{"indicator": "MACD_HISTOGRAM", "operator": "gt", "value": 0, "weight": 20},
				{"indicator": "VOL_RATIO", "operator": "gte", "value": 1.5, "weight": 20}
			]`,
		},
		{
			Name:        "Momentum Leader",
			Description: "Strong relative strength across all timeframes",
			Category:    "momentum",
			IsBuiltIn:   true,
			Conditions: `[
				{"indicator": "RS_AVG", "operator": "gte", "value": 80, "weight": 30, "required": true},
				{"indicator": "RS_3D", "operator": "gte", "value": 70, "weight": 20},
				{"indicator": "TRADING_VALUE", "operator": "gte", "value": 1, "weight": 10}
			]`,
		},
		{
			Name:        "Volume Breakout",
			Description: "Price breakout with significant volume spike",
			Category:    "breakout",
			IsBuiltIn:   true,
			Conditions: `[
				{"indicator": "VOL_RATIO", "operator": "gte", "value": 2, "weight": 35, "required": true},
				{"indicator": "RS_3D", "operator": "gte", "value": 85, "weight": 25},
				{"indicator": "PRICE", "operator": "gt", "compare_indicator": "MA10", "weight": 15},
				{"indicator": "MACD_HISTOGRAM", "operator": "gt", "value": 0, "weight": 15}
			]`,
		},
		{
			Name:        "Death Cross Warning",
			Description: "MA50 crosses below MA200 - bearish signal",
			Category:    "trend",
			IsBuiltIn:   true,
			Conditions: `[
				{"indicator": "MA50", "operator": "cross_below", "compare_indicator": "MA200", "weight": 40, "required": true},
				{"indicator": "MACD_HISTOGRAM", "operator": "lt", "value": 0, "weight": 20},
				{"indicator": "RSI", "operator": "lt", "value": 50, "weight": 15}
			]`,
		},
		{
			Name:        "Overbought Reversal",
			Description: "RSI overbought with potential pullback",
			Category:    "reversal",
			IsBuiltIn:   true,
			Conditions: `[
				{"indicator": "RSI", "operator": "gt", "value": 70, "weight": 30, "required": true},
				{"indicator": "PRICE_CHANGE", "operator": "gt", "value": 5, "weight": 20},
				{"indicator": "RS_3D", "operator": "gte", "value": 90, "weight": 15}
			]`,
		},
		{
			Name:        "Trend Continuation",
			Description: "Strong trend with healthy pullback",
			Category:    "trend",
			IsBuiltIn:   true,
			Conditions: `[
				{"indicator": "MA50", "operator": "gt", "compare_indicator": "MA200", "weight": 20, "required": true},
				{"indicator": "PRICE", "operator": "between", "value": 0.95, "value2": 1.05, "compare_indicator": "MA50", "weight": 25},
				{"indicator": "RSI", "operator": "between", "value": 40, "value2": 60, "weight": 20},
				{"indicator": "MACD_HISTOGRAM", "operator": "gt", "value": 0, "weight": 15}
			]`,
		},
		{
			Name:        "Value + Momentum",
			Description: "Undervalued with improving momentum",
			Category:    "custom",
			IsBuiltIn:   true,
			Conditions: `[
				{"indicator": "RS_1Y", "operator": "lt", "value": 50, "weight": 20},
				{"indicator": "RS_1M", "operator": "gte", "value": 60, "weight": 25},
				{"indicator": "RS_3D", "operator": "gte", "value": 70, "weight": 25},
				{"indicator": "VOL_RATIO", "operator": "gte", "value": 1.3, "weight": 15}
			]`,
		},
	}
}

// MigrateSignalConditionModels runs database migrations for signal condition models
func MigrateSignalConditionModels(db *gorm.DB) error {
	err := db.AutoMigrate(
		&SignalConditionGroup{},
		&SignalCondition{},
		&SignalRule{},
		&SignalAlert{},
		&SignalAlertHistory{},
		&SignalPerformance{},
		&SignalTemplate{},
	)
	if err != nil {
		return err
	}

	// Seed built-in templates
	for _, template := range BuiltInTemplates() {
		var existing SignalTemplate
		if db.Where("name = ?", template.Name).First(&existing).Error == gorm.ErrRecordNotFound {
			db.Create(&template)
		}
	}

	return nil
}
