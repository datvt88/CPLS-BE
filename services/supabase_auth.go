package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// SupabaseAuthService handles authentication with Supabase
type SupabaseAuthService struct {
	URL        string
	AnonKey    string
	ServiceKey string
	httpClient *http.Client
}

// NewSupabaseAuthService creates a new Supabase auth service
func NewSupabaseAuthService() (*SupabaseAuthService, error) {
	url := os.Getenv("SUPABASE_URL")
	anonKey := os.Getenv("SUPABASE_ANON_KEY")
	serviceKey := os.Getenv("SUPABASE_SERVICE_KEY")

	if url == "" {
		return nil, errors.New("SUPABASE_URL is required")
	}

	return &SupabaseAuthService{
		URL:        url,
		AnonKey:    anonKey,
		ServiceKey: serviceKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// SupabaseAuthResponse represents the response from Supabase auth
type SupabaseAuthResponse struct {
	AccessToken  string       `json:"access_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int          `json:"expires_in"`
	ExpiresAt    int64        `json:"expires_at"`
	RefreshToken string       `json:"refresh_token"`
	User         SupabaseUser `json:"user"`
}

// SupabaseUser represents a Supabase user
type SupabaseUser struct {
	ID               string                 `json:"id"`
	Aud              string                 `json:"aud"`
	Role             string                 `json:"role"`
	Email            string                 `json:"email"`
	EmailConfirmedAt string                 `json:"email_confirmed_at"`
	Phone            string                 `json:"phone"`
	ConfirmedAt      string                 `json:"confirmed_at"`
	LastSignInAt     string                 `json:"last_sign_in_at"`
	AppMetadata      map[string]interface{} `json:"app_metadata"`
	UserMetadata     map[string]interface{} `json:"user_metadata"`
	Identities       []interface{}          `json:"identities"`
	CreatedAt        string                 `json:"created_at"`
	UpdatedAt        string                 `json:"updated_at"`
}

// SupabaseErrorResponse represents an error from Supabase
type SupabaseErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Message          string `json:"message"`
	Code             int    `json:"code"`
}

// SignInWithPassword authenticates a user with email and password
func (s *SupabaseAuthService) SignInWithPassword(email, password string) (*SupabaseAuthResponse, error) {
	payload := map[string]string{
		"email":    email,
		"password": password,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/auth/v1/token?grant_type=password", s.URL), bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.AnonKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp SupabaseErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return nil, fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(respBody))
		}
		if errResp.ErrorDescription != "" {
			return nil, errors.New(errResp.ErrorDescription)
		}
		if errResp.Message != "" {
			return nil, errors.New(errResp.Message)
		}
		return nil, errors.New(errResp.Error)
	}

	var authResp SupabaseAuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &authResp, nil
}

// GetUser retrieves user information using an access token
func (s *SupabaseAuthService) GetUser(accessToken string) (*SupabaseUser, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/auth/v1/user", s.URL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("apikey", s.AnonKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user: %s", string(body))
	}

	var user SupabaseUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to parse user: %w", err)
	}

	return &user, nil
}

// SignOut signs out a user
func (s *SupabaseAuthService) SignOut(accessToken string) error {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/auth/v1/logout", s.URL), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("apikey", s.AnonKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to sign out: %s", string(body))
	}

	return nil
}

// RefreshToken refreshes an access token using a refresh token
func (s *SupabaseAuthService) RefreshToken(refreshToken string) (*SupabaseAuthResponse, error) {
	payload := map[string]string{
		"refresh_token": refreshToken,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/auth/v1/token?grant_type=refresh_token", s.URL), bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.AnonKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to refresh token: %s", string(body))
	}

	var authResp SupabaseAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &authResp, nil
}

// IsAdmin checks if a user has admin role in app_metadata
func (s *SupabaseAuthService) IsAdmin(user *SupabaseUser) bool {
	if user == nil || user.AppMetadata == nil {
		return false
	}
	role, ok := user.AppMetadata["role"].(string)
	if !ok {
		return false
	}
	return role == "admin" || role == "superadmin"
}

// SetUserRole sets the role in a user's app_metadata (requires service key)
func (s *SupabaseAuthService) SetUserRole(userID string, role string) error {
	if s.ServiceKey == "" {
		return errors.New("SUPABASE_SERVICE_KEY is required for admin operations")
	}

	payload := map[string]interface{}{
		"app_metadata": map[string]interface{}{
			"role": role,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/auth/v1/admin/users/%s", s.URL, userID), bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.ServiceKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.ServiceKey))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to set user role: %s", string(body))
	}

	return nil
}

// CreateUser creates a new user (requires service key)
func (s *SupabaseAuthService) CreateUser(email, password string, appMetadata map[string]interface{}) (*SupabaseUser, error) {
	if s.ServiceKey == "" {
		return nil, errors.New("SUPABASE_SERVICE_KEY is required for admin operations")
	}

	payload := map[string]interface{}{
		"email":        email,
		"password":     password,
		"app_metadata": appMetadata,
		"email_confirm": true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/auth/v1/admin/users", s.URL), bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.ServiceKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.ServiceKey))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create user: %s", string(body))
	}

	var user SupabaseUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to parse user: %w", err)
	}

	return &user, nil
}
