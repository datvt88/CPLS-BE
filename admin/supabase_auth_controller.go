package admin

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go_backend_project/models"
	"go_backend_project/services"

	"github.com/gin-gonic/gin"
)

// Session duration constants
const (
	SessionDuration     = 7 * 24 * time.Hour // 7 days session
	SessionCookieMaxAge = 7 * 24 * 60 * 60   // 7 days in seconds
	SessionExtendAfter  = 1 * time.Hour      // Extend session after 1 hour of activity
)

// SupabaseAuthController handles admin authentication via Supabase REST API
type SupabaseAuthController struct {
	supabaseClient *services.SupabaseDBClient
	memoryCache    *SessionCache // In-memory cache for fast lookups
}

// CachedSession wraps AdminSessionRecord with cache metadata
type CachedSession struct {
	Session  *services.AdminSessionRecord
	CachedAt time.Time
}

// SessionCache is an in-memory cache for sessions (backed by Supabase DB)
type SessionCache struct {
	mu       sync.RWMutex
	sessions map[string]*CachedSession
}

// NewSessionCache creates a new session cache
func NewSessionCache() *SessionCache {
	cache := &SessionCache{
		sessions: make(map[string]*CachedSession),
	}
	return cache
}

// Get retrieves a session from cache
func (c *SessionCache) Get(token string) (*CachedSession, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cached, exists := c.sessions[token]
	if !exists {
		return nil, false
	}
	if time.Now().After(cached.Session.ExpiresAt) {
		return nil, false
	}
	return cached, true
}

// Set stores a session in cache
func (c *SessionCache) Set(token string, session *services.AdminSessionRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessions[token] = &CachedSession{
		Session:  session,
		CachedAt: time.Now(),
	}
}

// Delete removes a session from cache
func (c *SessionCache) Delete(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, token)
}

// Global session cache
var globalSessionCache = NewSessionCache()

// NewSupabaseAuthController creates a new Supabase auth controller
func NewSupabaseAuthController() (*SupabaseAuthController, error) {
	client, err := services.NewSupabaseDBClient()
	if err != nil {
		return nil, err
	}

	controller := &SupabaseAuthController{
		supabaseClient: client,
		memoryCache:    globalSessionCache,
	}

	// Start background cleanup goroutine
	go controller.startSessionCleanup()

	return controller, nil
}

// startSessionCleanup periodically cleans up expired sessions from Supabase
func (ac *SupabaseAuthController) startSessionCleanup() {
	ticker := time.NewTicker(30 * time.Minute)
	for range ticker.C {
		if err := ac.supabaseClient.CleanupExpiredSessions(); err != nil {
			log.Printf("Session cleanup error: %v", err)
		}
	}
}

// isSecureModeSupabase returns true if running in production mode (HTTPS)
func isSecureModeSupabase() bool {
	return os.Getenv("ENVIRONMENT") == "production"
}

// LoginPage shows the login page
func (ac *SupabaseAuthController) LoginPage(c *gin.Context) {
	// Check if already logged in
	if _, err := ac.getSessionFromCookie(c); err == nil {
		c.Redirect(http.StatusFound, "/admin/dashboard")
		return
	}

	c.HTML(http.StatusOK, "login.html", gin.H{
		"error":           c.Query("error"),
		"supabaseEnabled": false, // Only using Supabase DB, not Supabase Auth
	})
}

// Login handles the login form submission
func (ac *SupabaseAuthController) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	if username == "" || password == "" {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{
			"error": "Username and password are required",
		})
		return
	}

	// Try to find user by username or email
	var user *services.AdminUserRecord
	var err error

	if strings.Contains(username, "@") {
		// Looks like an email
		user, err = ac.supabaseClient.GetAdminUserByEmail(username)
	} else {
		// Try username first
		user, err = ac.supabaseClient.GetAdminUserByUsername(username)
		if err != nil {
			// Fallback to email
			user, err = ac.supabaseClient.GetAdminUserByEmail(username)
		}
	}

	if err != nil {
		log.Printf("Admin login failed for %s: %v", username, err)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error": "Invalid username or password",
		})
		return
	}

	// Check password
	if !user.CheckPassword(password) {
		log.Printf("Admin login failed for %s: invalid password", username)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"error": "Invalid username or password",
		})
		return
	}

	// Create session
	token, err := generateSupabaseSessionToken()
	if err != nil {
		log.Printf("Failed to generate session token: %v", err)
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"error": "Failed to create session",
		})
		return
	}

	session := &services.AdminSessionRecord{
		Token:     token,
		AdminUser: user.ID,
		UserID:    user.ID, // Alias
		Username:  user.Username,
		Email:     user.Email,
		FullName:  user.FullName,
		Role:      user.Role,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		ExpiresAt: time.Now().Add(SessionDuration),
	}

	// Store session in Supabase DB (persisted across restarts)
	if err := ac.supabaseClient.CreateAdminSession(session); err != nil {
		log.Printf("Failed to create session in DB: %v", err)
		// Fallback: still allow login even if DB save fails
	}

	// Also cache in memory for fast lookups
	ac.memoryCache.Set(token, session)

	// Update last login in Supabase (async, don't block login)
	go func() {
		if err := ac.supabaseClient.UpdateLastLogin(user.ID); err != nil {
			log.Printf("Failed to update last login for user %d: %v", user.ID, err)
		}
	}()

	// Set session cookie (7 days)
	c.SetCookie("admin_session", token, SessionCookieMaxAge, "/admin", "", isSecureModeSupabase(), true)

	log.Printf("Admin user %s logged in successfully via Supabase", username)
	c.Redirect(http.StatusFound, "/admin/dashboard")
}

// Logout handles logout
func (ac *SupabaseAuthController) Logout(c *gin.Context) {
	token, err := c.Cookie("admin_session")
	if err == nil && token != "" {
		// Delete from memory cache
		ac.memoryCache.Delete(token)
		// Delete from Supabase DB
		ac.supabaseClient.DeleteAdminSession(token)
	}

	c.SetCookie("admin_session", "", -1, "/admin", "", isSecureModeSupabase(), true)
	c.Redirect(http.StatusFound, "/admin/login")
}

// AuthMiddleware checks if user is authenticated
func (ac *SupabaseAuthController) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cached, err := ac.getSessionFromCookie(c)
		if err != nil {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}

		session := cached.Session

		// Validate UserID before conversion (should always be positive)
		if session.UserID <= 0 {
			log.Printf("Invalid session UserID: %d", session.UserID)
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}

		// Create an AdminUser struct for template compatibility
		adminUser := models.AdminUser{
			ID:       uint(session.UserID),
			Username: session.Username,
			Email:    session.Email,
			FullName: session.FullName,
			Role:     session.Role,
			IsActive: true,
		}

		// Set admin user info in context (for template compatibility)
		c.Set("admin_user", adminUser)
		c.Set("admin_user_id", session.UserID)
		c.Set("admin_username", session.Username)
		c.Set("admin_email", session.Email)
		c.Set("admin_fullname", session.FullName)
		c.Set("admin_role", session.Role)
		c.Set("admin_session", session)

		// Extend session if cached for more than SessionExtendAfter
		token, _ := c.Cookie("admin_session")
		timeSinceCached := time.Since(cached.CachedAt)
		if timeSinceCached > SessionExtendAfter {
			newExpiry := time.Now().Add(SessionDuration)
			session.ExpiresAt = newExpiry
			ac.memoryCache.Set(token, session)
			// Extend in DB asynchronously
			go ac.supabaseClient.ExtendAdminSession(token, newExpiry)
			// Refresh cookie
			c.SetCookie("admin_session", token, SessionCookieMaxAge, "/admin", "", isSecureModeSupabase(), true)
		}

		c.Next()
	}
}

// getSessionFromCookie retrieves the admin session from cookie
func (ac *SupabaseAuthController) getSessionFromCookie(c *gin.Context) (*CachedSession, error) {
	token, err := c.Cookie("admin_session")
	if err != nil {
		return nil, err
	}

	// Try memory cache first (fast)
	if cached, exists := ac.memoryCache.Get(token); exists {
		return cached, nil
	}

	// Fallback to Supabase DB (persisted sessions)
	session, err := ac.supabaseClient.GetAdminSessionByToken(token)
	if err != nil {
		return nil, err
	}

	// Cache in memory for faster future lookups
	ac.memoryCache.Set(token, session)

	// Return newly cached session
	cached, _ := ac.memoryCache.Get(token)
	return cached, nil
}

// generateSupabaseSessionToken generates a secure random session token
func generateSupabaseSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GetSupabaseClient returns the Supabase client for other operations
func (ac *SupabaseAuthController) GetSupabaseClient() *services.SupabaseDBClient {
	return ac.supabaseClient
}

// TestConnection tests the connection to Supabase
func (ac *SupabaseAuthController) TestConnection() error {
	return ac.supabaseClient.TestConnection()
}

// ConnectionStatusHandler returns the Supabase connection status as JSON
func (ac *SupabaseAuthController) ConnectionStatusHandler(c *gin.Context) {
	err := ac.supabaseClient.TestConnection()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"connected": false,
			"status":    "disconnected",
			"message":   "Cannot connect to Supabase database",
			"error":     err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connected": true,
		"status":    "connected",
		"message":   "Successfully connected to Supabase",
	})
}
