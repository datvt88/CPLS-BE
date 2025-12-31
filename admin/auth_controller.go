package admin

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"go_backend_project/middleware"
	"go_backend_project/models"
	"go_backend_project/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Local session configuration constants
const (
	LocalSessionDurationHours = 24
	LocalSessionCookieMaxAge  = 24 * 60 * 60 // 24 hours in seconds
)

// AuthController handles admin authentication
type AuthController struct {
	db *gorm.DB
}

// NewAuthController creates a new auth controller
func NewAuthController(db *gorm.DB) *AuthController {
	return &AuthController{db: db}
}

// isSecureMode returns true if running in production mode (HTTPS)
func isSecureMode() bool {
	return os.Getenv("ENVIRONMENT") == "production"
}

// LoginPage shows the login page
func (ac *AuthController) LoginPage(c *gin.Context) {
	// Check if already logged in
	if _, err := ac.getSessionFromCookie(c); err == nil {
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	// Generate CSRF token for the form
	csrfToken := middleware.SetCSRFToken(c)

	// Check rate limit status
	ip := c.ClientIP()
	_, remaining, _ := middleware.GetLoginRateLimiter().Check(ip)

	c.HTML(http.StatusOK, "login.html", gin.H{
		"error":             c.Query("error"),
		"supabaseEnabled":   isSupabaseEnabled(),
		"csrf_token":        csrfToken,
		"attemptsRemaining": remaining,
	})
}

// Login handles the login form submission
// Supports both local admin login (username/password) and Supabase Auth login (email/password)
func (ac *AuthController) Login(c *gin.Context) {
	ip := c.ClientIP()

	// Validate CSRF token first
	csrfToken := c.PostForm("csrf_token")
	if csrfToken == "" {
		csrfToken = c.GetHeader("X-CSRF-Token")
	}
	if !middleware.ValidateCSRFToken(csrfToken) {
		log.Printf("SECURITY: CSRF validation failed from IP %s", ip)
		newCSRFToken := middleware.SetCSRFToken(c)
		c.HTML(http.StatusForbidden, "login.html", gin.H{
			"error":           "Security token expired. Please try again.",
			"supabaseEnabled": isSupabaseEnabled(),
			"csrf_token":      newCSRFToken,
		})
		return
	}

	// Check rate limit
	allowed, remaining, lockDuration := middleware.GetLoginRateLimiter().Check(ip)
	if !allowed {
		minutes := int(lockDuration.Minutes())
		seconds := int(lockDuration.Seconds()) % 60
		log.Printf("SECURITY: Rate limit exceeded from IP %s", ip)
		newCSRFToken := middleware.SetCSRFToken(c)
		c.HTML(http.StatusTooManyRequests, "login.html", gin.H{
			"error":           formatRateLimitMessage(minutes, seconds),
			"supabaseEnabled": isSupabaseEnabled(),
			"csrf_token":      newCSRFToken,
			"rateLimited":     true,
		})
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")
	loginMethod := c.PostForm("login_method") // "local" or "supabase"

	if username == "" || password == "" {
		newCSRFToken := middleware.SetCSRFToken(c)
		c.HTML(http.StatusBadRequest, "login.html", gin.H{
			"error":             "Username/Email and password are required",
			"supabaseEnabled":   isSupabaseEnabled(),
			"csrf_token":        newCSRFToken,
			"attemptsRemaining": remaining,
		})
		return
	}

	// If login method is supabase or username looks like an email, try Supabase first
	if loginMethod == "supabase" || (strings.Contains(username, "@") && isSupabaseEnabled()) {
		if ac.loginWithSupabase(c, username, password) {
			return
		}
		// If Supabase login fails and it's explicitly supabase method, don't fallback
		if loginMethod == "supabase" {
			return
		}
		// Otherwise, try local login as fallback
	}

	// Local admin login
	ac.loginWithLocal(c, username, password)
}

// formatRateLimitMessage formats a user-friendly rate limit message
func formatRateLimitMessage(minutes, _ int) string {
	if minutes > 0 {
		return "Too many failed login attempts. Please try again later."
	}
	return "Too many failed login attempts. Please try again shortly."
}

// loginWithLocal handles local admin authentication
func (ac *AuthController) loginWithLocal(c *gin.Context, username, password string) {
	ip := c.ClientIP()

	// Find admin user
	var admin models.AdminUser
	if err := ac.db.Where("username = ? AND is_active = ?", username, true).First(&admin).Error; err != nil {
		// Also try by email
		if err := ac.db.Where("email = ? AND is_active = ?", username, true).First(&admin).Error; err != nil {
			// Record failed attempt
			middleware.RecordLoginAttempt(ip, false)
			log.Printf("SECURITY: Admin login failed for user %s from IP %s: user not found", username, ip)

			newCSRFToken := middleware.SetCSRFToken(c)
			_, remaining, _ := middleware.GetLoginRateLimiter().Check(ip)
			c.HTML(http.StatusUnauthorized, "login.html", gin.H{
				"error":             "Invalid username or password",
				"supabaseEnabled":   isSupabaseEnabled(),
				"csrf_token":        newCSRFToken,
				"attemptsRemaining": remaining,
			})
			return
		}
	}

	// Check password
	if !admin.CheckPassword(password) {
		// Record failed attempt
		middleware.RecordLoginAttempt(ip, false)
		log.Printf("SECURITY: Admin login failed for user %s from IP %s: invalid password", username, ip)

		newCSRFToken := middleware.SetCSRFToken(c)
		_, remaining, _ := middleware.GetLoginRateLimiter().Check(ip)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error":             "Invalid username or password",
			"supabaseEnabled":   isSupabaseEnabled(),
			"csrf_token":        newCSRFToken,
			"attemptsRemaining": remaining,
		})
		return
	}

	// Successful login - clear rate limit
	middleware.RecordLoginAttempt(ip, true)

	// Create session
	ac.createSessionAndRedirect(c, &admin)
}

// loginWithSupabase handles Supabase Auth authentication
func (ac *AuthController) loginWithSupabase(c *gin.Context, email, password string) bool {
	ip := c.ClientIP()

	supabaseAuth, err := services.NewSupabaseAuthService()
	if err != nil {
		log.Printf("Supabase auth service error: %v", err)
		return false
	}

	// Authenticate with Supabase
	authResp, err := supabaseAuth.SignInWithPassword(email, password)
	if err != nil {
		// Record failed attempt
		middleware.RecordLoginAttempt(ip, false)
		log.Printf("SECURITY: Supabase login failed for %s from IP %s: %v", email, ip, err)

		newCSRFToken := middleware.SetCSRFToken(c)
		_, remaining, _ := middleware.GetLoginRateLimiter().Check(ip)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error":             "Invalid email or password",
			"supabaseEnabled":   isSupabaseEnabled(),
			"csrf_token":        newCSRFToken,
			"attemptsRemaining": remaining,
		})
		return true // Handled, don't fallback
	}

	// Check if user has admin role
	if !supabaseAuth.IsAdmin(&authResp.User) {
		log.Printf("SECURITY: Supabase login failed for %s from IP %s: not an admin", email, ip)
		newCSRFToken := middleware.SetCSRFToken(c)
		c.HTML(http.StatusForbidden, "login.html", gin.H{
			"error":           "You don't have admin privileges. Contact your administrator.",
			"supabaseEnabled": isSupabaseEnabled(),
			"csrf_token":      newCSRFToken,
		})
		return true
	}

	// Successful login - clear rate limit
	middleware.RecordLoginAttempt(ip, true)

	// Find or create admin user in local database
	var admin models.AdminUser
	if err := ac.db.Where("email = ?", email).First(&admin).Error; err != nil {
		// Create new admin user from Supabase
		admin = models.AdminUser{
			Username:     authResp.User.Email, // Use email as username
			Email:        authResp.User.Email,
			FullName:     getFullNameFromMetadata(authResp.User.UserMetadata),
			Role:         getRoleFromMetadata(authResp.User.AppMetadata),
			IsActive:     true,
			PasswordHash: "", // No local password, uses Supabase
		}
		// Set a random password hash (won't be used but required by schema)
		randomPass, _ := generateSessionToken()
		admin.SetPassword(randomPass)

		if err := ac.db.Create(&admin).Error; err != nil {
			log.Printf("Failed to create admin user from Supabase: %v", err)
			c.HTML(http.StatusInternalServerError, "login.html", gin.H{
				"error":           "Failed to sync admin user",
				"supabaseEnabled": isSupabaseEnabled(),
			})
			return true
		}
		log.Printf("Created new admin user from Supabase: %s", email)
	}

	// Create session
	ac.createSessionAndRedirect(c, &admin)
	log.Printf("Admin user %s logged in via Supabase", email)
	return true
}

// createSessionAndRedirect creates a session and redirects to admin dashboard
func (ac *AuthController) createSessionAndRedirect(c *gin.Context, admin *models.AdminUser) {
	token, err := generateSessionToken()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"error":           "Failed to create session",
			"supabaseEnabled": isSupabaseEnabled(),
		})
		return
	}

	session := models.AdminSession{
		AdminUserID: admin.ID,
		Token:       token,
		IPAddress:   c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		ExpiresAt:   time.Now().Add(24 * time.Hour), // 24 hour session
	}

	if err := ac.db.Create(&session).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"error":           "Failed to create session",
			"supabaseEnabled": isSupabaseEnabled(),
		})
		return
	}

	// Update last login
	now := time.Now()
	ac.db.Model(admin).Update("last_login_at", now)

	// Set session cookie (secure in production)
	c.SetCookie("admin_session", token, 86400, "/admin", "", isSecureMode(), true)

	log.Printf("Admin user %s logged in successfully", admin.Username)
	c.Redirect(http.StatusFound, "/admin")
}

// isSupabaseEnabled checks if Supabase Auth is configured
func isSupabaseEnabled() bool {
	return os.Getenv("SUPABASE_URL") != "" && os.Getenv("SUPABASE_ANON_KEY") != ""
}

// getFullNameFromMetadata extracts full name from user metadata
func getFullNameFromMetadata(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	if name, ok := metadata["full_name"].(string); ok {
		return name
	}
	if name, ok := metadata["name"].(string); ok {
		return name
	}
	return ""
}

// getRoleFromMetadata extracts role from app metadata
func getRoleFromMetadata(metadata map[string]interface{}) string {
	if metadata == nil {
		return "admin"
	}
	if role, ok := metadata["role"].(string); ok {
		return role
	}
	return "admin"
}

// Logout handles logout
func (ac *AuthController) Logout(c *gin.Context) {
	token, err := c.Cookie("admin_session")
	if err == nil && token != "" {
		// Delete session from database
		ac.db.Where("token = ?", token).Delete(&models.AdminSession{})
	}

	// Clear cookie (secure in production)
	c.SetCookie("admin_session", "", -1, "/admin", "", isSecureMode(), true)
	c.Redirect(http.StatusFound, "/admin/login")
}

// AuthMiddleware checks if user is authenticated
func (ac *AuthController) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := ac.getSessionFromCookie(c)
		if err != nil {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}

		// Set admin user in context
		c.Set("admin_user", session.AdminUser)
		c.Set("admin_session", session)
		c.Next()
	}
}

// getSessionFromCookie retrieves the admin session from cookie
func (ac *AuthController) getSessionFromCookie(c *gin.Context) (*models.AdminSession, error) {
	token, err := c.Cookie("admin_session")
	if err != nil {
		return nil, err
	}

	var session models.AdminSession
	if err := ac.db.Preload("AdminUser").Where("token = ?", token).First(&session).Error; err != nil {
		return nil, err
	}

	if session.IsExpired() {
		// Clean up expired session
		ac.db.Delete(&session)
		return nil, gorm.ErrRecordNotFound
	}

	return &session, nil
}

// generateSessionToken generates a secure random session token
func generateSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
