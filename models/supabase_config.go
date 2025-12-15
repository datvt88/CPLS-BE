package models

import (
	"fmt"
	"os"
)

// SupabaseConfig holds Supabase configuration
type SupabaseConfig struct {
	URL        string
	AnonKey    string
	ServiceKey string
	JWTSecret  string
}

// SupabaseClient provides methods for interacting with Supabase
type SupabaseClient struct {
	Config *SupabaseConfig
}

// NewSupabaseConfig creates a new Supabase configuration from environment variables
func NewSupabaseConfig() (*SupabaseConfig, error) {
	url := os.Getenv("SUPABASE_URL")
	anonKey := os.Getenv("SUPABASE_ANON_KEY")
	serviceKey := os.Getenv("SUPABASE_SERVICE_KEY")
	jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")

	if url == "" {
		return nil, fmt.Errorf("SUPABASE_URL is required")
	}

	return &SupabaseConfig{
		URL:        url,
		AnonKey:    anonKey,
		ServiceKey: serviceKey,
		JWTSecret:  jwtSecret,
	}, nil
}

// NewSupabaseClient creates a new Supabase client
func NewSupabaseClient(config *SupabaseConfig) *SupabaseClient {
	return &SupabaseClient{
		Config: config,
	}
}

// GetAuthURL returns the Supabase Auth URL
func (c *SupabaseClient) GetAuthURL() string {
	return fmt.Sprintf("%s/auth/v1", c.Config.URL)
}

// GetRESTURL returns the Supabase REST API URL
func (c *SupabaseClient) GetRESTURL() string {
	return fmt.Sprintf("%s/rest/v1", c.Config.URL)
}

// GetStorageURL returns the Supabase Storage URL
func (c *SupabaseClient) GetStorageURL() string {
	return fmt.Sprintf("%s/storage/v1", c.Config.URL)
}