package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LoginAttempt tracks login attempts from an IP
type LoginAttempt struct {
	Count    int
	FirstAt  time.Time
	LockedAt time.Time
	IsLocked bool
}

// RateLimiter manages rate limiting for login attempts
type RateLimiter struct {
	mu           sync.RWMutex
	attempts     map[string]*LoginAttempt
	maxAttempts  int
	windowPeriod time.Duration
	lockDuration time.Duration
}

// Global rate limiter instance
var loginRateLimiter *RateLimiter

// InitLoginRateLimiter initializes the global login rate limiter
func InitLoginRateLimiter() {
	loginRateLimiter = NewRateLimiter(5, 15*time.Minute, 30*time.Minute)
	// Start cleanup goroutine
	go loginRateLimiter.startCleanup()
}

// NewRateLimiter creates a new rate limiter
// maxAttempts: maximum login attempts allowed within the window
// windowPeriod: time window for counting attempts
// lockDuration: how long to lock the IP after max attempts exceeded
func NewRateLimiter(maxAttempts int, windowPeriod, lockDuration time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts:     make(map[string]*LoginAttempt),
		maxAttempts:  maxAttempts,
		windowPeriod: windowPeriod,
		lockDuration: lockDuration,
	}
}

// startCleanup periodically cleans up old entries
func (rl *RateLimiter) startCleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		rl.cleanup()
	}
}

// cleanup removes expired entries
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, attempt := range rl.attempts {
		// Remove if lock has expired and window has passed
		if attempt.IsLocked {
			if now.Sub(attempt.LockedAt) > rl.lockDuration {
				delete(rl.attempts, ip)
			}
		} else if now.Sub(attempt.FirstAt) > rl.windowPeriod {
			delete(rl.attempts, ip)
		}
	}
}

// Check checks if an IP is allowed to attempt login
func (rl *RateLimiter) Check(ip string) (bool, int, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	attempt, exists := rl.attempts[ip]

	if !exists {
		return true, rl.maxAttempts, 0
	}

	// Check if locked
	if attempt.IsLocked {
		remaining := rl.lockDuration - now.Sub(attempt.LockedAt)
		if remaining > 0 {
			return false, 0, remaining
		}
		// Lock expired, reset
		delete(rl.attempts, ip)
		return true, rl.maxAttempts, 0
	}

	// Check if window expired
	if now.Sub(attempt.FirstAt) > rl.windowPeriod {
		delete(rl.attempts, ip)
		return true, rl.maxAttempts, 0
	}

	attemptsRemaining := rl.maxAttempts - attempt.Count
	if attemptsRemaining <= 0 {
		return false, 0, rl.windowPeriod - now.Sub(attempt.FirstAt)
	}

	return true, attemptsRemaining, 0
}

// RecordAttempt records a login attempt for an IP
func (rl *RateLimiter) RecordAttempt(ip string, success bool) {
	if success {
		// Successful login, clear attempts
		rl.mu.Lock()
		delete(rl.attempts, ip)
		rl.mu.Unlock()
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	attempt, exists := rl.attempts[ip]

	if !exists {
		rl.attempts[ip] = &LoginAttempt{
			Count:   1,
			FirstAt: now,
		}
		return
	}

	// Check if window expired
	if now.Sub(attempt.FirstAt) > rl.windowPeriod {
		rl.attempts[ip] = &LoginAttempt{
			Count:   1,
			FirstAt: now,
		}
		return
	}

	attempt.Count++

	// Check if should lock
	if attempt.Count >= rl.maxAttempts {
		attempt.IsLocked = true
		attempt.LockedAt = now
	}
}

// GetRemainingAttempts returns remaining attempts for an IP
func (rl *RateLimiter) GetRemainingAttempts(ip string) int {
	_, remaining, _ := rl.Check(ip)
	return remaining
}

// LoginRateLimitMiddleware creates a middleware for rate limiting login attempts
func LoginRateLimitMiddleware() gin.HandlerFunc {
	// Ensure rate limiter is initialized
	if loginRateLimiter == nil {
		InitLoginRateLimiter()
	}

	return func(c *gin.Context) {
		// Only apply to POST requests (actual login attempts)
		if c.Request.Method != "POST" {
			c.Next()
			return
		}

		ip := c.ClientIP()
		allowed, remaining, lockDuration := loginRateLimiter.Check(ip)

		// Set headers for client awareness
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if !allowed {
			minutes := int(lockDuration.Minutes())
			seconds := int(lockDuration.Seconds()) % 60

			c.HTML(http.StatusTooManyRequests, "login.html", gin.H{
				"error":           formatRateLimitError(minutes, seconds),
				"supabaseEnabled": false,
				"rateLimited":     true,
				"retryAfter":      int(lockDuration.Seconds()),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// formatRateLimitError formats the rate limit error message
func formatRateLimitError(minutes, seconds int) string {
	if minutes > 0 {
		return fmt.Sprintf("Too many failed login attempts. Please try again in %d minute(s) and %d second(s).", minutes, seconds)
	}
	return fmt.Sprintf("Too many failed login attempts. Please try again in %d second(s).", seconds)
}

// RecordLoginAttempt records a login attempt from auth controller
func RecordLoginAttempt(ip string, success bool) {
	if loginRateLimiter == nil {
		InitLoginRateLimiter()
	}
	loginRateLimiter.RecordAttempt(ip, success)
}

// GetLoginRateLimiter returns the global login rate limiter
func GetLoginRateLimiter() *RateLimiter {
	if loginRateLimiter == nil {
		InitLoginRateLimiter()
	}
	return loginRateLimiter
}
