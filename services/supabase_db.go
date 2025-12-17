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
