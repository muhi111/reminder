package service

import (
	"context"
	"reminder/internal/application/dto"
	"reminder/internal/domain/constant"
	"reminder/internal/domain/entity"
)

// UserService defines the interface for user-related business logic.
type UserService interface {
	// GetOrCreateUser finds a user by ID or creates a new one if not found.
	GetOrCreateUser(ctx context.Context, userID string) (*entity.User, error)
	// GetUser finds a user by ID. Returns error if not found.
	GetUser(ctx context.Context, userID string) (*entity.User, error)
	// UpdateStatus updates the status of a user.
	UpdateStatus(ctx context.Context, req dto.UpdateUserStatusRequest) error
	// UpdateLastNotify updates the last notified reminder ID for a user.
	UpdateLastNotify(ctx context.Context, req dto.UpdateUserLastNotifyRequest) error
	// DeleteUser handles the unfollow event, deleting user data.
	DeleteUser(ctx context.Context, userID string) error
	// GetUserStatus retrieves the current status of a user.
	GetUserStatus(ctx context.Context, userID string) (constant.UserStatus, error)
}
