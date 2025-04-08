package entity

import "time"

// Reminder represents the reminder content and time.
type Reminder struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"`
	UserID     string    `gorm:"column:user_id;index"`
	Content    string    `gorm:"column:content;type:text"`
	RemindTime time.Time `gorm:"column:remind_time"`
}

// TableName specifies the table name for the Reminder entity.
func (Reminder) TableName() string {
	return "reminder_content"
}
