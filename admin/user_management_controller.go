package admin

import (
	"net/http"
	"strconv"
	"time"

	"go_backend_project/services"

	"github.com/gin-gonic/gin"
)

// UserManagementController handles user management operations
type UserManagementController struct {
	supabaseClient *services.SupabaseDBClient
}

// NewUserManagementController creates a new user management controller
func NewUserManagementController(client *services.SupabaseDBClient) *UserManagementController {
	return &UserManagementController{
		supabaseClient: client,
	}
}

// ListUsers handles GET /admin/users - displays user list page
func (ctrl *UserManagementController) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	result, err := ctrl.supabaseClient.GetProfiles(page, pageSize, search, sortBy, sortOrder)
	if err != nil {
		c.HTML(http.StatusOK, "users_management.html", gin.H{
			"Title":     "User Management",
			"AdminUser": c.GetString("admin_username"),
			"Error":     err.Error(),
		})
		return
	}

	// Get stats
	stats, _ := ctrl.supabaseClient.GetProfileStats()

	c.HTML(http.StatusOK, "users_management.html", gin.H{
		"Title":      "User Management",
		"AdminUser":  c.GetString("admin_username"),
		"Users":      result.Profiles,
		"Total":      result.Total,
		"Page":       result.Page,
		"PageSize":   result.PageSize,
		"TotalPages": result.TotalPages,
		"Search":     search,
		"SortBy":     sortBy,
		"SortOrder":  sortOrder,
		"Stats":      stats,
	})
}

// GetUser handles GET /admin/api/users/:id - returns user details as JSON
func (ctrl *UserManagementController) GetUser(c *gin.Context) {
	userID := c.Param("id")

	profile, err := ctrl.supabaseClient.GetProfileByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Try to get auth user info
	authUser, authErr := ctrl.supabaseClient.GetAuthUser(userID)

	c.JSON(http.StatusOK, gin.H{
		"profile":   profile,
		"auth_user": authUser,
		"auth_err":  authErr,
	})
}

// CreateUser handles POST /admin/api/users - creates a new user
func (ctrl *UserManagementController) CreateUser(c *gin.Context) {
	var input struct {
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"required,min=6"`
		FullName    string `json:"full_name"`
		Nickname    string `json:"nickname"`
		PhoneNumber string `json:"phone_number"`
		Plan        string `json:"subscription_plan"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create user in Supabase Auth
	metadata := map[string]interface{}{
		"full_name": input.FullName,
		"nickname":  input.Nickname,
	}

	authUser, err := ctrl.supabaseClient.CreateAuthUser(input.Email, input.Password, metadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create auth user: " + err.Error()})
		return
	}

	// Create profile
	profileInput := &services.UserProfileInput{
		Email:            input.Email,
		FullName:         input.FullName,
		Nickname:         input.Nickname,
		PhoneNumber:      input.PhoneNumber,
		SubscriptionPlan: input.Plan,
	}

	if profileInput.SubscriptionPlan == "" {
		profileInput.SubscriptionPlan = "free"
	}

	profile, err := ctrl.supabaseClient.CreateProfile(authUser.ID, profileInput)
	if err != nil {
		// Try to rollback auth user creation
		ctrl.supabaseClient.DeleteAuthUser(authUser.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create profile: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User created successfully",
		"profile": profile,
	})
}

// UpdateUser handles PUT /admin/api/users/:id - updates user info
func (ctrl *UserManagementController) UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	var input services.UserProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	profile, err := ctrl.supabaseClient.UpdateProfile(userID, &input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User updated successfully",
		"profile": profile,
	})
}

// DeleteUser handles DELETE /admin/api/users/:id - deletes a user
func (ctrl *UserManagementController) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	// Delete from Supabase Auth first
	if err := ctrl.supabaseClient.DeleteAuthUser(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete auth user: " + err.Error()})
		return
	}

	// Delete profile
	if err := ctrl.supabaseClient.DeleteProfile(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete profile: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

// BanUser handles POST /admin/api/users/:id/ban - bans a user
func (ctrl *UserManagementController) BanUser(c *gin.Context) {
	userID := c.Param("id")

	var input struct {
		Reason string `json:"reason"`
	}
	c.ShouldBindJSON(&input)

	if err := ctrl.supabaseClient.BanUser(userID, input.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User banned successfully"})
}

// UnbanUser handles POST /admin/api/users/:id/unban - unbans a user
func (ctrl *UserManagementController) UnbanUser(c *gin.Context) {
	userID := c.Param("id")

	if err := ctrl.supabaseClient.UnbanUser(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User unbanned successfully"})
}

// UpdateSubscription handles POST /admin/api/users/:id/subscription - updates user subscription
func (ctrl *UserManagementController) UpdateSubscription(c *gin.Context) {
	userID := c.Param("id")

	var input struct {
		Plan    string `json:"plan" binding:"required"`
		EndDate string `json:"end_date"` // RFC3339 format
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var endDate *time.Time
	if input.EndDate != "" {
		t, err := time.Parse(time.RFC3339, input.EndDate)
		if err != nil {
			// Try alternate format
			t, err = time.Parse("2006-01-02", input.EndDate)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_date format"})
				return
			}
		}
		endDate = &t
	}

	if err := ctrl.supabaseClient.UpdateSubscription(userID, input.Plan, endDate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subscription updated successfully"})
}

// ResetPassword handles POST /admin/api/users/:id/reset-password - resets user password
func (ctrl *UserManagementController) ResetPassword(c *gin.Context) {
	userID := c.Param("id")

	var input struct {
		NewPassword string `json:"new_password"`
		SendLink    bool   `json:"send_link"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// If new password provided, update directly
	if input.NewPassword != "" {
		if len(input.NewPassword) < 6 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 6 characters"})
			return
		}

		if err := ctrl.supabaseClient.UpdateAuthUserPassword(userID, input.NewPassword); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully"})
		return
	}

	// If send_link is true, generate password reset link
	if input.SendLink {
		profile, err := ctrl.supabaseClient.GetProfileByID(userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}

		link, err := ctrl.supabaseClient.GeneratePasswordResetLink(profile.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "Password reset link generated",
			"reset_link": link,
		})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "Provide new_password or set send_link to true"})
}

// SyncUser handles POST /admin/api/users/:id/sync - syncs profile with auth data
func (ctrl *UserManagementController) SyncUser(c *gin.Context) {
	userID := c.Param("id")

	profile, err := ctrl.supabaseClient.SyncProfileWithAuth(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User synced successfully",
		"profile": profile,
	})
}

// SyncAllUsers handles POST /admin/api/users/sync-all - syncs all profiles with auth data
func (ctrl *UserManagementController) SyncAllUsers(c *gin.Context) {
	// Get all profiles
	result, err := ctrl.supabaseClient.GetProfiles(1, 1000, "", "created_at", "desc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	synced := 0
	failed := 0
	var errors []string

	for _, profile := range result.Profiles {
		_, err := ctrl.supabaseClient.SyncProfileWithAuth(profile.ID)
		if err != nil {
			failed++
			errors = append(errors, profile.Email+": "+err.Error())
		} else {
			synced++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Sync completed",
		"synced":  synced,
		"failed":  failed,
		"errors":  errors,
	})
}

// GetStats handles GET /admin/api/users/stats - returns user statistics
func (ctrl *UserManagementController) GetStats(c *gin.Context) {
	stats, err := ctrl.supabaseClient.GetProfileStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ExportUsers handles GET /admin/api/users/export - exports users to JSON
func (ctrl *UserManagementController) ExportUsers(c *gin.Context) {
	result, err := ctrl.supabaseClient.GetProfiles(1, 10000, "", "created_at", "desc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Disposition", "attachment; filename=users_export.json")
	c.JSON(http.StatusOK, result.Profiles)
}
