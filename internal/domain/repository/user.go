package repository

import (
	"context"
	"reminder/internal/domain/entity"
)

// UserRepository defines the interface for user data operations.
type UserRepository interface {
	// FindByUserID retrieves a user by their LINE User ID.
	FindByUserID(ctx context.Context, userID string) (*entity.User, error)
	// Create creates a new user session.
	Create(ctx context.Context, user *entity.User) error
	// Update updates an existing user session.
	Update(ctx context.Context, user *entity.User) error
	// Delete deletes a user session by their LINE User ID.
	Delete(ctx context.Context, userID string) error
}
