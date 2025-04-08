package sqlite

import (
	"context"
	"errors"
	"fmt"
	"reminder/internal/domain/entity"
	"reminder/internal/domain/repository"
	"time"

	"gorm.io/gorm"
)

type reminderRepository struct {
	db *gorm.DB
}

// NewReminderRepository creates a new instance of ReminderRepository.
func NewReminderRepository(db *gorm.DB) repository.ReminderRepository {
	return &reminderRepository{db: db}
}

// FindByID retrieves a reminder by its ID.
func (r *reminderRepository) FindByID(ctx context.Context, id uint) (*entity.Reminder, error) {
	var reminder entity.Reminder
	if err := r.db.WithContext(ctx).First(&reminder, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("reminder with ID %d not found: %w", id, err)
		}
		return nil, fmt.Errorf("🔴 ERROR: failed to find reminder by id %d: %w", id, err)
	}
	return &reminder, nil
}

// FindByUserID retrieves all reminders for a specific user.
func (r *reminderRepository) FindByUserID(ctx context.Context, userID string) ([]*entity.Reminder, error) {
	var reminders []*entity.Reminder
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&reminders).Error; err != nil {
		return nil, fmt.Errorf("🔴 ERROR: failed to find reminders by user_id %s: %w", userID, err)
	}
	return reminders, nil
}

// FindActiveByUserID retrieves active reminders (remind_time in the future) for a specific user.
func (r *reminderRepository) FindActiveByUserID(ctx context.Context, userID string) ([]*entity.Reminder, error) {
	var reminders []*entity.Reminder
	now := time.Now()
	if err := r.db.WithContext(ctx).Where("user_id = ? AND remind_time > ?", userID, now).Order("remind_time asc").Find(&reminders).Error; err != nil {
		return nil, fmt.Errorf("🔴 ERROR: failed to find active reminders by user_id %s: %w", userID, err)
	}
	return reminders, nil
}

// FindByUserIDAndContent retrieves reminders by user ID and content.
func (r *reminderRepository) FindByUserIDAndContent(ctx context.Context, userID string, content string) ([]*entity.Reminder, error) {
	var reminders []*entity.Reminder
	if err := r.db.WithContext(ctx).Where("user_id = ? AND content = ?", userID, content).Find(&reminders).Error; err != nil {
		return nil, fmt.Errorf("🔴 ERROR: failed to find reminders by user_id %s and content: %w", userID, err)
	}
	return reminders, nil
}

// FindAll retrieves all reminders (used for rescheduling on startup).
func (r *reminderRepository) FindAll(ctx context.Context) ([]*entity.Reminder, error) {
	var reminders []*entity.Reminder
	if err := r.db.WithContext(ctx).Find(&reminders).Error; err != nil {
		return nil, fmt.Errorf("🔴 ERROR: failed to find all reminders: %w", err)
	}
	return reminders, nil
}

// Create creates a new reminder. Returns the ID of the created reminder.
func (r *reminderRepository) Create(ctx context.Context, reminder *entity.Reminder) (uint, error) {
	if err := r.db.WithContext(ctx).Create(reminder).Error; err != nil {
		return 0, fmt.Errorf("🔴 ERROR: failed to create reminder for user %s: %w", reminder.UserID, err)
	}
	return reminder.ID, nil
}

// Update updates an existing reminder.
func (r *reminderRepository) Update(ctx context.Context, reminder *entity.Reminder) error {
	if err := r.db.WithContext(ctx).Save(reminder).Error; err != nil {
		return fmt.Errorf("🔴 ERROR: failed to update reminder %d: %w", reminder.ID, err)
	}
	return nil
}

// Delete deletes a reminder by its ID.
func (r *reminderRepository) Delete(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&entity.Reminder{}, id).Error; err != nil {
		return fmt.Errorf("🔴 ERROR: failed to delete reminder %d: %w", id, err)
	}
	return nil
}

// DeleteByUserID deletes all reminders for a specific user.
func (r *reminderRepository) DeleteByUserID(ctx context.Context, userID string) error {
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&entity.Reminder{}).Error; err != nil {
		return fmt.Errorf("🔴 ERROR: failed to delete reminders for user %s: %w", userID, err)
	}
	return nil
}

// DeleteByUserIDAndContent deletes reminders by user ID and content.
func (r *reminderRepository) DeleteByUserIDAndContent(ctx context.Context, userID string, content string) error {
	if err := r.db.WithContext(ctx).Where("user_id = ? AND content = ?", userID, content).Delete(&entity.Reminder{}).Error; err != nil {
		return fmt.Errorf("🔴 ERROR: failed to delete reminders for user %s with content: %w", userID, err)
	}
	return nil
}

// DeleteOlderThan deletes reminders with remind_time older than the specified time.
func (r *reminderRepository) DeleteOlderThan(ctx context.Context, threshold time.Time) error {
	if err := r.db.WithContext(ctx).Where("remind_time < ?", threshold).Delete(&entity.Reminder{}).Error; err != nil {
		return fmt.Errorf("🔴 ERROR: failed to delete old reminders older than %v: %w", threshold, err)
	}
	return nil
}
