package service

import (
	"context"
	"errors"
	"fmt"
	"reminder/internal/application/dto"
	"reminder/internal/domain/constant"
	"reminder/internal/domain/entity"
	"reminder/internal/domain/repository"
	appErrors "reminder/internal/pkg/errors" // Alias to avoid collision
	"reminder/internal/pkg/logger"

	"gorm.io/gorm"
)

type userService struct {
	userRepo     repository.UserRepository
	reminderRepo repository.ReminderRepository // Needed for deleting reminders on unfollow
	log          logger.Logger
}

// NewUserService creates a new instance of UserService implementation.
func NewUserService(userRepo repository.UserRepository, reminderRepo repository.ReminderRepository, log logger.Logger) UserService {
	return &userService{
		userRepo:     userRepo,
		reminderRepo: reminderRepo,
		log:          log,
	}
}

// GetOrCreateUser finds a user by ID or creates a new one if not found.
func (s *userService) GetOrCreateUser(ctx context.Context, userID string) (*entity.User, error) {
	user, err := s.userRepo.FindByUserID(ctx, userID)
	if err != nil {
		// Check if the error is specifically "record not found"
		// The repository implementation returns a wrapped error, so we check the underlying cause.
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(errors.Unwrap(err), gorm.ErrRecordNotFound) {
			s.log.Info(fmt.Sprintf("User %s not found, creating new user.", userID))
			newUser := &entity.User{
				ID:     userID,
				Status: constant.StatusInitial.Int(), // Default status
			}
			if createErr := s.userRepo.Create(ctx, newUser); createErr != nil {
				s.log.Error("Failed to create user", createErr)
				return nil, fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, createErr)
			}
			return newUser, nil
		}
		// Other database error
		s.log.Error(fmt.Sprintf("Failed to find user %s", userID), err)
		return nil, fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}
	s.log.Debug(fmt.Sprintf("Found existing user %s", userID))
	return user, nil
}

// GetUser finds a user by ID. Returns error if not found.
func (s *userService) GetUser(ctx context.Context, userID string) (*entity.User, error) {
	user, err := s.userRepo.FindByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(errors.Unwrap(err), gorm.ErrRecordNotFound) {
			return nil, appErrors.ErrUserNotFound // Return specific app error
		}
		s.log.Error(fmt.Sprintf("Failed to get user %s", userID), err)
		return nil, fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}
	return user, nil
}

// UpdateStatus updates the status of a user.
func (s *userService) UpdateStatus(ctx context.Context, req dto.UpdateUserStatusRequest) error {
	user, err := s.GetUser(ctx, req.UserID) // Use GetUser to ensure user exists
	if err != nil {
		return err // Return ErrUserNotFound or ErrDatabaseOperation
	}

	user.SetStatus(req.Status)
	user.TextID = req.TextID // Update TextID if provided

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}
	s.log.Debug(fmt.Sprintf("Updated status for user %s to %d", req.UserID, req.Status))
	return nil
}

// UpdateLastNotify updates the last notified reminder ID for a user.
func (s *userService) UpdateLastNotify(ctx context.Context, req dto.UpdateUserLastNotifyRequest) error {
	user, err := s.GetUser(ctx, req.UserID)
	if err != nil {
		return err
	}

	user.LastNotifyTextID = &req.LastNotifyTextID

	if err := s.userRepo.Update(ctx, user); err != nil {
		s.log.Error(fmt.Sprintf("Failed to update last notify ID for user %s", req.UserID), err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}
	s.log.Debug(fmt.Sprintf("Updated last notify ID for user %s", req.UserID))
	return nil
}

// DeleteUser handles the unfollow event, deleting user data.
func (s *userService) DeleteUser(ctx context.Context, userID string) error {
	// Delete reminders first
	if err := s.reminderRepo.DeleteByUserID(ctx, userID); err != nil {
		// Log error but continue to delete user session if possible
		s.log.Error(fmt.Sprintf("Failed to delete reminders for user %s during unfollow", userID), err)
		// Don't return here, try to delete the user session anyway
	} else {
		s.log.Info(fmt.Sprintf("Deleted reminders for user %s due to unfollow.", userID))
	}

	// Delete user session
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		s.log.Error(fmt.Sprintf("Failed to delete user session for user %s during unfollow", userID), err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	s.log.Info(fmt.Sprintf("Deleted user session for user %s due to unfollow.", userID))
	return nil
}

// GetUserStatus retrieves the current status of a user.
func (s *userService) GetUserStatus(ctx context.Context, userID string) (constant.UserStatus, error) {
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return constant.StatusInitial, err // Return default status and error
	}
	return user.GetStatus(), nil
}
