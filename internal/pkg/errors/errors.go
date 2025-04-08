package errors

import "errors"

// Custom application errors
var (
	ErrUserNotFound      = errors.New("ユーザーが見つかりません")         // User not found
	ErrInvalidStatus     = errors.New("無効なステータスです")           // Invalid user status for the operation
	ErrReminderNotFound  = errors.New("リマインダーが見つかりません")       // Reminder not found
	ErrInvalidDateTime   = errors.New("無効な日時形式です")            // Invalid date/time format from user or postback
	ErrDatabaseOperation = errors.New("データベース操作に失敗しました")      // Generic database error
	ErrLineAPI           = errors.New("LINE APIとの通信に失敗しました")  // Generic LINE API error
	ErrScheduling        = errors.New("スケジューリングに失敗しました")      // Generic scheduling error
	ErrSnoozeUnavailable = errors.New("スヌーズ可能なリマインド履歴がありません") // Snooze attempted with no previous notification
	ErrInternalServer    = errors.New("内部サーバーエラーが発生しました")     // Generic internal error
)
