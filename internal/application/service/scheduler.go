package service

import (
	"context"
	"reminder/internal/domain/entity"
)

// SchedulerService defines the interface for scheduling operations.
type SchedulerService interface {
	// ScheduleReminder schedules a job to notify the user at the reminder time.
	ScheduleReminder(ctx context.Context, reminder *entity.Reminder) error
	// ScheduleReminderCleanup schedules a job to delete an old reminder after a grace period.
	ScheduleReminderCleanup(ctx context.Context, reminderID uint) error
	// CancelReminderSchedule cancels the notification job for a specific reminder.
	CancelReminderSchedule(ctx context.Context, reminderID uint) error
	// CancelCleanupSchedule cancels the cleanup job for a specific reminder (e.g., during snooze).
	CancelCleanupSchedule(ctx context.Context, reminderID uint) error
	// InitializeSchedules loads reminders from the DB and schedules them on startup.
	InitializeSchedules(ctx context.Context) error
	// Stop stops the underlying scheduler.
	Stop()
}
