package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// User represents a user in the system with Supabase authentication integration
type User struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	SupabaseUserID  string    `gorm:"uniqueIndex;not null" json:"supabase_user_id"` // Supabase auth user ID
	Email           string    `gorm:"uniqueIndex;not null" json:"email"`
	FullName        string    `json:"full_name"`
	AvatarURL       string    `json:"avatar_url"`
	Phone           string    `json:"phone"`
	Role            string    `gorm:"default:'user'" json:"role"` // user, admin, premium
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	EmailVerified   bool      `gorm:"default:false" json:"email_verified"`
	Balance         decimal.Decimal `gorm:"type:decimal(20,2);default:0" json:"balance"`
	TotalDeposit    decimal.Decimal `gorm:"type:decimal(20,2);default:0" json:"total_deposit"`
	TotalWithdraw   decimal.Decimal `gorm:"type:decimal(20,2);default:0" json:"total_withdraw"`
	Preferences     string    `gorm:"type:jsonb" json:"preferences"` // JSON for user preferences
	LastLoginAt     *time.Time `json:"last_login_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UserSession represents user session for tracking
type UserSession struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index" json:"user_id"`
	User        User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	AccessToken string    `gorm:"not null" json:"-"`
	RefreshToken string   `json:"-"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// Watchlist represents user's stock watchlist
type Watchlist struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	StockID   uint      `gorm:"index" json:"stock_id"`
	Stock     Stock     `gorm:"foreignKey:StockID" json:"stock,omitempty"`
	Notes     string    `json:"notes"`
	AlertPrice decimal.Decimal `gorm:"type:decimal(15,2)" json:"alert_price"`
	AlertType  string    `json:"alert_type"` // above, below, both
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserAlert represents price alerts for users
type UserAlert struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"index" json:"user_id"`
	User         User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	StockID      uint      `gorm:"index" json:"stock_id"`
	Stock        Stock     `gorm:"foreignKey:StockID" json:"stock,omitempty"`
	AlertType    string    `json:"alert_type"` // price_above, price_below, percent_change, volume_spike
	TargetValue  decimal.Decimal `gorm:"type:decimal(15,4)" json:"target_value"`
	IsTriggered  bool      `gorm:"default:false" json:"is_triggered"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	TriggeredAt  *time.Time `json:"triggered_at"`
	NotifyEmail  bool      `gorm:"default:true" json:"notify_email"`
	NotifyPush   bool      `gorm:"default:false" json:"notify_push"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Alert type constants for watchlist
const (
	AlertTypeAbove = "above"
	AlertTypeBelow = "below"
	AlertTypeBoth  = "both"
)

// Alert type constants for user alerts
const (
	UserAlertTypePriceAbove    = "price_above"
	UserAlertTypePriceBelow    = "price_below"
	UserAlertTypePercentChange = "percent_change"
	UserAlertTypeVolumeSpike   = "volume_spike"
)

// ValidWatchlistAlertTypes returns valid alert types for watchlist
func ValidWatchlistAlertTypes() []string {
	return []string{AlertTypeAbove, AlertTypeBelow, AlertTypeBoth}
}

// ValidUserAlertTypes returns valid alert types for user alerts
func ValidUserAlertTypes() []string {
	return []string{
		UserAlertTypePriceAbove,
		UserAlertTypePriceBelow,
		UserAlertTypePercentChange,
		UserAlertTypeVolumeSpike,
	}
}

// IsValidWatchlistAlertType checks if the alert type is valid
func IsValidWatchlistAlertType(alertType string) bool {
	for _, valid := range ValidWatchlistAlertTypes() {
		if alertType == valid {
			return true
		}
	}
	return false
}

// IsValidUserAlertType checks if the alert type is valid
func IsValidUserAlertType(alertType string) bool {
	for _, valid := range ValidUserAlertTypes() {
		if alertType == valid {
			return true
		}
	}
	return false
}

// MigrateUserModels runs database migrations for user-related models
func MigrateUserModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&UserSession{},
		&Watchlist{},
		&UserAlert{},
	)
}