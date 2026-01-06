package services

import (
	"fmt"
	"go_backend_project/models"
	"log"

	"gorm.io/gorm"
)

// UserService handles user management operations
type UserService struct {
	db             *gorm.DB
	supabaseClient *SupabaseDBClient
}

// NewUserService creates a new user service instance
func NewUserService(db *gorm.DB) *UserService {
	service := &UserService{
		db: db,
	}

	// Try to initialize Supabase client for profiles access
	if client, err := NewSupabaseDBClient(); err == nil {
		service.supabaseClient = client
		log.Println("UserService: Using Supabase client for profiles")
	} else {
		log.Printf("UserService: Supabase client not available: %v", err)
	}

	return service
}

// GetAdminUsers retrieves admin users from the admin_users table (public schema)
// This table contains admins who can access the admin panel
func (s *UserService) GetAdminUsers(limit, offset int) ([]models.AdminUser, int64, error) {
	var users []models.AdminUser
	var total int64

	// Count total
	if err := s.db.Model(&models.AdminUser{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count admin users: %w", err)
	}

	// Get users with pagination
	query := s.db.Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}

	if err := query.Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch admin users: %w", err)
	}

	return users, total, nil
}

// GetAdminUserByUsername retrieves an admin user by username
func (s *UserService) GetAdminUserByUsername(username string) (*models.AdminUser, error) {
	var user models.AdminUser
	if err := s.db.Where("username = ? AND is_active = ?", username, true).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("admin user not found: %s", username)
		}
		return nil, fmt.Errorf("failed to fetch admin user: %w", err)
	}
	return &user, nil
}

// GetAdminUserByID retrieves an admin user by ID
func (s *UserService) GetAdminUserByID(id uint) (*models.AdminUser, error) {
	var user models.AdminUser
	if err := s.db.First(&user, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("admin user not found: %d", id)
		}
		return nil, fmt.Errorf("failed to fetch admin user: %w", err)
	}
	return &user, nil
}

// CreateAdminUser creates a new admin user in the admin_users table
func (s *UserService) CreateAdminUser(user *models.AdminUser) error {
	if err := s.db.Create(user).Error; err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}
	return nil
}

// UpdateAdminUser updates an existing admin user
func (s *UserService) UpdateAdminUser(user *models.AdminUser) error {
	if err := s.db.Save(user).Error; err != nil {
		return fmt.Errorf("failed to update admin user: %w", err)
	}
	return nil
}

// GetAppUsers retrieves app users from Supabase profiles table (auth schema)
// This uses Supabase client to query the profiles table
func (s *UserService) GetAppUsers(page, pageSize int, search, sortBy, sortOrder string) (*ProfilesListResponse, error) {
	if s.supabaseClient == nil {
		// Fallback to local users table if Supabase is not available
		return s.getLocalUsers(page, pageSize)
	}

	// Use Supabase client to fetch profiles
	result, err := s.supabaseClient.GetProfiles(page, pageSize, search, sortBy, sortOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profiles from Supabase: %w", err)
	}

	return result, nil
}

// getLocalUsers is a fallback method to get users from local database
func (s *UserService) getLocalUsers(page, pageSize int) (*ProfilesListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	var users []models.User
	var total int64

	// Count total
	if err := s.db.Model(&models.User{}).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	// Get users with pagination
	if err := s.db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch users: %w", err)
	}

	// Convert to UserProfile format for consistency
	profiles := make([]UserProfile, len(users))
	for i, user := range users {
		profiles[i] = UserProfile{
			ID:          user.SupabaseUserID,
			Email:       user.Email,
			FullName:    user.FullName,
			PhoneNumber: user.Phone,
			Role:        user.Role,
			IsActive:    user.IsActive,
			CreatedAt:   user.CreatedAt,
			UpdatedAt:   user.UpdatedAt,
		}
		if user.LastLoginAt != nil {
			profiles[i].LastLoginAt = user.LastLoginAt
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

// GetUserBySupabaseID retrieves a user by their Supabase User ID
func (s *UserService) GetUserBySupabaseID(supabaseUserID string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("supabase_user_id = ?", supabaseUserID).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found: %s", supabaseUserID)
		}
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}
	return &user, nil
}

// SyncUserFromSupabase syncs a user from Supabase to local database
func (s *UserService) SyncUserFromSupabase(profile *UserProfile) (*models.User, error) {
	// Check if user already exists
	var existingUser models.User
	err := s.db.Where("supabase_user_id = ?", profile.ID).First(&existingUser).Error

	if err == gorm.ErrRecordNotFound {
		// Create new user
		newUser := &models.User{
			SupabaseUserID: profile.ID,
			Email:          profile.Email,
			FullName:       profile.FullName,
			Phone:          profile.PhoneNumber,
			Role:           profile.Role,
			IsActive:       profile.IsActive,
		}

		if err := s.db.Create(newUser).Error; err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}

		return newUser, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Update existing user
	existingUser.Email = profile.Email
	existingUser.FullName = profile.FullName
	existingUser.Phone = profile.PhoneNumber
	existingUser.Role = profile.Role
	existingUser.IsActive = profile.IsActive

	if err := s.db.Save(&existingUser).Error; err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return &existingUser, nil
}

// SearchAdminUsers searches admin users by username, email, or full name
func (s *UserService) SearchAdminUsers(query string, limit int) ([]models.AdminUser, error) {
	var users []models.AdminUser

	searchQuery := "%" + query + "%"
	err := s.db.Where(
		"username ILIKE ? OR email ILIKE ? OR full_name ILIKE ?",
		searchQuery, searchQuery, searchQuery,
	).Limit(limit).Find(&users).Error

	if err != nil {
		return nil, fmt.Errorf("failed to search admin users: %w", err)
	}

	return users, nil
}
