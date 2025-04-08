package service

import (
	"context"
	"fmt"
	"reminder/internal/domain/entity"
	"reminder/internal/domain/repository"
	"reminder/internal/infrastructure/scheduler" // Assuming this is the infrastructure package
	appErrors "reminder/internal/pkg/errors"
	"reminder/internal/pkg/logger"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Define constants for job types or tags if needed for identification
const (
	jobTypeNotify  = "notify"
	jobTypeCleanup = "cleanup"
	// Grace period after notification before cleanup
	cleanupGracePeriod = 24 * time.Hour
)

type schedulerService struct {
	cronScheduler *scheduler.Scheduler // The infrastructure scheduler
	reminderRepo  repository.ReminderRepository
	// Need ReminderService to handle the actual notification/cleanup logic
	// This creates a circular dependency if injected directly.
	// We'll use a function closure or pass the service method later.
	handleNotificationFunc func(ctx context.Context, reminderID uint) error
	handleCleanupFunc      func(ctx context.Context, reminderID uint) error
	log                    logger.Logger
	// Store job IDs associated with reminder IDs
	// map[reminderID]map[jobType]cron.EntryID
	jobStore map[uint]map[string]cron.EntryID
	mu       sync.Mutex // Protect jobStore access
}

// NewSchedulerService creates a new instance of SchedulerService implementation.
// Note: handleNotificationFunc and handleCleanupFunc need to be set later to avoid circular deps.
func NewSchedulerService(
	cronScheduler *scheduler.Scheduler,
	reminderRepo repository.ReminderRepository,
	log logger.Logger,
) SchedulerService {
	return &schedulerService{
		cronScheduler: cronScheduler,
		reminderRepo:  reminderRepo,
		log:           log,
		jobStore:      make(map[uint]map[string]cron.EntryID),
	}
}

// SetNotificationHandler sets the function to be called when a reminder notification job runs.
// This is called during dependency injection setup to break circular dependency.
func (s *schedulerService) SetNotificationHandler(handler func(ctx context.Context, reminderID uint) error) {
	s.handleNotificationFunc = handler
}

// SetCleanupHandler sets the function to be called when a reminder cleanup job runs.
// This is called during dependency injection setup to break circular dependency.
func (s *schedulerService) SetCleanupHandler(handler func(ctx context.Context, reminderID uint) error) {
	s.handleCleanupFunc = handler
}

// formatCronSpec generates a cron spec string for a specific time.
func formatCronSpec(t time.Time) string {
	// Seconds Minutes Hours DayOfMonth Month DayOfWeek
	return fmt.Sprintf("%d %d %d %d %d *", t.Second(), t.Minute(), t.Hour(), t.Day(), t.Month())
}

// storeJobID stores the cron EntryID for a specific reminder and job type.
func (s *schedulerService) storeJobID(reminderID uint, jobType string, entryID cron.EntryID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobStore[reminderID]; !ok {
		s.jobStore[reminderID] = make(map[string]cron.EntryID)
	}
	s.jobStore[reminderID][jobType] = entryID
	s.log.Debug(fmt.Sprintf("Stored job ID %d for reminder %d, type %s", entryID, reminderID, jobType))
}

// removeJobID removes and returns the cron EntryID for a specific reminder and job type.
func (s *schedulerService) removeJobID(reminderID uint, jobType string) (cron.EntryID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if jobs, ok := s.jobStore[reminderID]; ok {
		if entryID, exists := jobs[jobType]; exists {
			delete(s.jobStore[reminderID], jobType)
			// Clean up reminder map if empty
			if len(s.jobStore[reminderID]) == 0 {
				delete(s.jobStore, reminderID)
			}
			s.log.Debug(fmt.Sprintf("Removed job ID %d for reminder %d, type %s", entryID, reminderID, jobType))
			return entryID, true
		}
	}
	return 0, false
}

// ScheduleReminder schedules a job to notify the user at the reminder time.
func (s *schedulerService) ScheduleReminder(ctx context.Context, reminder *entity.Reminder) error {
	if s.handleNotificationFunc == nil {
		s.log.Error("Notification handler function is not set in SchedulerService", nil)
		return fmt.Errorf("%w: notification handler not set", appErrors.ErrInternalServer)
	}
	if reminder.RemindTime.IsZero() || reminder.RemindTime.Before(time.Now()) {
		s.log.Warn(fmt.Sprintf("Attempted to schedule reminder %d with invalid or past time: %v", reminder.ID, reminder.RemindTime))
		return fmt.Errorf("%w: cannot schedule reminder with past or zero time", appErrors.ErrScheduling)
	}

	// Cancel existing notification job for this reminder, if any
	s.CancelReminderSchedule(ctx, reminder.ID)

	spec := formatCronSpec(reminder.RemindTime)
	reminderID := reminder.ID

	jobFunc := func() {
		s.log.Info(fmt.Sprintf("Executing notification job for reminder %d", reminderID))
		// Use background context for cron job execution
		if err := s.handleNotificationFunc(context.Background(), reminderID); err != nil {
			s.log.Error(fmt.Sprintf("Error handling notification for reminder %d", reminderID), err)
		}
		// Remove the job ID from store after execution (it's a one-off)
		s.removeJobID(reminderID, jobTypeNotify)
	}

	entryID, err := s.cronScheduler.AddJob(spec, jobFunc)
	if err != nil {
		return fmt.Errorf("%w: %v", appErrors.ErrScheduling, err)
	}

	s.storeJobID(reminderID, jobTypeNotify, entryID)
	s.log.Info(fmt.Sprintf("Scheduled notification for reminder %d at %v (Job ID: %d)", reminderID, reminder.RemindTime, entryID))
	return nil
}

// ScheduleReminderCleanup schedules a job to delete an old reminder after a grace period.
func (s *schedulerService) ScheduleReminderCleanup(ctx context.Context, reminderID uint) error {
	if s.handleCleanupFunc == nil {
		s.log.Error("Cleanup handler function is not set in SchedulerService", nil)
		return fmt.Errorf("%w: cleanup handler not set", appErrors.ErrInternalServer)
	}

	// Cancel existing cleanup job first
	s.CancelCleanupSchedule(ctx, reminderID)

	cleanupTime := time.Now().Add(cleanupGracePeriod)
	spec := formatCronSpec(cleanupTime)
	capturedReminderID := reminderID

	jobFunc := func() {
		s.log.Info(fmt.Sprintf("Executing cleanup job for reminder %d", capturedReminderID))
		// Use background context for cron job execution
		if err := s.handleCleanupFunc(context.Background(), capturedReminderID); err != nil {
			s.log.Error(fmt.Sprintf("Error handling cleanup for reminder %d", capturedReminderID), err)
		}
		// Remove the job ID from store after execution
		s.removeJobID(capturedReminderID, jobTypeCleanup)
	}

	entryID, err := s.cronScheduler.AddJob(spec, jobFunc)
	if err != nil {
		return fmt.Errorf("%w: %v", appErrors.ErrScheduling, err)
	}

	s.storeJobID(capturedReminderID, jobTypeCleanup, entryID)
	s.log.Info(fmt.Sprintf("Scheduled cleanup for reminder %d at %v (Job ID: %d)", capturedReminderID, cleanupTime, entryID))
	return nil
}

// CancelReminderSchedule cancels the notification job for a specific reminder.
func (s *schedulerService) CancelReminderSchedule(ctx context.Context, reminderID uint) error {
	if entryID, ok := s.removeJobID(reminderID, jobTypeNotify); ok {
		s.cronScheduler.RemoveJob(entryID)
		s.log.Info(fmt.Sprintf("Cancelled notification schedule for reminder %d (Job ID: %d)", reminderID, entryID))
	} else {
		s.log.Debug(fmt.Sprintf("No active notification schedule found for reminder %d to cancel.", reminderID))
	}
	return nil
}

// CancelCleanupSchedule cancels the cleanup job for a specific reminder.
func (s *schedulerService) CancelCleanupSchedule(ctx context.Context, reminderID uint) error {
	if entryID, ok := s.removeJobID(reminderID, jobTypeCleanup); ok {
		s.cronScheduler.RemoveJob(entryID)
		s.log.Info(fmt.Sprintf("Cancelled cleanup schedule for reminder %d (Job ID: %d)", reminderID, entryID))
	} else {
		s.log.Debug(fmt.Sprintf("No active cleanup schedule found for reminder %d to cancel.", reminderID))
	}
	return nil
}

// InitializeSchedules loads reminders from the DB and schedules them on startup.
func (s *schedulerService) InitializeSchedules(ctx context.Context) error {
	s.log.Info("Initializing schedules from database...")
	reminders, err := s.reminderRepo.FindAll(ctx)
	if err != nil {
		s.log.Error("Failed to retrieve reminders for initialization", err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	now := time.Now()
	scheduledCount := 0
	deletedCount := 0

	for _, reminder := range reminders {
		if reminder.RemindTime.IsZero() {
			s.log.Warn(fmt.Sprintf("Reminder %d found with zero time during init, skipping.", reminder.ID))
			continue
		}

		if reminder.RemindTime.Before(now) {
			// Delete past reminders immediately on startup
			if err := s.reminderRepo.Delete(ctx, reminder.ID); err != nil {
				s.log.Error(fmt.Sprintf("Failed to delete past reminder %d during init", reminder.ID), err)
				// Continue trying to schedule others
			} else {
				deletedCount++
				s.log.Info(fmt.Sprintf("Deleted past reminder %d during init.", reminder.ID))
			}
		} else {
			// Schedule future reminders
			if err := s.ScheduleReminder(ctx, reminder); err != nil {
				s.log.Error(fmt.Sprintf("Failed to schedule reminder %d during init", reminder.ID), err)
				// Continue trying to schedule others
			} else {
				scheduledCount++
			}
		}
	}

	s.log.Info(fmt.Sprintf("Schedule initialization complete. Scheduled: %d, Deleted Past: %d", scheduledCount, deletedCount))
	// Log current jobs for debugging
	s.log.Debug(fmt.Sprintf("Current cron entries: %v", s.cronScheduler.GetEntries()))
	return nil
}

// Stop stops the underlying scheduler.
func (s *schedulerService) Stop() {
	s.cronScheduler.Stop()
}
