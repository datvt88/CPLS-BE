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

// SubscriptionController handles subscription-related requests
type SubscriptionController struct {
	db *gorm.DB
}

// NewSubscriptionController creates a new subscription controller
func NewSubscriptionController(db *gorm.DB) *SubscriptionController {
	return &SubscriptionController{db: db}
}

// GetPlans returns all available subscription plans
// GET /api/v1/subscriptions/plans
func (sc *SubscriptionController) GetPlans(c *gin.Context) {
	var plans []models.SubscriptionPlan

	if err := sc.db.Where("is_active = ?", true).Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch plans"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": plans})
}

// GetPlan returns a single plan by ID
// GET /api/v1/subscriptions/plans/:id
func (sc *SubscriptionController) GetPlan(c *gin.Context) {
	id := c.Param("id")

	var plan models.SubscriptionPlan
	if err := sc.db.First(&plan, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Plan not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": plan})
}

// CreatePlan creates a new subscription plan (admin only)
// POST /api/v1/subscriptions/plans
func (sc *SubscriptionController) CreatePlan(c *gin.Context) {
	var request struct {
		Name                  string  `json:"name" binding:"required"`
		Description           string  `json:"description"`
		Price                 float64 `json:"price" binding:"required"`
		Currency              string  `json:"currency"`
		BillingCycle          string  `json:"billing_cycle" binding:"required"`
		Features              string  `json:"features"`
		MaxWatchlist          int     `json:"max_watchlist"`
		MaxAlerts             int     `json:"max_alerts"`
		MaxStrategies         int     `json:"max_strategies"`
		HasBacktesting        bool    `json:"has_backtesting"`
		HasAutoTrading        bool    `json:"has_auto_trading"`
		HasAdvancedIndicators bool    `json:"has_advanced_indicators"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	plan := models.SubscriptionPlan{
		Name:                  request.Name,
		Description:           request.Description,
		Price:                 decimal.NewFromFloat(request.Price),
		Currency:              request.Currency,
		BillingCycle:          request.BillingCycle,
		Features:              request.Features,
		MaxWatchlist:          request.MaxWatchlist,
		MaxAlerts:             request.MaxAlerts,
		MaxStrategies:         request.MaxStrategies,
		HasBacktesting:        request.HasBacktesting,
		HasAutoTrading:        request.HasAutoTrading,
		HasAdvancedIndicators: request.HasAdvancedIndicators,
		IsActive:              true,
	}

	if err := sc.db.Create(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create plan"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": plan})
}

// GetUserSubscription returns user's current subscription
// GET /api/v1/subscriptions/user/:user_id
func (sc *SubscriptionController) GetUserSubscription(c *gin.Context) {
	userID := c.Param("user_id")

	var subscription models.Subscription
	if err := sc.db.Where("user_id = ?", userID).Preload("Plan").First(&subscription).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "No active subscription"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subscription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": subscription})
}

// Subscribe subscribes user to a plan
// POST /api/v1/subscriptions/subscribe
func (sc *SubscriptionController) Subscribe(c *gin.Context) {
	var request struct {
		UserID        uint   `json:"user_id" binding:"required"`
		PlanID        uint   `json:"plan_id" binding:"required"`
		PaymentMethod string `json:"payment_method" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if plan exists
	var plan models.SubscriptionPlan
	if err := sc.db.First(&plan, request.PlanID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Plan not found"})
		return
	}

	// Check if user already has an active subscription
	var existing models.Subscription
	if err := sc.db.Where("user_id = ? AND status = ?", request.UserID, "active").First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User already has an active subscription"})
		return
	}

	now := time.Now()
	var endDate time.Time
	switch plan.BillingCycle {
	case "monthly":
		endDate = now.AddDate(0, 1, 0)
	case "yearly":
		endDate = now.AddDate(1, 0, 0)
	default:
		endDate = now.AddDate(0, 1, 0)
	}

	subscription := models.Subscription{
		UserID:        request.UserID,
		PlanID:        request.PlanID,
		Status:        "active",
		StartDate:     now,
		EndDate:       endDate,
		AutoRenew:     true,
		PaymentMethod: request.PaymentMethod,
		NextPaymentAt: &endDate,
	}

	if err := sc.db.Create(&subscription).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create subscription"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": subscription})
}

// CancelSubscription cancels user's subscription
// POST /api/v1/subscriptions/cancel
func (sc *SubscriptionController) CancelSubscription(c *gin.Context) {
	var request struct {
		UserID       uint   `json:"user_id" binding:"required"`
		CancelReason string `json:"cancel_reason"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var subscription models.Subscription
	if err := sc.db.Where("user_id = ? AND status = ?", request.UserID, "active").First(&subscription).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active subscription found"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":        "cancelled",
		"auto_renew":    false,
		"cancelled_at":  now,
		"cancel_reason": request.CancelReason,
	}

	if err := sc.db.Model(&subscription).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel subscription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subscription cancelled", "valid_until": subscription.EndDate})
}

// GetPaymentHistory returns user's payment history
// GET /api/v1/subscriptions/payments/:user_id
func (sc *SubscriptionController) GetPaymentHistory(c *gin.Context) {
	userID := c.Param("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	var payments []models.PaymentHistory
	var total int64

	sc.db.Model(&models.PaymentHistory{}).Where("user_id = ?", userID).Count(&total)

	if err := sc.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&payments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch payment history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": payments,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}