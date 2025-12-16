package admin

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"time"

	"go_backend_project/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

	c.HTML(http.StatusOK, "login.html", gin.H{
		"error": c.Query("error"),
	})
}

// Login handles the login form submission
func (ac *AuthController) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	if username == "" || password == "" {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{
			"error": "Username and password are required",
		})
		return
	}

	// Find admin user
	var admin models.AdminUser
	if err := ac.db.Where("username = ? AND is_active = ?", username, true).First(&admin).Error; err != nil {
		log.Printf("Admin login failed for user %s: user not found", username)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error": "Invalid username or password",
		})
		return
	}

	// Check password
	if !admin.CheckPassword(password) {
		log.Printf("Admin login failed for user %s: invalid password", username)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error": "Invalid username or password",
		})
		return
	}

	// Create session
	token, err := generateSessionToken()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"error": "Failed to create session",
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
			"error": "Failed to create session",
		})
		return
	}

	// Update last login
	now := time.Now()
	ac.db.Model(&admin).Update("last_login_at", now)

	// Set session cookie (secure in production)
	c.SetCookie("admin_session", token, 86400, "/admin", "", isSecureMode(), true)

	log.Printf("Admin user %s logged in successfully", username)
	c.Redirect(http.StatusFound, "/admin")
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
