package service

import (
	"context"
	"reminder/internal/application/dto"
	"reminder/internal/domain/entity"
)

// ReminderService defines the interface for reminder-related business logic.
type ReminderService interface {
	// CreateReminder creates a new reminder entry (initially without a time).
	// It returns the ID of the newly created reminder.
	CreateReminder(ctx context.Context, req dto.CreateReminderRequest) (uint, error)
	// SetReminderTime sets the time for a specific reminder and schedules it.
	SetReminderTime(ctx context.Context, req dto.SetReminderTimeRequest) error
	// ListActiveReminders retrieves a list of active reminders for a user.
	ListActiveReminders(ctx context.Context, userID string) ([]dto.ReminderResponse, error)
	// CancelReminder cancels reminders matching the given content for a user.
	CancelReminder(ctx context.Context, req dto.CancelReminderRequest) error
	// Snooze reschedules the last notified reminder for the user.
	Snooze(ctx context.Context, req dto.SnoozeRequest) (reminderContent string, err error)
	// HandleReminderNotification sends the reminder notification via LINE.
	HandleReminderNotification(ctx context.Context, reminderID uint) error
	// CleanupOldReminders deletes reminders that have passed their notification time + grace period.
	CleanupOldReminders(ctx context.Context, reminderID uint) error
	// GetReminder retrieves a reminder by its ID.
	GetReminder(ctx context.Context, reminderID uint) (*entity.Reminder, error)
}
