package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// CSRFToken represents a CSRF token with expiration
type CSRFToken struct {
	Token     string
	CreatedAt time.Time
}

// CSRFStore manages CSRF tokens
type CSRFStore struct {
	mu     sync.RWMutex
	tokens map[string]*CSRFToken
	ttl    time.Duration
}

// Global CSRF store
var csrfStore *CSRFStore

// InitCSRFStore initializes the global CSRF store
func InitCSRFStore() {
	csrfStore = NewCSRFStore(30 * time.Minute)
	go csrfStore.startCleanup()
}

// NewCSRFStore creates a new CSRF store
func NewCSRFStore(ttl time.Duration) *CSRFStore {
	return &CSRFStore{
		tokens: make(map[string]*CSRFToken),
		ttl:    ttl,
	}
}

// startCleanup periodically removes expired tokens
func (s *CSRFStore) startCleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		s.cleanup()
	}
}

// cleanup removes expired tokens
func (s *CSRFStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for token, data := range s.tokens {
		if now.Sub(data.CreatedAt) > s.ttl {
			delete(s.tokens, token)
		}
	}
}

// GenerateToken creates a new CSRF token
func (s *CSRFStore) GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)

	s.mu.Lock()
	s.tokens[token] = &CSRFToken{
		Token:     token,
		CreatedAt: time.Now(),
	}
	s.mu.Unlock()

	return token, nil
}

// ValidateToken checks if a token is valid and removes it (one-time use)
func (s *CSRFStore) ValidateToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, exists := s.tokens[token]
	if !exists {
		return false
	}

	// Check expiration
	if time.Since(data.CreatedAt) > s.ttl {
		delete(s.tokens, token)
		return false
	}

	// Remove token after validation (one-time use)
	delete(s.tokens, token)
	return true
}

// GetCSRFStore returns the global CSRF store
func GetCSRFStore() *CSRFStore {
	if csrfStore == nil {
		InitCSRFStore()
	}
	return csrfStore
}

// GenerateCSRFToken generates a new CSRF token
func GenerateCSRFToken() (string, error) {
	return GetCSRFStore().GenerateToken()
}

// ValidateCSRFToken validates a CSRF token
func ValidateCSRFToken(token string) bool {
	return GetCSRFStore().ValidateToken(token)
}

// CSRFMiddleware creates middleware for CSRF protection
// It validates CSRF tokens on POST requests and generates tokens for GET requests
func CSRFMiddleware() gin.HandlerFunc {
	if csrfStore == nil {
		InitCSRFStore()
	}

	return func(c *gin.Context) {
		if c.Request.Method == "POST" {
			// Get token from form or header
			token := c.PostForm("csrf_token")
			if token == "" {
				token = c.GetHeader("X-CSRF-Token")
			}

			// Validate token
			if token == "" || !ValidateCSRFToken(token) {
				c.HTML(http.StatusForbidden, "login.html", gin.H{
					"error":           "Invalid or expired security token. Please refresh the page and try again.",
					"supabaseEnabled": false,
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// SetCSRFToken sets a new CSRF token in the context for use in templates
func SetCSRFToken(c *gin.Context) string {
	token, err := GenerateCSRFToken()
	if err != nil {
		return ""
	}
	c.Set("csrf_token", token)
	return token
}

// SecureCompare performs constant-time string comparison
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
