package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Stock represents a Vietnamese stock symbol
type Stock struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Symbol      string    `gorm:"uniqueIndex;not null" json:"symbol"`
	Name        string    `json:"name"`
	Exchange    string    `json:"exchange"` // HOSE, HNX, UPCOM
	Industry    string    `json:"industry"`
	Sector      string    `json:"sector"`
	MarketCap   decimal.Decimal `gorm:"type:decimal(20,2)" json:"market_cap"`
	ListingDate *time.Time `json:"listing_date"`
	Status      string    `json:"status"` // active, delisted, suspended
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StockPrice represents historical and real-time price data
type StockPrice struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	StockID     uint      `gorm:"index:idx_stock_date" json:"stock_id"`
	Stock       Stock     `gorm:"foreignKey:StockID" json:"stock,omitempty"`
	Date        time.Time `gorm:"index:idx_stock_date" json:"date"`
	Open        decimal.Decimal `gorm:"type:decimal(15,2)" json:"open"`
	High        decimal.Decimal `gorm:"type:decimal(15,2)" json:"high"`
	Low         decimal.Decimal `gorm:"type:decimal(15,2)" json:"low"`
	Close       decimal.Decimal `gorm:"type:decimal(15,2)" json:"close"`
	Volume      int64     `json:"volume"`
	Value       decimal.Decimal `gorm:"type:decimal(20,2)" json:"value"`
	AdjClose    decimal.Decimal `gorm:"type:decimal(15,2)" json:"adj_close"`
	Change      decimal.Decimal `gorm:"type:decimal(15,2)" json:"change"`
	ChangePercent decimal.Decimal `gorm:"type:decimal(10,4)" json:"change_percent"`
	CreatedAt   time.Time `json:"created_at"`
}

// TechnicalIndicator stores calculated technical indicators
type TechnicalIndicator struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	StockID   uint      `gorm:"index:idx_stock_date_type" json:"stock_id"`
	Stock     Stock     `gorm:"foreignKey:StockID" json:"stock,omitempty"`
	Date      time.Time `gorm:"index:idx_stock_date_type" json:"date"`
	Type      string    `gorm:"index:idx_stock_date_type" json:"type"` // SMA, EMA, RSI, MACD, etc.
	Period    int       `json:"period"` // e.g., 20 for SMA20
	Value     decimal.Decimal `gorm:"type:decimal(15,6)" json:"value"`
	Signal    decimal.Decimal `gorm:"type:decimal(15,6)" json:"signal"` // For MACD signal line
	Histogram decimal.Decimal `gorm:"type:decimal(15,6)" json:"histogram"` // For MACD histogram
	CreatedAt time.Time `json:"created_at"`
}

// MarketIndex represents market indices (VN-Index, HNX-Index, etc.)
type MarketIndex struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"uniqueIndex" json:"name"` // VN-Index, HNX-Index, UPCOM-Index
	Code          string    `gorm:"uniqueIndex" json:"code"`
	Date          time.Time `gorm:"index" json:"date"`
	Open          decimal.Decimal `gorm:"type:decimal(15,2)" json:"open"`
	High          decimal.Decimal `gorm:"type:decimal(15,2)" json:"high"`
	Low           decimal.Decimal `gorm:"type:decimal(15,2)" json:"low"`
	Close         decimal.Decimal `gorm:"type:decimal(15,2)" json:"close"`
	Volume        int64     `json:"volume"`
	Value         decimal.Decimal `gorm:"type:decimal(20,2)" json:"value"`
	Change        decimal.Decimal `gorm:"type:decimal(15,2)" json:"change"`
	ChangePercent decimal.Decimal `gorm:"type:decimal(10,4)" json:"change_percent"`
	CreatedAt     time.Time `json:"created_at"`
}

// AutoMigrate runs database migrations for stock-related models
func MigrateStockModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&Stock{},
		&StockPrice{},
		&TechnicalIndicator{},
		&MarketIndex{},
	)
}
