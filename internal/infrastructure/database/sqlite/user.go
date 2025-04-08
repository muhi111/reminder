package sqlite

import (
	"context"
	"errors"
	"fmt"
	"reminder/internal/domain/entity"
	"reminder/internal/domain/repository"

	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new instance of UserRepository.
func NewUserRepository(db *gorm.DB) repository.UserRepository {
	return &userRepository{db: db}
}

// FindByUserID retrieves a user by their LINE User ID.
func (r *userRepository) FindByUserID(ctx context.Context, userID string) (*entity.User, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user with ID %s not found: %w", userID, err)
		}
		return nil, fmt.Errorf("🔴 ERROR: failed to find user by user_id %s: %w", userID, err)
	}
	return &user, nil
}

// Create creates a new user session.
func (r *userRepository) Create(ctx context.Context, user *entity.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("🔴 ERROR: failed to create user %s: %w", user.ID, err)
	}
	return nil
}

// Update updates an existing user session.
func (r *userRepository) Update(ctx context.Context, user *entity.User) error {
	// Use Save to update all fields, including zero values
	if err := r.db.WithContext(ctx).Save(user).Error; err != nil {
		return fmt.Errorf("🔴 ERROR: failed to update user %s: %w", user.ID, err)
	}
	return nil
}

// Delete deletes a user session by their LINE User ID.
func (r *userRepository) Delete(ctx context.Context, userID string) error {
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&entity.User{}).Error; err != nil {
		return fmt.Errorf("🔴 ERROR: failed to delete user %s: %w", userID, err)
	}
	return nil
}
