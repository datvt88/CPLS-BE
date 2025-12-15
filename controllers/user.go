package controllers

import (
	"net/http"
	"strconv"
	"time"

	"go_backend_project/models"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// UserController handles user-related requests
type UserController struct {
	db *gorm.DB
}

// NewUserController creates a new user controller
func NewUserController(db *gorm.DB) *UserController {
	return &UserController{db: db}
}

// GetUsers returns list of all users with pagination
// GET /api/v1/users
func (uc *UserController) GetUsers(c *gin.Context) {
	var users []models.User

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	uc.db.Model(&models.User{}).Count(&total)

	query := uc.db.Model(&models.User{})

	// Filter by role
	if role := c.Query("role"); role != "" {
		query = query.Where("role = ?", role)
	}

	// Filter by active status
	if isActive := c.Query("is_active"); isActive != "" {
		query = query.Where("is_active = ?", isActive == "true")
	}

	// Search by email or name
	if search := c.Query("search"); search != "" {
		query = query.Where("email ILIKE ? OR full_name ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if err := query.Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": users,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetUser returns a single user by ID or Supabase ID
// GET /api/v1/users/:id
func (uc *UserController) GetUser(c *gin.Context) {
	id := c.Param("id")

	var user models.User
	if err := uc.db.Where("id = ? OR supabase_user_id = ?", id, id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// CreateUser creates a new user (called after Supabase auth)
// POST /api/v1/users
func (uc *UserController) CreateUser(c *gin.Context) {
	var request struct {
		SupabaseUserID string `json:"supabase_user_id" binding:"required"`
		Email          string `json:"email" binding:"required,email"`
		FullName       string `json:"full_name"`
		AvatarURL      string `json:"avatar_url"`
		Phone          string `json:"phone"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if user already exists
	var existing models.User
	if err := uc.db.Where("supabase_user_id = ? OR email = ?", request.SupabaseUserID, request.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
		return
	}

	user := models.User{
		SupabaseUserID: request.SupabaseUserID,
		Email:          request.Email,
		FullName:       request.FullName,
		AvatarURL:      request.AvatarURL,
		Phone:          request.Phone,
		Role:           "user",
		IsActive:       true,
		Balance:        decimal.Zero,
	}

	if err := uc.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": user})
}

// UpdateUser updates user information
// PUT /api/v1/users/:id
func (uc *UserController) UpdateUser(c *gin.Context) {
	id := c.Param("id")

	var user models.User
	if err := uc.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var request struct {
		FullName    string `json:"full_name"`
		AvatarURL   string `json:"avatar_url"`
		Phone       string `json:"phone"`
		Preferences string `json:"preferences"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if request.FullName != "" {
		updates["full_name"] = request.FullName
	}
	if request.AvatarURL != "" {
		updates["avatar_url"] = request.AvatarURL
	}
	if request.Phone != "" {
		updates["phone"] = request.Phone
	}
	if request.Preferences != "" {
		updates["preferences"] = request.Preferences
	}

	if err := uc.db.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// DeleteUser soft deletes a user (deactivates)
// DELETE /api/v1/users/:id
func (uc *UserController) DeleteUser(c *gin.Context) {
	id := c.Param("id")

	if err := uc.db.Model(&models.User{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to deactivate user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deactivated successfully"})
}

// GetUserWatchlist returns user's stock watchlist
// GET /api/v1/users/:id/watchlist
func (uc *UserController) GetUserWatchlist(c *gin.Context) {
	userID := c.Param("id")

	var watchlist []models.Watchlist
	if err := uc.db.Where("user_id = ?", userID).Preload("Stock").Find(&watchlist).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch watchlist"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": watchlist})
}

// AddToWatchlist adds a stock to user's watchlist
// POST /api/v1/users/:id/watchlist
func (uc *UserController) AddToWatchlist(c *gin.Context) {
	userID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var request struct {
		StockID    uint    `json:"stock_id" binding:"required"`
		Notes      string  `json:"notes"`
		AlertPrice float64 `json:"alert_price"`
		AlertType  string  `json:"alert_type"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if already in watchlist
	var existing models.Watchlist
	if err := uc.db.Where("user_id = ? AND stock_id = ?", userID, request.StockID).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Stock already in watchlist"})
		return
	}

	watchlist := models.Watchlist{
		UserID:     uint(userID),
		StockID:    request.StockID,
		Notes:      request.Notes,
		AlertPrice: decimal.NewFromFloat(request.AlertPrice),
		AlertType:  request.AlertType,
	}

	if err := uc.db.Create(&watchlist).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add to watchlist"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": watchlist})
}

// RemoveFromWatchlist removes a stock from user's watchlist
// DELETE /api/v1/users/:id/watchlist/:stock_id
func (uc *UserController) RemoveFromWatchlist(c *gin.Context) {
	userID := c.Param("id")
	stockID := c.Param("stock_id")

	if err := uc.db.Where("user_id = ? AND stock_id = ?", userID, stockID).Delete(&models.Watchlist{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove from watchlist"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Removed from watchlist"})
}

// GetUserAlerts returns user's price alerts
// GET /api/v1/users/:id/alerts
func (uc *UserController) GetUserAlerts(c *gin.Context) {
	userID := c.Param("id")

	var alerts []models.UserAlert
	query := uc.db.Where("user_id = ?", userID)

	if isActive := c.Query("is_active"); isActive != "" {
		query = query.Where("is_active = ?", isActive == "true")
	}

	if err := query.Preload("Stock").Find(&alerts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch alerts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": alerts})
}

// CreateUserAlert creates a price alert for user
// POST /api/v1/users/:id/alerts
func (uc *UserController) CreateUserAlert(c *gin.Context) {
	userID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var request struct {
		StockID     uint    `json:"stock_id" binding:"required"`
		AlertType   string  `json:"alert_type" binding:"required"`
		TargetValue float64 `json:"target_value" binding:"required"`
		NotifyEmail bool    `json:"notify_email"`
		NotifyPush  bool    `json:"notify_push"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate alert type
	if !models.IsValidUserAlertType(request.AlertType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "Invalid alert type",
			"valid_alert_types": models.ValidUserAlertTypes(),
		})
		return
	}

	alert := models.UserAlert{
		UserID:      uint(userID),
		StockID:     request.StockID,
		AlertType:   request.AlertType,
		TargetValue: decimal.NewFromFloat(request.TargetValue),
		IsActive:    true,
		NotifyEmail: request.NotifyEmail,
		NotifyPush:  request.NotifyPush,
	}

	if err := uc.db.Create(&alert).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create alert"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": alert})
}

// DeleteUserAlert deletes a user alert
// DELETE /api/v1/users/:id/alerts/:alert_id
func (uc *UserController) DeleteUserAlert(c *gin.Context) {
	userID := c.Param("id")
	alertID := c.Param("alert_id")

	if err := uc.db.Where("user_id = ? AND id = ?", userID, alertID).Delete(&models.UserAlert{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete alert"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert deleted"})
}

// UpdateLastLogin updates the last login timestamp
// POST /api/v1/users/:id/login
func (uc *UserController) UpdateLastLogin(c *gin.Context) {
	id := c.Param("id")

	now := time.Now()
	if err := uc.db.Model(&models.User{}).Where("id = ? OR supabase_user_id = ?", id, id).
		Update("last_login_at", now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update login time"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Login recorded", "timestamp": now})
}

// SyncFromSupabase syncs user data from Supabase auth
// POST /api/v1/users/sync
func (uc *UserController) SyncFromSupabase(c *gin.Context) {
	var request struct {
		SupabaseUserID string `json:"supabase_user_id" binding:"required"`
		Email          string `json:"email" binding:"required"`
		FullName       string `json:"full_name"`
		AvatarURL      string `json:"avatar_url"`
		EmailVerified  bool   `json:"email_verified"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	err := uc.db.Where("supabase_user_id = ?", request.SupabaseUserID).First(&user).Error

	if err == gorm.ErrRecordNotFound {
		// Create new user
		user = models.User{
			SupabaseUserID: request.SupabaseUserID,
			Email:          request.Email,
			FullName:       request.FullName,
			AvatarURL:      request.AvatarURL,
			EmailVerified:  request.EmailVerified,
			Role:           "user",
			IsActive:       true,
			Balance:        decimal.Zero,
		}
		if err := uc.db.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	} else {
		// Update existing user
		updates := map[string]interface{}{
			"email":          request.Email,
			"email_verified": request.EmailVerified,
		}
		if request.FullName != "" {
			updates["full_name"] = request.FullName
		}
		if request.AvatarURL != "" {
			updates["avatar_url"] = request.AvatarURL
		}
		if err := uc.db.Model(&user).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}