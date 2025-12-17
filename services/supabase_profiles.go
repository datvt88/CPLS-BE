package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// UserProfile represents a user profile from the profiles table
type UserProfile struct {
	ID                   string     `json:"id"`
	Email                string     `json:"email"`
	PhoneNumber          string     `json:"phone_number"`
	FullName             string     `json:"full_name"`
	Nickname             string     `json:"nickname"`
	StockAccountNumber   string     `json:"stock_account_number"`
	AvatarURL            string     `json:"avatar_url"`
	ZaloID               string     `json:"zalo_id"`
	Birthday             string     `json:"birthday"`
	Gender               string     `json:"gender"`
	Provider             string     `json:"provider"`
	ProviderID           string     `json:"provider_id"`
	Membership           string     `json:"membership"` // free, basic, premium, enterprise
	MembershipExpiresAt  *time.Time `json:"membership_expires_at"`
	TcbsAPIKey           string     `json:"tcbs_api_key"`
	TcbsConnectedAt      *time.Time `json:"tcbs_connected_at"`
	Role                 string     `json:"role"`
	IsActive             bool       `json:"is_active"`
	IsBanned             bool       `json:"is_banned"`
	BanReason            string     `json:"ban_reason"`
	LastLoginAt          *time.Time `json:"last_login_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// UserProfileInput is used for creating/updating user profiles
type UserProfileInput struct {
	Email                string `json:"email,omitempty"`
	PhoneNumber          string `json:"phone_number,omitempty"`
	FullName             string `json:"full_name,omitempty"`
	Nickname             string `json:"nickname,omitempty"`
	StockAccountNumber   string `json:"stock_account_number,omitempty"`
	AvatarURL            string `json:"avatar_url,omitempty"`
	ZaloID               string `json:"zalo_id,omitempty"`
	Birthday             string `json:"birthday,omitempty"`
	Gender               string `json:"gender,omitempty"`
	Provider             string `json:"provider,omitempty"`
	ProviderID           string `json:"provider_id,omitempty"`
	Membership           string `json:"membership,omitempty"` // free, basic, premium, enterprise
	MembershipExpiresAt  string `json:"membership_expires_at,omitempty"`
	TcbsAPIKey           string `json:"tcbs_api_key,omitempty"`
	Role                 string `json:"role,omitempty"`
	IsActive             *bool  `json:"is_active,omitempty"`
	IsBanned             *bool  `json:"is_banned,omitempty"`
	BanReason            string `json:"ban_reason,omitempty"`
}

// ProfilesListResponse contains paginated profiles results
type ProfilesListResponse struct {
	Profiles   []UserProfile `json:"profiles"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalPages int           `json:"total_pages"`
}

// SupabaseAuthUser represents a user in Supabase Auth
type SupabaseAuthUser struct {
	ID               string                 `json:"id"`
	Email            string                 `json:"email"`
	Phone            string                 `json:"phone"`
	EmailConfirmedAt *time.Time             `json:"email_confirmed_at"`
	PhoneConfirmedAt *time.Time             `json:"phone_confirmed_at"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	LastSignInAt     *time.Time             `json:"last_sign_in_at"`
	UserMetadata     map[string]interface{} `json:"user_metadata"`
	AppMetadata      map[string]interface{} `json:"app_metadata"`
}

// SupabaseAuthUserList is the response from listing auth users
type SupabaseAuthUserList struct {
	Users []SupabaseAuthUser `json:"users"`
}

// GetProfiles fetches profiles with pagination and optional search
func (c *SupabaseDBClient) GetProfiles(page, pageSize int, search, sortBy, sortOrder string) (*ProfilesListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// Build query URL
	queryURL := fmt.Sprintf("%s/rest/v1/profiles?select=*&order=%s.%s&limit=%d&offset=%d",
		c.URL, sortBy, sortOrder, pageSize, offset)

	// Add search filter if provided
	if search != "" {
		searchFilter := fmt.Sprintf("&or=(email.ilike.%%%s%%,full_name.ilike.%%%s%%,nickname.ilike.%%%s%%,phone_number.ilike.%%%s%%)",
			url.QueryEscape(search), url.QueryEscape(search), url.QueryEscape(search), url.QueryEscape(search))
		queryURL += searchFilter
	}

	req, err := http.NewRequest("GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.getAPIKey())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "count=exact")

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

	var profiles []UserProfile
	if err := json.Unmarshal(body, &profiles); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Get total count from Content-Range header
	var total int64
	contentRange := resp.Header.Get("Content-Range")
	if contentRange != "" {
		fmt.Sscanf(contentRange, "*/%d", &total)
		if total == 0 {
			var start, end int64
			fmt.Sscanf(contentRange, "%d-%d/%d", &start, &end, &total)
		}
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &ProfilesListResponse{
		Profiles:   profiles,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetProfileByID fetches a single profile by ID
func (c *SupabaseDBClient) GetProfileByID(id string) (*UserProfile, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/profiles?id=eq.%s&limit=1", c.URL, url.QueryEscape(id))

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

	var profiles []UserProfile
	if err := json.Unmarshal(body, &profiles); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(profiles) == 0 {
		return nil, errors.New("profile not found")
	}

	return &profiles[0], nil
}

// GetProfileByEmail fetches a profile by email
func (c *SupabaseDBClient) GetProfileByEmail(email string) (*UserProfile, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/profiles?email=eq.%s&limit=1", c.URL, url.QueryEscape(email))

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

	var profiles []UserProfile
	if err := json.Unmarshal(body, &profiles); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(profiles) == 0 {
		return nil, errors.New("profile not found")
	}

	return &profiles[0], nil
}

// UpdateProfile updates an existing profile
func (c *SupabaseDBClient) UpdateProfile(id string, input *UserProfileInput) (*UserProfile, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/profiles?id=eq.%s", c.URL, url.QueryEscape(id))

	// Add updated_at timestamp
	updateData := make(map[string]interface{})
	if input.Email != "" {
		updateData["email"] = input.Email
	}
	if input.PhoneNumber != "" {
		updateData["phone_number"] = input.PhoneNumber
	}
	if input.FullName != "" {
		updateData["full_name"] = input.FullName
	}
	if input.Nickname != "" {
		updateData["nickname"] = input.Nickname
	}
	if input.AvatarURL != "" {
		updateData["avatar_url"] = input.AvatarURL
	}
	if input.StockAccountNumber != "" {
		updateData["stock_account_number"] = input.StockAccountNumber
	}
	if input.ZaloID != "" {
		updateData["zalo_id"] = input.ZaloID
	}
	if input.Birthday != "" {
		updateData["birthday"] = input.Birthday
	}
	if input.Gender != "" {
		updateData["gender"] = input.Gender
	}
	if input.Provider != "" {
		updateData["provider"] = input.Provider
	}
	if input.ProviderID != "" {
		updateData["provider_id"] = input.ProviderID
	}
	if input.Membership != "" {
		updateData["membership"] = input.Membership
	}
	if input.MembershipExpiresAt != "" {
		updateData["membership_expires_at"] = input.MembershipExpiresAt
	}
	if input.TcbsAPIKey != "" {
		updateData["tcbs_api_key"] = input.TcbsAPIKey
	}
	if input.Role != "" {
		updateData["role"] = input.Role
	}
	if input.IsActive != nil {
		updateData["is_active"] = *input.IsActive
	}
	if input.IsBanned != nil {
		updateData["is_banned"] = *input.IsBanned
	}
	if input.BanReason != "" {
		updateData["ban_reason"] = input.BanReason
	}
	updateData["updated_at"] = time.Now().UTC().Format(time.RFC3339)

	payload, err := json.Marshal(updateData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal update data: %w", err)
	}

	req, err := http.NewRequest("PATCH", queryURL, bytes.NewReader(payload))
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

	var profiles []UserProfile
	if err := json.Unmarshal(body, &profiles); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(profiles) == 0 {
		return nil, errors.New("profile not found after update")
	}

	return &profiles[0], nil
}

// CreateProfile creates a new profile (usually linked to Supabase Auth user)
func (c *SupabaseDBClient) CreateProfile(id string, input *UserProfileInput) (*UserProfile, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/profiles", c.URL)

	profileData := map[string]interface{}{
		"id":           id,
		"email":        input.Email,
		"phone_number": input.PhoneNumber,
		"full_name":    input.FullName,
		"nickname":     input.Nickname,
		"membership":   "free",
		"role":         "user",
		"provider":     "email",
		"is_active":    true,
		"is_banned":    false,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"updated_at":   time.Now().UTC().Format(time.RFC3339),
	}

	if input.Membership != "" {
		profileData["membership"] = input.Membership
	}
	if input.Role != "" {
		profileData["role"] = input.Role
	}

	payload, err := json.Marshal(profileData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal profile data: %w", err)
	}

	req, err := http.NewRequest("POST", queryURL, bytes.NewReader(payload))
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

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	var profiles []UserProfile
	if err := json.Unmarshal(body, &profiles); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(profiles) == 0 {
		return nil, errors.New("failed to create profile")
	}

	return &profiles[0], nil
}

// DeleteProfile deletes a profile by ID
func (c *SupabaseDBClient) DeleteProfile(id string) error {
	queryURL := fmt.Sprintf("%s/rest/v1/profiles?id=eq.%s", c.URL, url.QueryEscape(id))

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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetProfileCount returns the total count of profiles
func (c *SupabaseDBClient) GetProfileCount() (int64, error) {
	queryURL := fmt.Sprintf("%s/rest/v1/profiles?select=count", c.URL)

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

	var total int64
	contentRange := resp.Header.Get("Content-Range")
	if contentRange != "" {
		fmt.Sscanf(contentRange, "*/%d", &total)
		if total == 0 {
			var start, end int64
			fmt.Sscanf(contentRange, "%d-%d/%d", &start, &end, &total)
		}
	}

	return total, nil
}

// BanUser bans a user with a reason
func (c *SupabaseDBClient) BanUser(id, reason string) error {
	isBanned := true
	_, err := c.UpdateProfile(id, &UserProfileInput{
		IsBanned:  &isBanned,
		BanReason: reason,
	})
	return err
}

// UnbanUser unbans a user
func (c *SupabaseDBClient) UnbanUser(id string) error {
	isBanned := false
	_, err := c.UpdateProfile(id, &UserProfileInput{
		IsBanned:  &isBanned,
		BanReason: "",
	})
	return err
}

// UpdateSubscription updates user's membership plan
func (c *SupabaseDBClient) UpdateSubscription(id, plan string, endDate *time.Time) error {
	input := &UserProfileInput{
		Membership: plan,
	}
	if endDate != nil {
		input.MembershipExpiresAt = endDate.Format(time.RFC3339)
	}
	_, err := c.UpdateProfile(id, input)
	return err
}

// === Supabase Auth Admin API Functions ===

// CreateAuthUser creates a new user in Supabase Auth
func (c *SupabaseDBClient) CreateAuthUser(email, password string, metadata map[string]interface{}) (*SupabaseAuthUser, error) {
	if c.ServiceKey == "" {
		return nil, errors.New("service key required for admin operations")
	}

	authURL := fmt.Sprintf("%s/auth/v1/admin/users", c.URL)

	userData := map[string]interface{}{
		"email":         email,
		"password":      password,
		"email_confirm": true,
	}

	if metadata != nil {
		userData["user_metadata"] = metadata
	}

	payload, err := json.Marshal(userData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user data: %w", err)
	}

	req, err := http.NewRequest("POST", authURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.ServiceKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.ServiceKey))
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("supabase auth error (status %d): %s", resp.StatusCode, string(body))
	}

	var authUser SupabaseAuthUser
	if err := json.Unmarshal(body, &authUser); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &authUser, nil
}

// DeleteAuthUser deletes a user from Supabase Auth
func (c *SupabaseDBClient) DeleteAuthUser(id string) error {
	if c.ServiceKey == "" {
		return errors.New("service key required for admin operations")
	}

	authURL := fmt.Sprintf("%s/auth/v1/admin/users/%s", c.URL, url.PathEscape(id))

	req, err := http.NewRequest("DELETE", authURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.ServiceKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.ServiceKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase auth error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetAuthUser gets a user from Supabase Auth
func (c *SupabaseDBClient) GetAuthUser(id string) (*SupabaseAuthUser, error) {
	if c.ServiceKey == "" {
		return nil, errors.New("service key required for admin operations")
	}

	authURL := fmt.Sprintf("%s/auth/v1/admin/users/%s", c.URL, url.PathEscape(id))

	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.ServiceKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.ServiceKey))

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
		return nil, fmt.Errorf("supabase auth error (status %d): %s", resp.StatusCode, string(body))
	}

	var authUser SupabaseAuthUser
	if err := json.Unmarshal(body, &authUser); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &authUser, nil
}

// UpdateAuthUserPassword updates a user's password in Supabase Auth
func (c *SupabaseDBClient) UpdateAuthUserPassword(id, newPassword string) error {
	if c.ServiceKey == "" {
		return errors.New("service key required for admin operations")
	}

	authURL := fmt.Sprintf("%s/auth/v1/admin/users/%s", c.URL, url.PathEscape(id))

	userData := map[string]interface{}{
		"password": newPassword,
	}

	payload, err := json.Marshal(userData)
	if err != nil {
		return fmt.Errorf("failed to marshal user data: %w", err)
	}

	req, err := http.NewRequest("PUT", authURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.ServiceKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.ServiceKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase auth error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// ListAuthUsers lists all users from Supabase Auth
func (c *SupabaseDBClient) ListAuthUsers(page, perPage int) (*SupabaseAuthUserList, error) {
	if c.ServiceKey == "" {
		return nil, errors.New("service key required for admin operations")
	}

	authURL := fmt.Sprintf("%s/auth/v1/admin/users?page=%d&per_page=%d", c.URL, page, perPage)

	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.ServiceKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.ServiceKey))

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
		return nil, fmt.Errorf("supabase auth error (status %d): %s", resp.StatusCode, string(body))
	}

	var userList SupabaseAuthUserList
	if err := json.Unmarshal(body, &userList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &userList, nil
}

// GeneratePasswordResetLink generates a password reset link for a user
func (c *SupabaseDBClient) GeneratePasswordResetLink(email string) (string, error) {
	if c.ServiceKey == "" {
		return "", errors.New("service key required for admin operations")
	}

	authURL := fmt.Sprintf("%s/auth/v1/admin/generate_link", c.URL)

	linkData := map[string]interface{}{
		"type":  "recovery",
		"email": email,
	}

	payload, err := json.Marshal(linkData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal link data: %w", err)
	}

	req, err := http.NewRequest("POST", authURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", c.ServiceKey)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.ServiceKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("supabase auth error (status %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if link, ok := result["action_link"].(string); ok {
		return link, nil
	}

	return "", errors.New("failed to get reset link from response")
}

// SyncProfileWithAuth syncs profile data with Supabase Auth user
func (c *SupabaseDBClient) SyncProfileWithAuth(profileID string) (*UserProfile, error) {
	// Get auth user data
	authUser, err := c.GetAuthUser(profileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth user: %w", err)
	}

	// Update profile with auth data
	input := &UserProfileInput{
		Email: authUser.Email,
	}

	if authUser.Phone != "" {
		input.PhoneNumber = authUser.Phone
	}

	// Get metadata
	if fullName, ok := authUser.UserMetadata["full_name"].(string); ok && fullName != "" {
		input.FullName = fullName
	}
	if nickname, ok := authUser.UserMetadata["nickname"].(string); ok && nickname != "" {
		input.Nickname = nickname
	}
	if avatarURL, ok := authUser.UserMetadata["avatar_url"].(string); ok && avatarURL != "" {
		input.AvatarURL = avatarURL
	}

	return c.UpdateProfile(profileID, input)
}

// GetProfileStats returns statistics about user profiles
func (c *SupabaseDBClient) GetProfileStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get total count
	total, err := c.GetProfileCount()
	if err != nil {
		return nil, err
	}
	stats["total"] = total

	// Get active count
	activeURL := fmt.Sprintf("%s/rest/v1/profiles?is_active=eq.true&select=count", c.URL)
	activeReq, _ := http.NewRequest("GET", activeURL, nil)
	activeReq.Header.Set("apikey", c.getAPIKey())
	activeReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	activeReq.Header.Set("Prefer", "count=exact")

	activeResp, err := c.httpClient.Do(activeReq)
	if err == nil {
		defer activeResp.Body.Close()
		contentRange := activeResp.Header.Get("Content-Range")
		if contentRange != "" {
			var activeCount int64
			fmt.Sscanf(contentRange, "*/%d", &activeCount)
			stats["active"] = activeCount
		}
	}

	// Get banned count
	bannedURL := fmt.Sprintf("%s/rest/v1/profiles?is_banned=eq.true&select=count", c.URL)
	bannedReq, _ := http.NewRequest("GET", bannedURL, nil)
	bannedReq.Header.Set("apikey", c.getAPIKey())
	bannedReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.getAPIKey()))
	bannedReq.Header.Set("Prefer", "count=exact")

	bannedResp, err := c.httpClient.Do(bannedReq)
	if err == nil {
		defer bannedResp.Body.Close()
		contentRange := bannedResp.Header.Get("Content-Range")
		if contentRange != "" {
			var bannedCount int64
			fmt.Sscanf(contentRange, "*/%d", &bannedCount)
			stats["banned"] = bannedCount
		}
	}

	return stats, nil
}
