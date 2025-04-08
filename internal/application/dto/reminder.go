package dto

import (
	"reminder/internal/domain/entity"
	"time"
)

// ReminderResponse is the DTO for sending reminder information to the client (e.g., listing reminders).
type ReminderResponse struct {
	ID         uint      `json:"id"`
	Content    string    `json:"content"`
	RemindTime time.Time `json:"remind_time"`
}

// ToReminderResponse converts an entity.Reminder to a ReminderResponse DTO.
func ToReminderResponse(r *entity.Reminder) ReminderResponse {
	return ReminderResponse{
		ID:         r.ID,
		Content:    r.Content,
		RemindTime: r.RemindTime,
	}
}

// ToReminderResponseList converts a slice of entity.Reminder to a slice of ReminderResponse DTOs.
func ToReminderResponseList(reminders []*entity.Reminder) []ReminderResponse {
	list := make([]ReminderResponse, len(reminders))
	for i, r := range reminders {
		list[i] = ToReminderResponse(r)
	}
	return list
}

// CreateReminderRequest is the DTO for creating a new reminder.
type CreateReminderRequest struct {
	UserID  string `json:"user_id"`
	Content string `json:"content"`
}

// SetReminderTimeRequest is the DTO for setting the time of a reminder.
type SetReminderTimeRequest struct {
	UserID     string    `json:"user_id"`
	ReminderID uint      `json:"reminder_id"`
	RemindTime time.Time `json:"remind_time"`
}

// CancelReminderRequest is the DTO for cancelling a reminder.
type CancelReminderRequest struct {
	UserID  string `json:"user_id"`
	Content string `json:"content"` // User provides content to identify reminder for cancellation
}

// SnoozeRequest is the DTO for snoozing a reminder.
type SnoozeRequest struct {
	UserID string `json:"user_id"`
}
