package service

import (
	"context"
	"errors"
	"fmt"
	"reminder/internal/application/dto"
	"reminder/internal/domain/constant"
	"reminder/internal/domain/entity"
	"reminder/internal/domain/repository"
	"reminder/internal/infrastructure/line" // Assuming this is the infrastructure package
	appErrors "reminder/internal/pkg/errors"
	"reminder/internal/pkg/logger"
	"strconv"
	"time"

	"github.com/line/line-bot-sdk-go/v7/linebot"
	"gorm.io/gorm"
)

type reminderService struct {
	reminderRepo repository.ReminderRepository
	userRepo     repository.UserRepository
	schedulerSvc SchedulerService // Use the interface
	lineClient   *line.Client     // Use the infrastructure client wrapper
	log          logger.Logger
}

// NewReminderService creates a new instance of ReminderService implementation.
func NewReminderService(
	reminderRepo repository.ReminderRepository,
	userRepo repository.UserRepository,
	schedulerSvc SchedulerService,
	lineClient *line.Client,
	log logger.Logger,
) ReminderService {
	// Cast to the implementation type to set handlers (dependency injection workaround)
	schedulerImpl, ok := schedulerSvc.(*schedulerService)
	if !ok {
		// Handle error: schedulerSvc is not the expected implementation type
		// This indicates a setup issue in main.go or dependency injection container.
		log.Error("🔴 ERROR: SchedulerService provided is not the expected implementation type (*schedulerService)", nil)
		return nil
	}

	rs := &reminderService{
		reminderRepo: reminderRepo,
		userRepo:     userRepo,
		schedulerSvc: schedulerSvc, // Store the interface
		lineClient:   lineClient,
		log:          log,
	}

	// Set the handlers on the scheduler implementation if casting was successful
	if schedulerImpl != nil {
		schedulerImpl.SetNotificationHandler(rs.HandleReminderNotification)
		schedulerImpl.SetCleanupHandler(rs.CleanupOldReminders)
		log.Info("Notification and cleanup handlers set for SchedulerService.")
	}

	return rs
}

// CreateReminder creates a new reminder entry (initially without a time).
func (s *reminderService) CreateReminder(ctx context.Context, req dto.CreateReminderRequest) (uint, error) {
	reminder := &entity.Reminder{
		UserID:  req.UserID,
		Content: req.Content,
		// RemindTime is initially zero
	}
	reminderID, err := s.reminderRepo.Create(ctx, reminder)
	if err != nil {
		s.log.Error(fmt.Sprintf("Failed to create reminder for user %s", req.UserID), err)
		return 0, fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}
	s.log.Info(fmt.Sprintf("Created initial reminder entry %d for user %s", reminderID, req.UserID))
	return reminderID, nil
}

// SetReminderTime sets the time for a specific reminder and schedules it.
func (s *reminderService) SetReminderTime(ctx context.Context, req dto.SetReminderTimeRequest) error {
	// Fetch the reminder using the TextID stored in the user session
	user, err := s.userRepo.FindByUserID(ctx, req.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(errors.Unwrap(err), gorm.ErrRecordNotFound) {
			return appErrors.ErrUserNotFound
		}
		s.log.Error(fmt.Sprintf("Failed to find user %s while setting reminder time", req.UserID), err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	if user.TextID == nil || *user.TextID == "" {
		s.log.Warn(fmt.Sprintf("User %s has no pending reminder (TextID is nil) when trying to set time.", req.UserID))
		return fmt.Errorf("%w: no pending reminder found for user", appErrors.ErrInvalidStatus)
	}

	// Convert TextID (string pointer) to uint for reminder lookup
	reminderIDUint64, err := strconv.ParseUint(*user.TextID, 10, 64)
	if err != nil {
		s.log.Error(fmt.Sprintf("Failed to parse TextID %s to uint for user %s", *user.TextID, req.UserID), err)
		return fmt.Errorf("%w: invalid TextID format", appErrors.ErrInternalServer)
	}
	reminderID := uint(reminderIDUint64)

	reminder, err := s.reminderRepo.FindByID(ctx, reminderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(errors.Unwrap(err), gorm.ErrRecordNotFound) {
			s.log.Error(fmt.Sprintf("Pending reminder %d not found for user %s (TextID mismatch?)", reminderID, req.UserID), nil)
			return fmt.Errorf("%w: pending reminder referenced by TextID not found", appErrors.ErrReminderNotFound)
		}
		s.log.Error(fmt.Sprintf("Failed to find reminder %d for user %s", reminderID, req.UserID), err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	// Validate time
	if req.RemindTime.IsZero() || req.RemindTime.Before(time.Now()) {
		return appErrors.ErrInvalidDateTime
	}

	reminder.RemindTime = req.RemindTime

	// Update reminder in DB
	if err := s.reminderRepo.Update(ctx, reminder); err != nil {
		s.log.Error(fmt.Sprintf("Failed to update reminder time for reminder %d", reminder.ID), err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	// Schedule the reminder
	if err := s.schedulerSvc.ScheduleReminder(ctx, reminder); err != nil {
		s.log.Error(fmt.Sprintf("Failed to schedule reminder %d after setting time", reminder.ID), err)
		return err // Return the specific scheduling error
	}

	s.log.Info(fmt.Sprintf("Set time and scheduled reminder %d for user %s at %v", reminder.ID, req.UserID, req.RemindTime))
	return nil
}

// ListActiveReminders retrieves a list of active reminders for a user.
func (s *reminderService) ListActiveReminders(ctx context.Context, userID string) ([]dto.ReminderResponse, error) {
	reminders, err := s.reminderRepo.FindActiveByUserID(ctx, userID)
	if err != nil {
		s.log.Error(fmt.Sprintf("Failed to list active reminders for user %s", userID), err)
		return nil, fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}
	return dto.ToReminderResponseList(reminders), nil
}

// CancelReminder cancels reminders matching the given content for a user.
func (s *reminderService) CancelReminder(ctx context.Context, req dto.CancelReminderRequest) error {
	// Find reminders to get their IDs for schedule cancellation
	remindersToCancel, err := s.reminderRepo.FindByUserIDAndContent(ctx, req.UserID, req.Content)
	if err != nil {
		s.log.Error(fmt.Sprintf("Failed to find reminders by content '%s' for user %s during cancellation", req.Content, req.UserID), err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	if len(remindersToCancel) == 0 {
		return appErrors.ErrReminderNotFound // No reminders matched the content
	}

	// Delete from DB first
	if err := s.reminderRepo.DeleteByUserIDAndContent(ctx, req.UserID, req.Content); err != nil {
		s.log.Error(fmt.Sprintf("Failed to delete reminders by content '%s' for user %s", req.Content, req.UserID), err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	// Cancel schedules for each deleted reminder
	cancelledCount := 0
	for _, reminder := range remindersToCancel {
		// Cancel notification schedule
		if err := s.schedulerSvc.CancelReminderSchedule(ctx, reminder.ID); err != nil {
			// Log error but continue trying to cancel others
			s.log.Error(fmt.Sprintf("Failed to cancel notification schedule for reminder %d during bulk cancel", reminder.ID), err)
		}
		// Cancel cleanup schedule (if any)
		if err := s.schedulerSvc.CancelCleanupSchedule(ctx, reminder.ID); err != nil {
			s.log.Error(fmt.Sprintf("Failed to cancel cleanup schedule for reminder %d during bulk cancel", reminder.ID), err)
		}
		cancelledCount++
	}

	s.log.Info(fmt.Sprintf("Cancelled %d reminders with content '%s' for user %s", cancelledCount, req.Content, req.UserID))
	return nil
}

// Snooze reschedules the last notified reminder for the user.
func (s *reminderService) Snooze(ctx context.Context, req dto.SnoozeRequest) (string, error) {
	user, err := s.userRepo.FindByUserID(ctx, req.UserID)
	if err != nil {
		// Handle user not found or DB error
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(errors.Unwrap(err), gorm.ErrRecordNotFound) {
			return "", appErrors.ErrUserNotFound
		}
		s.log.Error(fmt.Sprintf("Failed to find user %s during snooze", req.UserID), err)
		return "", fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	if user.LastNotifyTextID == nil || *user.LastNotifyTextID == "" {
		return "", appErrors.ErrSnoozeUnavailable
	}

	// Convert LastNotifyTextID to uint
	lastNotifyIDUint64, err := strconv.ParseUint(*user.LastNotifyTextID, 10, 64)
	if err != nil {
		s.log.Error(fmt.Sprintf("Failed to parse LastNotifyTextID %s to uint for user %s during snooze", *user.LastNotifyTextID, req.UserID), err)
		return "", fmt.Errorf("%w: invalid LastNotifyTextID format", appErrors.ErrInternalServer)
	}
	lastNotifyID := uint(lastNotifyIDUint64)

	// Find the original reminder content (it might have been cleaned up already)
	// We fetch by ID, assuming cleanup might have failed or not run yet.
	// If FindByID fails, we can't snooze.
	originalReminder, err := s.reminderRepo.FindByID(ctx, lastNotifyID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(errors.Unwrap(err), gorm.ErrRecordNotFound) {
			s.log.Warn(fmt.Sprintf("Original reminder %d for snooze not found for user %s (likely cleaned up)", lastNotifyID, req.UserID))
			return "", appErrors.ErrSnoozeUnavailable // Original reminder gone
		}
		s.log.Error(fmt.Sprintf("Failed to find original reminder %d for user %s during snooze", lastNotifyID, req.UserID), err)
		return "", fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	// Cancel the cleanup job for the original reminder, if it exists
	if err := s.schedulerSvc.CancelCleanupSchedule(ctx, originalReminder.ID); err != nil {
		// Log error but proceed with snooze
		s.log.Error(fmt.Sprintf("Failed to cancel cleanup schedule for original reminder %d during snooze", originalReminder.ID), err)
	}

	// Create a *new* reminder entry with the same content
	newReminderID, err := s.CreateReminder(ctx, dto.CreateReminderRequest{
		UserID:  req.UserID,
		Content: originalReminder.Content,
	})
	if err != nil {
		// CreateReminder already logs the error
		return "", err // Return the error from CreateReminder
	}

	// Update user status to awaiting time, storing the *new* reminder ID in TextID
	newReminderIDStr := strconv.FormatUint(uint64(newReminderID), 10)
	// Use the UserService interface method (requires userService instance or refactoring)
	// For now, directly update repository (less clean)
	user.SetStatus(constant.StatusAwaitingTime)
	user.TextID = &newReminderIDStr
	if err := s.userRepo.Update(ctx, user); err != nil {
		s.log.Error(fmt.Sprintf("Failed to update user status during snooze for user %s", req.UserID), err)
		return "", fmt.Errorf("%w: failed to update user status after creating snooze reminder: %v", appErrors.ErrDatabaseOperation, err)
	}

	s.log.Info(fmt.Sprintf("Snooze initiated for user %s. New reminder %d created. Awaiting time.", req.UserID, newReminderID))
	return originalReminder.Content, nil // Return the content for confirmation message
}

// HandleReminderNotification sends the reminder notification via LINE.
func (s *reminderService) HandleReminderNotification(ctx context.Context, reminderID uint) error {
	s.log.Info(fmt.Sprintf("Handling notification for reminder %d", reminderID))
	reminder, err := s.reminderRepo.FindByID(ctx, reminderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(errors.Unwrap(err), gorm.ErrRecordNotFound) {
			s.log.Warn(fmt.Sprintf("Reminder %d not found during notification handling (already deleted?)", reminderID))
			return nil // Don't treat as error if already deleted
		}
		s.log.Error(fmt.Sprintf("Failed to find reminder %d for notification", reminderID), err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}

	// Send push message
	messageText := fmt.Sprintf("「%s」の時間です", reminder.Content)
	snoozeButton := linebot.NewQuickReplyButton("", linebot.NewMessageAction("スヌーズ", "スヌーズ")) // Use empty string for no image URL
	quickReply := linebot.NewQuickReplyItems(snoozeButton)
	message := linebot.NewTextMessage(messageText).WithQuickReplies(quickReply)

	if err := s.lineClient.PushMessages(reminder.UserID, message); err != nil {
		s.log.Error(fmt.Sprintf("Failed to push notification for reminder %d to user %s", reminderID, reminder.UserID), err)
		// return appErrors.ErrLineAPI // Indicate LINE API failure
	} else {
		s.log.Info(fmt.Sprintf("Successfully pushed notification for reminder %d to user %s", reminderID, reminder.UserID))
	}

	// Update user's last notified ID
	reminderIDStr := strconv.FormatUint(uint64(reminderID), 10)
	// Use UserService or update repo directly
	user, err := s.userRepo.FindByUserID(ctx, reminder.UserID)
	if err != nil {
		s.log.Error(fmt.Sprintf("Failed to find user %s after sending notification for reminder %d", reminder.UserID, reminderID), err)
		// Log error but proceed with cleanup scheduling
	} else {
		user.LastNotifyTextID = &reminderIDStr
		if err := s.userRepo.Update(ctx, user); err != nil {
			s.log.Error(fmt.Sprintf("Failed to update LastNotifyTextID for user %s after notification for reminder %d", reminder.UserID, reminderID), err)
			// Log error but proceed with cleanup scheduling
		} else {
			s.log.Debug(fmt.Sprintf("Updated LastNotifyTextID for user %s to %d", reminder.UserID, reminderID))
		}
	}

	// Schedule cleanup job
	if err := s.schedulerSvc.ScheduleReminderCleanup(ctx, reminderID); err != nil {
		s.log.Error(fmt.Sprintf("Failed to schedule cleanup for reminder %d after notification", reminderID), err)
		// Log error, but notification was sent.
	}

	return nil
}

// CleanupOldReminders deletes a specific reminder after its grace period.
func (s *reminderService) CleanupOldReminders(ctx context.Context, reminderID uint) error {
	s.log.Info(fmt.Sprintf("Cleaning up old reminder %d", reminderID))
	err := s.reminderRepo.Delete(ctx, reminderID)
	if err != nil {
		// Check if already deleted
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(errors.Unwrap(err), gorm.ErrRecordNotFound) {
			s.log.Warn(fmt.Sprintf("Reminder %d already deleted before cleanup job ran.", reminderID))
			return nil
		}
		s.log.Error(fmt.Sprintf("Failed to delete reminder %d during cleanup", reminderID), err)
		return fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}
	s.log.Info(fmt.Sprintf("Successfully cleaned up reminder %d", reminderID))
	return nil
}

// GetReminder retrieves a reminder by its ID.
func (s *reminderService) GetReminder(ctx context.Context, reminderID uint) (*entity.Reminder, error) {
	reminder, err := s.reminderRepo.FindByID(ctx, reminderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(errors.Unwrap(err), gorm.ErrRecordNotFound) {
			return nil, appErrors.ErrReminderNotFound
		}
		s.log.Error(fmt.Sprintf("Failed to get reminder %d", reminderID), err)
		return nil, fmt.Errorf("%w: %v", appErrors.ErrDatabaseOperation, err)
	}
	return reminder, nil
}
