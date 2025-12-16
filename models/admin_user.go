package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AdminUser represents an administrator account for the admin panel
type AdminUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	Email        string    `gorm:"uniqueIndex" json:"email"`
	FullName     string    `json:"full_name"`
	Role         string    `gorm:"default:'admin'" json:"role"` // admin, superadmin
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SetPassword hashes and sets the password for the admin user
func (u *AdminUser) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword verifies the provided password against the stored hash
func (u *AdminUser) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}

// AdminSession represents an active admin session
type AdminSession struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	AdminUserID uint      `gorm:"index" json:"admin_user_id"`
	AdminUser   AdminUser `gorm:"foreignKey:AdminUserID" json:"admin_user,omitempty"`
	Token       string    `gorm:"uniqueIndex;not null" json:"token"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// IsExpired checks if the session has expired
func (s *AdminSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// MigrateAdminModels runs database migrations for admin-related models
func MigrateAdminModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&AdminUser{},
		&AdminSession{},
	)
}

// SeedDefaultAdminUser creates the default admin user if it doesn't exist
func SeedDefaultAdminUser(db *gorm.DB) error {
	var count int64
	db.Model(&AdminUser{}).Count(&count)
	if count > 0 {
		// Admin user already exists
		return nil
	}

	// Create default admin user
	admin := &AdminUser{
		Username: "datvt8x",
		Email:    "admin@cpls.com",
		FullName: "Administrator",
		Role:     "superadmin",
		IsActive: true,
	}
	if err := admin.SetPassword("@abcd4321"); err != nil {
		return err
	}

	return db.Create(admin).Error
}
