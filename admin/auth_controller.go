package admin

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"go_backend_project/models"
	"go_backend_project/services"

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

// setAdminSessionCookie sets the admin session cookie with proper Cloud Run configuration
func setAdminSessionCookie(c *gin.Context, token string, maxAge int) {
	// For Cloud Run: Use Path="/", Secure=true in production, SameSite=None for cross-origin
	secure := isSecureMode()
	sameSite := http.SameSiteDefaultMode
	
	if secure {
		// For HTTPS (Cloud Run production), use SameSite=None to allow cross-origin
		sameSite = http.SameSiteNoneMode
	} else {
		// For local development (HTTP), use SameSite=Lax
		sameSite = http.SameSiteLaxMode
	}
	
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "admin_session",
		Value:    token,
		Path:     "/", // Changed from "/admin" to "/" for broader access
		MaxAge:   maxAge,
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})
}

// clearAdminSessionCookie removes the admin session cookie
func clearAdminSessionCookie(c *gin.Context) {
	setAdminSessionCookie(c, "", -1)
}

// LoginPage shows the login page
func (ac *AuthController) LoginPage(c *gin.Context) {
	// Check if already logged in
	if _, err := ac.getSessionFromCookie(c); err == nil {
		c.Redirect(http.StatusFound, "/admin/dashboard")
		return
	}

	// Check database connection status
	connectionError := ""
	if ac.db != nil {
		sqlDB, err := ac.db.DB()
		if err != nil {
			log.Printf("Database connection check failed: %v", err)
			connectionError = "Không thể kết nối đến cơ sở dữ liệu. Vui lòng kiểm tra cấu hình hoặc liên hệ quản trị viên."
		} else if err := sqlDB.Ping(); err != nil {
			log.Printf("Database ping failed: %v", err)
			connectionError = "Không thể kết nối đến cơ sở dữ liệu. Vui lòng kiểm tra cấu hình hoặc liên hệ quản trị viên."
		}
	} else {
		connectionError = "Cơ sở dữ liệu chưa được khởi tạo. Vui lòng liên hệ quản trị viên."
	}

	c.HTML(http.StatusOK, "login.html", gin.H{
		"error":            c.Query("error"),
		"connectionError":  connectionError,
		"supabaseEnabled":  isSupabaseEnabled(),
	})
}

// RootRedirect handles the root /admin path and redirects appropriately
// Redirects to login if no session cookie, or to dashboard if cookie exists.
// Note: This only checks cookie existence for a quick decision. If the cookie contains
// an invalid/expired token, the user will be redirected to /admin/dashboard first,
// then the AuthMiddleware will redirect them back to /admin/login. This extra redirect
// is acceptable and only happens once per invalid session.
// Full session validation happens in AuthMiddleware on protected routes.
func (ac *AuthController) RootRedirect(c *gin.Context) {
	// Quick check for session cookie existence
	// Full session validation happens in AuthMiddleware on protected routes
	if token, err := c.Cookie("admin_session"); err == nil && token != "" {
		// Has cookie, redirect to dashboard (AuthMiddleware will validate)
		c.Redirect(http.StatusFound, "/admin/dashboard")
	} else {
		// No cookie, redirect to login
		c.Redirect(http.StatusFound, "/admin/login")
	}
}

// Login handles the login form submission
// Supports both local admin login (username/password) and Supabase Auth login (email/password)
func (ac *AuthController) Login(c *gin.Context) {
	// Check database connection before attempting login
	if ac.db != nil {
		sqlDB, err := ac.db.DB()
		if err != nil {
			log.Printf("Database connection check failed during login: %v", err)
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error":            "Không thể đăng nhập do lỗi kết nối cơ sở dữ liệu.",
				"connectionError":  "Không thể kết nối đến cơ sở dữ liệu. Vui lòng kiểm tra cấu hình hoặc liên hệ quản trị viên.",
				"supabaseEnabled":  isSupabaseEnabled(),
			})
			return
		} else if err := sqlDB.Ping(); err != nil {
			log.Printf("Database ping failed during login: %v", err)
			c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
				"error":            "Không thể đăng nhập do lỗi kết nối cơ sở dữ liệu.",
				"connectionError":  "Không thể kết nối đến cơ sở dữ liệu. Vui lòng kiểm tra cấu hình hoặc liên hệ quản trị viên.",
				"supabaseEnabled":  isSupabaseEnabled(),
			})
			return
		}
	} else {
		c.HTML(http.StatusServiceUnavailable, "login.html", gin.H{
			"error":            "Không thể đăng nhập do lỗi kết nối cơ sở dữ liệu.",
			"connectionError":  "Cơ sở dữ liệu chưa được khởi tạo. Vui lòng liên hệ quản trị viên.",
			"supabaseEnabled":  isSupabaseEnabled(),
		})
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")
	loginMethod := c.PostForm("login_method") // "local" or "supabase"

	if username == "" || password == "" {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{
			"error":            "Username/Email and password are required",
			"supabaseEnabled":  isSupabaseEnabled(),
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

// loginWithLocal handles local admin authentication
func (ac *AuthController) loginWithLocal(c *gin.Context, username, password string) {
	// Find admin user
	var admin models.AdminUser
	if err := ac.db.Where("username = ? AND is_active = ?", username, true).First(&admin).Error; err != nil {
		// Also try by email
		if err := ac.db.Where("email = ? AND is_active = ?", username, true).First(&admin).Error; err != nil {
			log.Printf("Admin login failed for user %s: user not found", username)
			c.HTML(http.StatusUnauthorized, "login.html", gin.H{
				"error":            "Invalid username or password",
				"supabaseEnabled":  isSupabaseEnabled(),
			})
			return
		}
	}

	// Check password
	if !admin.CheckPassword(password) {
		log.Printf("Admin login failed for user %s: invalid password", username)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error":            "Invalid username or password",
			"supabaseEnabled":  isSupabaseEnabled(),
		})
		return
	}

	// Create session
	ac.createSessionAndRedirect(c, &admin)
}

// loginWithSupabase handles Supabase Auth authentication
func (ac *AuthController) loginWithSupabase(c *gin.Context, email, password string) bool {
	supabaseAuth, err := services.NewSupabaseAuthService()
	if err != nil {
		log.Printf("Supabase auth service error: %v", err)
		return false
	}

	// Authenticate with Supabase
	authResp, err := supabaseAuth.SignInWithPassword(email, password)
	if err != nil {
		log.Printf("Supabase login failed for %s: %v", email, err)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error":            "Invalid email or password",
			"supabaseEnabled":  isSupabaseEnabled(),
		})
		return true // Handled, don't fallback
	}

	// Check if user has admin role
	if !supabaseAuth.IsAdmin(&authResp.User) {
		log.Printf("Supabase login failed for %s: not an admin", email)
		c.HTML(http.StatusForbidden, "login.html", gin.H{
			"error":            "You don't have admin privileges. Contact your administrator.",
			"supabaseEnabled":  isSupabaseEnabled(),
		})
		return true
	}

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
				"error":            "Failed to sync admin user",
				"supabaseEnabled":  isSupabaseEnabled(),
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
			"error":            "Failed to create session",
			"supabaseEnabled":  isSupabaseEnabled(),
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
			"error":            "Failed to create session",
			"supabaseEnabled":  isSupabaseEnabled(),
		})
		return
	}

	// Update last login
	now := time.Now()
	ac.db.Model(admin).Update("last_login_at", now)

	// Set session cookie (secure in production) - 24 hours
	setAdminSessionCookie(c, token, 86400)

	log.Printf("Admin user %s logged in successfully", admin.Username)
	c.Redirect(http.StatusFound, "/admin/dashboard")
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

	// Clear session cookie
	clearAdminSessionCookie(c)
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
