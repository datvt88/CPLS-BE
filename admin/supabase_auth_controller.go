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

	"go_backend_project/services"

	"github.com/gin-gonic/gin"
)

// SupabaseAuthController handles admin authentication via Supabase REST API
type SupabaseAuthController struct {
	supabaseClient *services.SupabaseDBClient
	sessions       *SessionStore
}

// SessionStore stores admin sessions in memory (since we don't have direct DB access)
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*AdminSessionData
}

// AdminSessionData represents a session stored in memory
type AdminSessionData struct {
	UserID    int
	Username  string
	Email     string
	FullName  string
	Role      string
	IPAddress string
	UserAgent string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// NewSessionStore creates a new session store
func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*AdminSessionData),
	}
	// Start cleanup goroutine
	go store.cleanupExpiredSessions()
	return store
}

// cleanupExpiredSessions periodically removes expired sessions
func (s *SessionStore) cleanupExpiredSessions() {
	ticker := time.NewTicker(15 * time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for token, session := range s.sessions {
			if now.After(session.ExpiresAt) {
				delete(s.sessions, token)
			}
		}
		s.mu.Unlock()
	}
}

// Set stores a session
func (s *SessionStore) Set(token string, session *AdminSessionData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = session
}

// Get retrieves a session
func (s *SessionStore) Get(token string) (*AdminSessionData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, exists := s.sessions[token]
	if !exists {
		return nil, false
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, false
	}
	return session, true
}

// Delete removes a session
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// Global session store
var globalSessionStore = NewSessionStore()

// NewSupabaseAuthController creates a new Supabase auth controller
func NewSupabaseAuthController() (*SupabaseAuthController, error) {
	client, err := services.NewSupabaseDBClient()
	if err != nil {
		return nil, err
	}

	return &SupabaseAuthController{
		supabaseClient: client,
		sessions:       globalSessionStore,
	}, nil
}

// isSecureModeSupabase returns true if running in production mode (HTTPS)
func isSecureModeSupabase() bool {
	return os.Getenv("ENVIRONMENT") == "production"
}

// LoginPage shows the login page
func (ac *SupabaseAuthController) LoginPage(c *gin.Context) {
	// Check if already logged in
	if _, err := ac.getSessionFromCookie(c); err == nil {
		c.Redirect(http.StatusFound, "/admin")
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

	session := &AdminSessionData{
		UserID:    user.ID,
		Username:  user.Username,
		Email:     user.Email,
		FullName:  user.FullName,
		Role:      user.Role,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}

	// Store session in memory
	ac.sessions.Set(token, session)

	// Update last login in Supabase (async, don't block login)
	go func() {
		if err := ac.supabaseClient.UpdateLastLogin(user.ID); err != nil {
			log.Printf("Failed to update last login for user %d: %v", user.ID, err)
		}
	}()

	// Set session cookie
	c.SetCookie("admin_session", token, 86400, "/admin", "", isSecureModeSupabase(), true)

	log.Printf("Admin user %s logged in successfully via Supabase", username)
	c.Redirect(http.StatusFound, "/admin")
}

// Logout handles logout
func (ac *SupabaseAuthController) Logout(c *gin.Context) {
	token, err := c.Cookie("admin_session")
	if err == nil && token != "" {
		ac.sessions.Delete(token)
	}

	c.SetCookie("admin_session", "", -1, "/admin", "", isSecureModeSupabase(), true)
	c.Redirect(http.StatusFound, "/admin/login")
}

// AuthMiddleware checks if user is authenticated
func (ac *SupabaseAuthController) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := ac.getSessionFromCookie(c)
		if err != nil {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}

		// Set admin user info in context
		c.Set("admin_user_id", session.UserID)
		c.Set("admin_username", session.Username)
		c.Set("admin_email", session.Email)
		c.Set("admin_fullname", session.FullName)
		c.Set("admin_role", session.Role)
		c.Set("admin_session", session)
		c.Next()
	}
}

// getSessionFromCookie retrieves the admin session from cookie
func (ac *SupabaseAuthController) getSessionFromCookie(c *gin.Context) (*AdminSessionData, error) {
	token, err := c.Cookie("admin_session")
	if err != nil {
		return nil, err
	}

	session, exists := ac.sessions.Get(token)
	if !exists {
		return nil, http.ErrNoCookie
	}

	return session, nil
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
