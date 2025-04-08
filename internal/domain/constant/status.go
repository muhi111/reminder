package constant

// UserStatus defines the possible states of a user interaction.
type UserStatus int

const (
	// StatusInitial represents the state where the bot is waiting for reminder content.
	StatusInitial UserStatus = iota // 0: リマインド内容入力待ち
	// StatusAwaitingTime represents the state where the bot is waiting for the reminder time.
	StatusAwaitingTime // 1: 時刻入力待ち
	// StatusAwaitingCancel represents the state where the bot is waiting for the user to select a reminder to cancel.
	StatusAwaitingCancel // 2: キャンセル選択待ち
)

func (s UserStatus) Int() int {
	return int(s)
}
