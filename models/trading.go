package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TradingStrategy represents a trading strategy configuration
type TradingStrategy struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex" json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // trend_following, mean_reversion, breakout, etc.
	Parameters  string    `gorm:"type:jsonb" json:"parameters"` // JSON configuration
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Trade represents a trade execution
type Trade struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index" json:"user_id"`
	StockID     uint      `gorm:"index" json:"stock_id"`
	Stock       Stock     `gorm:"foreignKey:StockID" json:"stock,omitempty"`
	StrategyID  uint      `json:"strategy_id,omitempty"`
	Strategy    *TradingStrategy `gorm:"foreignKey:StrategyID" json:"strategy,omitempty"`
	Type        string    `json:"type"` // BUY, SELL
	Quantity    int64     `json:"quantity"`
	Price       decimal.Decimal `gorm:"type:decimal(15,2)" json:"price"`
	Commission  decimal.Decimal `gorm:"type:decimal(15,2)" json:"commission"`
	Tax         decimal.Decimal `gorm:"type:decimal(15,2)" json:"tax"`
	TotalAmount decimal.Decimal `gorm:"type:decimal(20,2)" json:"total_amount"`
	Status      string    `json:"status"` // pending, executed, cancelled, failed
	OrderType   string    `json:"order_type"` // market, limit, stop_loss
	ExecutedAt  *time.Time `json:"executed_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Portfolio represents user's stock holdings
type Portfolio struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index:idx_user_stock,unique" json:"user_id"`
	StockID     uint      `gorm:"index:idx_user_stock,unique" json:"stock_id"`
	Stock       Stock     `gorm:"foreignKey:StockID" json:"stock,omitempty"`
	Quantity    int64     `json:"quantity"`
	AvgPrice    decimal.Decimal `gorm:"type:decimal(15,2)" json:"avg_price"`
	CurrentPrice decimal.Decimal `gorm:"type:decimal(15,2)" json:"current_price"`
	TotalCost   decimal.Decimal `gorm:"type:decimal(20,2)" json:"total_cost"`
	MarketValue decimal.Decimal `gorm:"type:decimal(20,2)" json:"market_value"`
	UnrealizedPnL decimal.Decimal `gorm:"type:decimal(20,2)" json:"unrealized_pnl"`
	UnrealizedPnLPercent decimal.Decimal `gorm:"type:decimal(10,4)" json:"unrealized_pnl_percent"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Backtest represents a backtest run
type Backtest struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `json:"name"`
	StrategyID  uint      `json:"strategy_id"`
	Strategy    TradingStrategy `gorm:"foreignKey:StrategyID" json:"strategy,omitempty"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date"`
	InitialCapital decimal.Decimal `gorm:"type:decimal(20,2)" json:"initial_capital"`
	FinalCapital decimal.Decimal `gorm:"type:decimal(20,2)" json:"final_capital"`
	TotalReturn decimal.Decimal `gorm:"type:decimal(15,4)" json:"total_return"`
	AnnualReturn decimal.Decimal `gorm:"type:decimal(15,4)" json:"annual_return"`
	MaxDrawdown decimal.Decimal `gorm:"type:decimal(15,4)" json:"max_drawdown"`
	SharpeRatio decimal.Decimal `gorm:"type:decimal(10,4)" json:"sharpe_ratio"`
	TotalTrades int       `json:"total_trades"`
	WinningTrades int     `json:"winning_trades"`
	LosingTrades int      `json:"losing_trades"`
	WinRate     decimal.Decimal `gorm:"type:decimal(10,4)" json:"win_rate"`
	AvgWin      decimal.Decimal `gorm:"type:decimal(15,2)" json:"avg_win"`
	AvgLoss     decimal.Decimal `gorm:"type:decimal(15,2)" json:"avg_loss"`
	ProfitFactor decimal.Decimal `gorm:"type:decimal(10,4)" json:"profit_factor"`
	Results     string    `gorm:"type:jsonb" json:"results"` // Detailed results in JSON
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// BacktestTrade represents individual trades in a backtest
type BacktestTrade struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BacktestID uint      `gorm:"index" json:"backtest_id"`
	Backtest   Backtest  `gorm:"foreignKey:BacktestID" json:"backtest,omitempty"`
	StockID    uint      `json:"stock_id"`
	Stock      Stock     `gorm:"foreignKey:StockID" json:"stock,omitempty"`
	Type       string    `json:"type"` // BUY, SELL
	Date       time.Time `json:"date"`
	Quantity   int64     `json:"quantity"`
	Price      decimal.Decimal `gorm:"type:decimal(15,2)" json:"price"`
	Commission decimal.Decimal `gorm:"type:decimal(15,2)" json:"commission"`
	PnL        decimal.Decimal `gorm:"type:decimal(15,2)" json:"pnl"`
	Signal     string    `json:"signal"` // What triggered this trade
	CreatedAt  time.Time `json:"created_at"`
}

// Signal represents trading signals generated by strategies
type Signal struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	StockID    uint      `gorm:"index" json:"stock_id"`
	Stock      Stock     `gorm:"foreignKey:StockID" json:"stock,omitempty"`
	StrategyID uint      `json:"strategy_id"`
	Strategy   TradingStrategy `gorm:"foreignKey:StrategyID" json:"strategy,omitempty"`
	Type       string    `json:"type"` // BUY, SELL, HOLD
	Strength   decimal.Decimal `gorm:"type:decimal(5,2)" json:"strength"` // Signal strength 0-100
	Price      decimal.Decimal `gorm:"type:decimal(15,2)" json:"price"`
	TargetPrice decimal.Decimal `gorm:"type:decimal(15,2)" json:"target_price"`
	StopLoss   decimal.Decimal `gorm:"type:decimal(15,2)" json:"stop_loss"`
	Confidence decimal.Decimal `gorm:"type:decimal(5,2)" json:"confidence"` // 0-100
	Reason     string    `json:"reason"`
	IsActive   bool      `json:"is_active"`
	ExecutedAt *time.Time `json:"executed_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// MigrateTradingModels runs database migrations for trading-related models
func MigrateTradingModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&TradingStrategy{},
		&Trade{},
		&Portfolio{},
		&Backtest{},
		&BacktestTrade{},
		&Signal{},
	)
}
