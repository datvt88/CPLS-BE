package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// SupabaseDBClient handles database operations via Supabase REST API
type SupabaseDBClient struct {
	URL        string
	AnonKey    string
	ServiceKey string
	httpClient *http.Client
}

// AdminUserRecord represents an admin user from the database
type AdminUserRecord struct {
	ID           int        `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"password_hash"`
	Email        string     `json:"email"`
	FullName     string     `json:"full_name"`
	Role         string     `json:"role"`
	IsActive     bool       `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// NewSupabaseDBClient creates a new Supabase database client
func NewSupabaseDBClient() (*SupabaseDBClient, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	anonKey := os.Getenv("SUPABASE_ANON_KEY")
	serviceKey := os.Getenv("SUPABASE_SERVICE_KEY")

	if supabaseURL == "" {
		return nil, errors.New("SUPABASE_URL is required")
	}
	if anonKey == "" && serviceKey == "" {
		return nil, errors.New("SUPABASE_ANON_KEY or SUPABASE_SERVICE_KEY is required")
	}

	return &SupabaseDBClient{
		URL:        supabaseURL,
		AnonKey:    anonKey,
		ServiceKey: serviceKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// getAPIKey returns the best available API key (service key preferred)
func (c *SupabaseDBClient) getAPIKey() string {
	if c.ServiceKey != "" {
		return c.ServiceKey
	}
	return c.AnonKey
}

// GetAdminUserByUsername fetches an admin user by username from Supabase
func (c *SupabaseDBClient) GetAdminUserByUsername(username string) (*AdminUserRecord, error) {
	// Build query URL - using PostgREST syntax
	queryURL := fmt.Sprintf("%s/rest/v1/admin_users?username=eq.%s&is_active=eq.true&limit=1",
		c.URL, url.QueryEscape(username))

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers for Supabase REST API
	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	var users []AdminUserRecord
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(users) == 0 {
		return nil, errors.New("user not found")
	}

	return &users[0], nil
}

// GetAdminUserByEmail fetches an admin user by email from Supabase
func (c *SupabaseDBClient) GetAdminUserByEmail(email string) (*AdminUserRecord, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/admin_users?email=eq.%s&is_active=eq.true&limit=1",
		c.URL, url.QueryEscape(email))

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	var users []AdminUserRecord
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(users) == 0 {
		return nil, errors.New("user not found")
	}

	return &users[0], nil
}

// CheckPassword verifies the password against the stored hash
func (u *AdminUserRecord) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}

// UpdateLastLogin updates the last login timestamp for a user
func (c *SupabaseDBClient) UpdateLastLogin(userID int) error {
	queryURL := fmt.Sprintf("%s/rest/v1/admin_users?id=eq.%d", c.URL, userID)

	// PATCH request to update last_login_at
	payload := `{"last_login_at": "` + time.Now().UTC().Format(time.RFC3339) + `"}`

	req, err := http.NewRequest("PATCH", queryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	// Set body
	req.Body = io.NopCloser(strings.NewReader(payload))
	req.ContentLength = int64(len(payload))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update last login (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// TestConnection tests the connection to Supabase
func (c *SupabaseDBClient) TestConnection() error {
	// Try to query admin_users table with limit 0 just to test connection
	queryURL := fmt.Sprintf("%s/rest/v1/admin_users?limit=0", c.URL)

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Supabase: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase connection test failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// AdminSessionRecord represents an admin session in the database
// This matches the Supabase admin_sessions table structure
type AdminSessionRecord struct {
	ID        int       `json:"id"`
	AdminUser int       `json:"admin_user"` // Foreign key to admin_users.id
	Token     string    `json:"token"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	ExpiresAt time.Time `json:"expires_at"`
	// These fields are populated from admin_users table (not stored in sessions)
	Username string `json:"-"`
	Email    string `json:"-"`
	FullName string `json:"-"`
	Role     string `json:"-"`
	UserID   int    `json:"-"` // Alias for AdminUser
}

// CreateAdminSession creates a new admin session in Supabase
func (c *SupabaseDBClient) CreateAdminSession(session *AdminSessionRecord) error {
	queryURL := fmt.Sprintf("%s/rest/v1/admin_sessions", c.URL)

	// Only include fields that exist in the admin_sessions table
	payload := fmt.Sprintf(`{
		"token": "%s",
		"admin_user": %d,
		"ip_address": "%s",
		"user_agent": "%s",
		"expires_at": "%s"
	}`, session.Token, session.AdminUser, session.IPAddress,
		escapeJSON(session.UserAgent), session.ExpiresAt.UTC().Format(time.RFC3339))

	req, err := http.NewRequest("POST", queryURL, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create session (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// escapeJSON escapes special characters for JSON string
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// GetAdminSessionByToken retrieves an admin session by token and populates user info
func (c *SupabaseDBClient) GetAdminSessionByToken(token string) (*AdminSessionRecord, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/admin_sessions?token=eq.%s&limit=1",
		c.URL, url.QueryEscape(token))

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	var sessions []AdminSessionRecord
	if err := json.Unmarshal(body, &sessions); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(sessions) == 0 {
		return nil, errors.New("session not found")
	}

	session := &sessions[0]

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		// Delete expired session
		c.DeleteAdminSession(token)
		return nil, errors.New("session expired")
	}

	// Set UserID alias
	session.UserID = session.AdminUser

	// Fetch user info from admin_users table
	user, err := c.GetAdminUserByID(session.AdminUser)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Populate user fields
	session.Username = user.Username
	session.Email = user.Email
	session.FullName = user.FullName
	session.Role = user.Role

	return session, nil
}

// GetAdminUserByID fetches an admin user by ID from Supabase
func (c *SupabaseDBClient) GetAdminUserByID(userID int) (*AdminUserRecord, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/admin_users?id=eq.%d&limit=1", c.URL, userID)

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	var users []AdminUserRecord
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(users) == 0 {
		return nil, errors.New("user not found")
	}

	return &users[0], nil
}

// DeleteAdminSession deletes an admin session by token
func (c *SupabaseDBClient) DeleteAdminSession(token string) error {
	queryURL := fmt.Sprintf("%s/rest/v1/admin_sessions?token=eq.%s",
		c.URL, url.QueryEscape(token))

	req, err := http.NewRequest("DELETE", queryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// ExtendAdminSession extends the session expiration time
func (c *SupabaseDBClient) ExtendAdminSession(token string, newExpiry time.Time) error {
	queryURL := fmt.Sprintf("%s/rest/v1/admin_sessions?token=eq.%s",
		c.URL, url.QueryEscape(token))

	payload := fmt.Sprintf(`{"expires_at": "%s"}`, newExpiry.UTC().Format(time.RFC3339))

	req, err := http.NewRequest("PATCH", queryURL, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=minimal")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// CleanupExpiredSessions removes all expired sessions from the database
func (c *SupabaseDBClient) CleanupExpiredSessions() error {
	now := time.Now().UTC().Format(time.RFC3339)
	queryURL := fmt.Sprintf("%s/rest/v1/admin_sessions?expires_at=lt.%s",
		c.URL, url.QueryEscape(now))

	req, err := http.NewRequest("DELETE", queryURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// GetAdminUserCount returns the count of admin users
func (c *SupabaseDBClient) GetAdminUserCount() (int64, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/admin_users?select=count", c.URL)

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Prefer", "count=exact")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Get count from Content-Range header
	contentRange := resp.Header.Get("Content-Range")
	if contentRange != "" {
		var count int64
		// Format: "0-0/5" or "*/5"
		fmt.Sscanf(contentRange, "*/%d", &count)
		if count > 0 {
			return count, nil
		}
		// Try alternate format
		var start, end int64
		fmt.Sscanf(contentRange, "%d-%d/%d", &start, &end, &count)
		return count, nil
	}

	return 0, nil
}
