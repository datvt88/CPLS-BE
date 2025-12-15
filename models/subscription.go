package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// SubscriptionPlan represents available subscription plans
type SubscriptionPlan struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"uniqueIndex;not null" json:"name"` // Free, Basic, Premium, Pro
	Description  string    `json:"description"`
	Price        decimal.Decimal `gorm:"type:decimal(15,2)" json:"price"`
	Currency     string    `gorm:"default:'VND'" json:"currency"`
	BillingCycle string    `json:"billing_cycle"` // monthly, yearly
	Features     string    `gorm:"type:jsonb" json:"features"` // JSON array of features
	MaxWatchlist int       `gorm:"default:10" json:"max_watchlist"`
	MaxAlerts    int       `gorm:"default:5" json:"max_alerts"`
	MaxStrategies int      `gorm:"default:3" json:"max_strategies"`
	HasBacktesting bool    `gorm:"default:false" json:"has_backtesting"`
	HasAutoTrading bool    `gorm:"default:false" json:"has_auto_trading"`
	HasAdvancedIndicators bool `gorm:"default:false" json:"has_advanced_indicators"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Subscription represents user subscription
type Subscription struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"uniqueIndex" json:"user_id"`
	User           User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	PlanID         uint      `gorm:"index" json:"plan_id"`
	Plan           SubscriptionPlan `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
	Status         string    `json:"status"` // active, cancelled, expired, pending
	StartDate      time.Time `json:"start_date"`
	EndDate        time.Time `json:"end_date"`
	AutoRenew      bool      `gorm:"default:true" json:"auto_renew"`
	PaymentMethod  string    `json:"payment_method"` // card, bank_transfer, momo, zalopay
	LastPaymentAt  *time.Time `json:"last_payment_at"`
	NextPaymentAt  *time.Time `json:"next_payment_at"`
	CancelledAt    *time.Time `json:"cancelled_at"`
	CancelReason   string    `json:"cancel_reason"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// PaymentHistory represents payment records
type PaymentHistory struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"index" json:"user_id"`
	User           User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	SubscriptionID uint      `gorm:"index" json:"subscription_id"`
	Subscription   Subscription `gorm:"foreignKey:SubscriptionID" json:"subscription,omitempty"`
	Amount         decimal.Decimal `gorm:"type:decimal(15,2)" json:"amount"`
	Currency       string    `gorm:"default:'VND'" json:"currency"`
	PaymentMethod  string    `json:"payment_method"`
	TransactionID  string    `gorm:"uniqueIndex" json:"transaction_id"`
	Status         string    `json:"status"` // pending, completed, failed, refunded
	Description    string    `json:"description"`
	Metadata       string    `gorm:"type:jsonb" json:"metadata"` // Payment gateway response
	ProcessedAt    *time.Time `json:"processed_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// Currency constants
const (
	CurrencyVND = "VND"
	CurrencyUSD = "USD"
)

// ValidCurrencies returns valid currency codes
func ValidCurrencies() []string {
	return []string{CurrencyVND, CurrencyUSD}
}

// IsValidCurrency checks if the currency is valid
func IsValidCurrency(currency string) bool {
	for _, valid := range ValidCurrencies() {
		if currency == valid {
			return true
		}
	}
	return false
}

// MigrateSubscriptionModels runs database migrations for subscription-related models
func MigrateSubscriptionModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&SubscriptionPlan{},
		&Subscription{},
		&PaymentHistory{},
	)
}