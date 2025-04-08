package entity

import "reminder/internal/domain/constant"

// User represents the user session information.
type User struct {
	ID               string  `gorm:"column:user_id;primaryKey"`
	TextID           *string `gorm:"column:text_id"`             // Temporarily holds the ID of the reminder being set up (before time is confirmed)
	LastNotifyTextID *string `gorm:"column:last_notify_text_id"` // Holds the ID of the last notified reminder for snooze functionality
	Status           int     `gorm:"column:status"`
}

// TableName specifies the table name for the User entity.
func (User) TableName() string {
	return "user_session"
}

// GetStatus returns the user status as a UserStatus type.
func (u *User) GetStatus() constant.UserStatus {
	return constant.UserStatus(u.Status)
}

// SetStatus sets the user status.
func (u *User) SetStatus(status constant.UserStatus) {
	u.Status = status.Int()
}
