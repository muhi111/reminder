package dto

import (
	"reminder/internal/domain/constant"
)

// UpdateUserStatusRequest is the DTO for updating a user's status.
type UpdateUserStatusRequest struct {
	UserID string              `json:"user_id"`
	Status constant.UserStatus `json:"status"`
	TextID *string             `json:"text_id,omitempty"` // Optional: ID of the reminder being processed
}

// UpdateUserLastNotifyRequest is the DTO for updating the last notified reminder ID.
type UpdateUserLastNotifyRequest struct {
	UserID           string `json:"user_id"`
	LastNotifyTextID string `json:"last_notify_text_id"` // ID of the reminder just notified
}
