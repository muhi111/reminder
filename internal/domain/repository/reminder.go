package repository

import (
	"context"
	"reminder/internal/domain/entity"
	"time"
)

// ReminderRepository defines the interface for reminder data operations.
type ReminderRepository interface {
	// FindByID retrieves a reminder by its ID.
	FindByID(ctx context.Context, id uint) (*entity.Reminder, error)
	// FindByUserID retrieves all reminders for a specific user.
	FindByUserID(ctx context.Context, userID string) ([]*entity.Reminder, error)
	// FindActiveByUserID retrieves active reminders (remind_time in the future) for a specific user.
	FindActiveByUserID(ctx context.Context, userID string) ([]*entity.Reminder, error)
	// FindByUserIDAndContent retrieves reminders by user ID and content.
	FindByUserIDAndContent(ctx context.Context, userID string, content string) ([]*entity.Reminder, error)
	// FindAll retrieves all reminders (used for rescheduling on startup).
	FindAll(ctx context.Context) ([]*entity.Reminder, error)
	// Create creates a new reminder. Returns the ID of the created reminder.
	Create(ctx context.Context, reminder *entity.Reminder) (uint, error)
	// Update updates an existing reminder.
	Update(ctx context.Context, reminder *entity.Reminder) error
	// Delete deletes a reminder by its ID.
	Delete(ctx context.Context, id uint) error
	// DeleteByUserID deletes all reminders for a specific user.
	DeleteByUserID(ctx context.Context, userID string) error
	// DeleteByUserIDAndContent deletes reminders by user ID and content.
	DeleteByUserIDAndContent(ctx context.Context, userID string, content string) error
	// DeleteOlderThan deletes reminders with remind_time older than the specified time.
	DeleteOlderThan(ctx context.Context, threshold time.Time) error
}
