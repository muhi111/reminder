package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reminder/internal/application/dto"
	"reminder/internal/application/service"
	"reminder/internal/domain/constant"
	"reminder/internal/infrastructure/line"
	appErrors "reminder/internal/pkg/errors"
	"reminder/internal/pkg/logger"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/line/line-bot-sdk-go/v7/linebot"
)

// LineHandler handles incoming LINE webhook events.
type LineHandler struct {
	lineClient      *line.Client // Use the wrapper
	userService     service.UserService
	reminderService service.ReminderService
	log             logger.Logger
}

// NewLineHandler creates a new LineHandler.
func NewLineHandler(
	lineClient *line.Client,
	userService service.UserService,
	reminderService service.ReminderService,
	log logger.Logger,
) *LineHandler {
	return &LineHandler{
		lineClient:      lineClient,
		userService:     userService,
		reminderService: reminderService,
		log:             log,
	}
}

// HandleWebhook is the main entry point for webhook requests.
func (h *LineHandler) HandleWebhook(c echo.Context) error {
	ctx := c.Request().Context()
	events, err := h.lineClient.ParseRequest(c.Request())
	if err != nil {
		if errors.Is(err, linebot.ErrInvalidSignature) {
			h.log.Warn("Invalid LINE signature received")
			return c.String(http.StatusBadRequest, "Invalid signature")
		}
		h.log.Error("Failed to parse LINE webhook request", err)
		return c.String(http.StatusInternalServerError, "Error parsing request")
	}

	for _, event := range events {
		h.log.Info(fmt.Sprintf("Processing event type: %s", event.Type))
		switch event.Type {
		case linebot.EventTypeMessage:
			h.handleMessageEvent(ctx, event)
		case linebot.EventTypeFollow:
			h.handleFollowEvent(ctx, event)
		case linebot.EventTypeUnfollow:
			h.handleUnfollowEvent(ctx, event)
		case linebot.EventTypePostback:
			h.handlePostbackEvent(ctx, event)
			// Add other event types if needed (Join, Leave, etc.)
		default:
			h.log.Info(fmt.Sprintf("Unhandled event type: %s", event.Type))
		}
	}

	return c.String(http.StatusOK, "OK")
}

// handleFollowEvent processes follow events.
func (h *LineHandler) handleFollowEvent(ctx context.Context, event *linebot.Event) {
	userID := event.Source.UserID
	replyToken := event.ReplyToken
	h.log.Info(fmt.Sprintf("User %s followed the bot.", userID))

	_, err := h.userService.GetOrCreateUser(ctx, userID)
	if err != nil {
		// Error already logged by service
		h.replyWithError(replyToken, "ユーザー情報の初期化に失敗しました。")
		return
	}

	// Send welcome messages
	welcomeMsg1 := linebot.NewTextMessage("このbotはデモ版です。個人情報等などは登録しないで下さい。")
	welcomeMsg2 := linebot.NewTextMessage("また、MessagingAPIの無料枠の関係上、本格的な利用は不可能です。一か月あたり全体で多くとも200回のリマインドしか送れません。")
	welcomeMsg3 := linebot.NewTextMessage("使い方を知りたい場合「使い方」と入力してください。")

	if err := h.lineClient.SendMessages(replyToken, welcomeMsg1, welcomeMsg2, welcomeMsg3); err != nil {
		h.log.Error(fmt.Sprintf("Failed to send follow reply to user %s", userID), err)
		// Don't return here, try to send admin notification anyway
	}

	// Notify admin user if MY_USER_ID is set
	adminUserID := os.Getenv("MY_USER_ID")
	if adminUserID != "" {
		// Get follower's profile
		profile, err := h.lineClient.GetProfile(userID).Do()
		var notificationMessage string
		if err != nil {
			h.log.Warn(fmt.Sprintf("Failed to get profile for follower %s: %v", userID, err))
			// Send notification without display name
			notificationMessage = fmt.Sprintf("ユーザー (ID: %s) がボットをフォローしました。", userID)
		} else {
			notificationMessage = fmt.Sprintf("ユーザー「%s」(ID: %s) がボットをフォローしました。", profile.DisplayName, userID)
		}

		// Send push message to admin
		if pushErr := h.lineClient.PushMessages(adminUserID, linebot.NewTextMessage(notificationMessage)); pushErr != nil {
			h.log.Error(fmt.Sprintf("Failed to send follow notification to admin %s for follower %s", adminUserID, userID), pushErr)
		} else {
			h.log.Info(fmt.Sprintf("Sent follow notification to admin %s for follower %s", adminUserID, userID))
		}
	} else {
		h.log.Warn("MY_USER_ID environment variable not set. Skipping admin notification for follow event.")
	}
}

// handleUnfollowEvent processes unfollow events.
func (h *LineHandler) handleUnfollowEvent(ctx context.Context, event *linebot.Event) {
	userID := event.Source.UserID
	h.log.Info(fmt.Sprintf("User %s unfollowed or blocked the bot.", userID))

	if err := h.userService.DeleteUser(ctx, userID); err != nil {
		// Error already logged by service
		// No reply possible for unfollow events
	}
}

// handleMessageEvent processes message events.
func (h *LineHandler) handleMessageEvent(ctx context.Context, event *linebot.Event) {
	userID := event.Source.UserID
	replyToken := event.ReplyToken

	switch message := event.Message.(type) {
	case *linebot.TextMessage:
		text := message.Text
		h.log.Info(fmt.Sprintf("Received text message from %s: %s", userID, text))

		// Get user status first
		status, err := h.userService.GetUserStatus(ctx, userID)
		if err != nil {
			// If user not found, create them (shouldn't happen if follow worked, but handle defensively)
			if errors.Is(err, appErrors.ErrUserNotFound) {
				h.log.Warn(fmt.Sprintf("User %s not found during message event, attempting creation.", userID))
				_, createErr := h.userService.GetOrCreateUser(ctx, userID)
				if createErr != nil {
					h.replyWithError(replyToken, "ユーザー情報の取得または作成に失敗しました。")
					return
				}
				status = constant.StatusInitial // Newly created user starts here
			} else {
				// Other DB error
				h.replyWithError(replyToken, "ユーザー情報の取得に失敗しました。")
				return
			}
		}

		// Handle common commands regardless of status
		switch text {
		case "使い方":
			h.sendHowToUse(replyToken)
			return
		case "一覧":
			h.sendReminderList(ctx, replyToken, userID)
			return
		case "取り消し":
			// Only allow cancellation initiation from initial state
			if status == constant.StatusInitial {
				h.initiateCancellation(ctx, replyToken, userID)
			} else {
				h.replyWithError(replyToken, "現在、リマインダーの取り消しを開始できません。")
			}
			return
		case "スヌーズ":
			// Only allow snooze from initial state
			if status == constant.StatusInitial {
				h.handleSnooze(ctx, replyToken, userID)
			} else {
				h.replyWithError(replyToken, "現在、スヌーズを開始できません。")
			}
			return
		}

		// Handle status-specific logic
		switch status {
		case constant.StatusInitial:
			h.handleStatusInitialMessage(ctx, replyToken, userID, text)
		case constant.StatusAwaitingTime:
			h.handleStatusAwaitingTimeMessage(ctx, replyToken, userID, text)
		case constant.StatusAwaitingCancel:
			h.handleStatusAwaitingCancelMessage(ctx, replyToken, userID, text)
		default:
			h.log.Warn(fmt.Sprintf("User %s has unknown status: %d", userID, status))
			h.replyWithError(replyToken, "不明な状態です。最初からやり直してください。")
			// Optionally reset status
			if err := h.userService.UpdateStatus(ctx, dto.UpdateUserStatusRequest{UserID: userID, Status: constant.StatusInitial, TextID: nil}); err != nil {
				h.log.Error(fmt.Sprintf("Failed to reset status for user %s with unknown status %d", userID, status), err) // Log added for ignored error
			}
		}

	default:
		h.log.Info(fmt.Sprintf("Received non-text message type from %s", userID))
		// Optionally reply for non-text messages
		// h.replyWithError(replyToken, "テキストメッセージのみ対応しています。")
	}
}

// handlePostbackEvent processes postback events (datetime picker).
func (h *LineHandler) handlePostbackEvent(ctx context.Context, event *linebot.Event) {
	userID := event.Source.UserID
	replyToken := event.ReplyToken
	data := event.Postback.Data
	params := event.Postback.Params

	h.log.Info(fmt.Sprintf("Received postback from %s: data=%s, params=%v", userID, data, params))

	// Check user status - should be AwaitingTime
	status, err := h.userService.GetUserStatus(ctx, userID)
	if err != nil {
		h.replyWithError(replyToken, "ユーザー情報の取得に失敗しました。")
		return
	}

	if status != constant.StatusAwaitingTime {
		h.log.Warn(fmt.Sprintf("Received postback from user %s with unexpected status %d", userID, status))
		h.replyWithError(replyToken, "無効な日時選択アクションです。")
		return
	}

	// Extract datetime
	if params == nil || params.Datetime == "" {
		h.log.Warn(fmt.Sprintf("Postback from user %s missing datetime params", userID))
		h.replyWithError(replyToken, "日時の取得に失敗しました。")
		return
	}

	// Parse datetime (Format: "2017-12-25T01:00")
	remindTime, err := time.Parse("2006-01-02T15:04", params.Datetime)
	if err != nil {
		h.log.Error(fmt.Sprintf("Failed to parse datetime '%s' from postback for user %s", params.Datetime, userID), err)
		h.replyWithError(replyToken, "日時の形式が無効です。")
		return
	}

	// Call reminder service to set time
	setReq := dto.SetReminderTimeRequest{
		UserID:     userID,
		RemindTime: remindTime,
		// ReminderID is implicitly handled by the service using user's TextID
	}
	err = h.reminderService.SetReminderTime(ctx, setReq)
	if err != nil {
		// Handle specific errors from service
		if errors.Is(err, appErrors.ErrInvalidDateTime) {
			h.replyWithError(replyToken, "過去の日時や無効な日時は指定できません。")
		} else if errors.Is(err, appErrors.ErrReminderNotFound) {
			h.replyWithError(replyToken, "対象のリマインダーが見つかりませんでした。")
		} else if errors.Is(err, appErrors.ErrScheduling) {
			h.replyWithError(replyToken, "リマインダーのスケジュール設定に失敗しました。")
		} else {
			h.replyWithError(replyToken, "リマインダー日時の設定に失敗しました。")
		}
		return // Keep user in AwaitingTime status on failure
	}

	// Update user status back to initial
	updateStatusReq := dto.UpdateUserStatusRequest{
		UserID: userID,
		Status: constant.StatusInitial,
		TextID: nil, // Clear TextID
	}
	if err := h.userService.UpdateStatus(ctx, updateStatusReq); err != nil {
		// Log error, but the reminder is set and scheduled.
		h.log.Error(fmt.Sprintf("Failed to update user %s status after setting time", userID), err)
	}

	// Send confirmation
	confirmationMsg := fmt.Sprintf("登録できました!\n%sにリマインドします", remindTime.Format("2006/01/02 15:04"))
	if err := h.lineClient.SendMessages(replyToken, linebot.NewTextMessage(confirmationMsg)); err != nil {
		h.log.Error(fmt.Sprintf("Failed to send time confirmation to user %s", userID), err)
	}
}

// --- Helper methods for message handling ---

func (h *LineHandler) sendHowToUse(replyToken string) {
	howToUse := `まず、リマインドしたいことを教えてください!
その後にリマインドして欲しい日時を教えてください!

日時の指定は日時選択ボタンから行えます。

「一覧」と入力することで現在登録されているリマインドを確認できます。
「取り消し」と入力すると登録したリマインダーを削除できます。
「スヌーズ」と入力すると前回リマインドした内容を再び設定できます。`

	quickReply := linebot.NewQuickReplyItems(
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("一覧", "一覧")),
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("取り消し", "取り消し")),
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("スヌーズ", "スヌーズ")),
	)
	message := linebot.NewTextMessage(howToUse).WithQuickReplies(quickReply)
	if err := h.lineClient.SendMessages(replyToken, message); err != nil {
		h.log.Error("Failed to send 'how to use' message", err)
	}
}

func (h *LineHandler) sendReminderList(ctx context.Context, replyToken, userID string) {
	reminders, err := h.reminderService.ListActiveReminders(ctx, userID)
	if err != nil {
		h.replyWithError(replyToken, "リマインダー一覧の取得に失敗しました。")
		return
	}

	if len(reminders) == 0 {
		if err := h.lineClient.SendMessages(replyToken, linebot.NewTextMessage("現在登録されているリマインドはありません")); err != nil {
			h.log.Error(fmt.Sprintf("Failed to send empty list message to user %s", userID), err)
		}
		return
	}

	var builder strings.Builder
	for _, r := range reminders {
		builder.WriteString(fmt.Sprintf("%s \n%s\n\n", r.RemindTime.Format("2006/01/02 15:04"), r.Content))
	}
	listStr := strings.TrimSuffix(builder.String(), "\n\n")

	if err := h.lineClient.SendMessages(replyToken, linebot.NewTextMessage(listStr)); err != nil {
		h.log.Error(fmt.Sprintf("Failed to send reminder list to user %s", userID), err)
	}
}

func (h *LineHandler) initiateCancellation(ctx context.Context, replyToken, userID string) {
	reminders, err := h.reminderService.ListActiveReminders(ctx, userID)
	if err != nil {
		h.replyWithError(replyToken, "取り消し可能なリマインダー一覧の取得に失敗しました。")
		return
	}

	if len(reminders) == 0 {
		if err := h.lineClient.SendMessages(replyToken, linebot.NewTextMessage("現在登録されているリマインドはありません")); err != nil {
			h.log.Error(fmt.Sprintf("Failed to send empty list for cancellation to user %s", userID), err)
		}
		return
	}

	// Build list string
	var builder strings.Builder
	for _, r := range reminders {
		builder.WriteString(fmt.Sprintf("%s \n%s\n\n", r.RemindTime.Format("2006/01/02 15:04"), r.Content))
	}
	listStr := strings.TrimSuffix(builder.String(), "\n\n")

	// Update user status
	updateStatusReq := dto.UpdateUserStatusRequest{
		UserID: userID,
		Status: constant.StatusAwaitingCancel,
		TextID: nil, // Not needed for cancellation
	}
	if err := h.userService.UpdateStatus(ctx, updateStatusReq); err != nil {
		h.log.Error(fmt.Sprintf("Failed to update user %s status during cancellation initiation", userID), err) // Log added
		h.replyWithError(replyToken, "取り消し処理の開始に失敗しました。")
		return
	}

	// Send messages
	msg1 := linebot.NewTextMessage(listStr)
	msg2 := linebot.NewTextMessage("どのリマインドを取り消すか内容を正確に入力して下さい。\n(同じ内容のリマインドはすべて削除されます)")
	quickReply := linebot.NewQuickReplyItems(
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("キャンセル", "キャンセル")),
	)
	msg3 := linebot.NewTextMessage("取り消しをやめたい場合は「キャンセル」と入力してください。").WithQuickReplies(quickReply)

	if err := h.lineClient.SendMessages(replyToken, msg1, msg2, msg3); err != nil {
		h.log.Error(fmt.Sprintf("Failed to send cancellation initiation messages to user %s", userID), err)
		// Attempt to revert status?
		_ = h.userService.UpdateStatus(ctx, dto.UpdateUserStatusRequest{UserID: userID, Status: constant.StatusInitial, TextID: nil})
	}
}

func (h *LineHandler) handleSnooze(ctx context.Context, replyToken, userID string) {
	snoozeReq := dto.SnoozeRequest{UserID: userID}
	originalContent, err := h.reminderService.Snooze(ctx, snoozeReq)

	if err != nil {
		if errors.Is(err, appErrors.ErrSnoozeUnavailable) {
			h.replyWithError(replyToken, "スヌーズ可能なリマインド履歴がありません。")
		} else {
			h.replyWithError(replyToken, "スヌーズ処理に失敗しました。")
		}
		return
	}

	// Send confirmation and datetime picker
	msg1 := linebot.NewTextMessage(fmt.Sprintf("「%s」のスヌーズを開始します。", originalContent))
	h.sendDateTimePicker(replyToken, userID, msg1) // Reuse datetime picker sending logic
}

func (h *LineHandler) handleStatusInitialMessage(ctx context.Context, replyToken, userID, text string) {
	// Assume text is reminder content
	createReq := dto.CreateReminderRequest{
		UserID:  userID,
		Content: text,
	}
	reminderID, err := h.reminderService.CreateReminder(ctx, createReq)
	if err != nil {
		h.replyWithError(replyToken, "リマインダーの作成に失敗しました。")
		return
	}

	// Update user status to AwaitingTime and store reminder ID in TextID
	reminderIDStr := strconv.FormatUint(uint64(reminderID), 10)
	updateStatusReq := dto.UpdateUserStatusRequest{
		UserID: userID,
		Status: constant.StatusAwaitingTime,
		TextID: &reminderIDStr,
	}
	if err := h.userService.UpdateStatus(ctx, updateStatusReq); err != nil {
		h.log.Error(fmt.Sprintf("Failed to update user %s status after creating reminder %d", userID, reminderID), err) // Log added
		h.replyWithError(replyToken, "リマインダー作成後の状態更新に失敗しました。")
		// Attempt to delete the created reminder?
		return
	}

	// Send datetime picker
	h.sendDateTimePicker(replyToken, userID, nil) // No preceding message needed
}

func (h *LineHandler) handleStatusAwaitingTimeMessage(ctx context.Context, replyToken, userID, text string) {
	if text == "キャンセル" {
		// Get TextID to delete the pending reminder
		user, err := h.userService.GetUser(ctx, userID)
		if err != nil {
			h.replyWithError(replyToken, "ユーザー情報の取得に失敗しました。")
			return
		}
		if user.TextID != nil && *user.TextID != "" {
			reminderIDUint64, parseErr := strconv.ParseUint(*user.TextID, 10, 64)
			if parseErr == nil {
				// Attempt to delete the reminder entry
				// TODO: Implement ReminderService.DeleteReminderByID to properly clean up here.
				// Current CancelReminder expects content.
				h.log.Warn(fmt.Sprintf("Skipping deletion of pending reminder %d for user %s during AwaitingTime cancel (requires DeleteByID)", uint(reminderIDUint64), userID))
				// if delErr := h.reminderService.DeleteReminderByID(ctx, uint(reminderIDUint64)); delErr != nil {
				// 	h.log.Error(fmt.Sprintf("Failed to delete pending reminder %d during AwaitingTime cancel", uint(reminderIDUint64)), delErr)
				// }
			} else {
				h.log.Error(fmt.Sprintf("Failed to parse TextID %s during AwaitingTime cancel for user %s", *user.TextID, userID), parseErr)
			}
		}

		// Update status back to initial, clearing TextID
		updateStatusReq := dto.UpdateUserStatusRequest{
			UserID: userID,
			Status: constant.StatusInitial,
			TextID: nil,
		}
		if err := h.userService.UpdateStatus(ctx, updateStatusReq); err != nil {
			h.log.Error(fmt.Sprintf("Failed to update user %s status during AwaitingTime cancel", userID), err) // Log added
			h.replyWithError(replyToken, "キャンセル処理中の状態更新に失敗しました。")
			return
		}
		if err := h.lineClient.SendMessages(replyToken, linebot.NewTextMessage("キャンセルしました")); err != nil {
			h.log.Error(fmt.Sprintf("Failed to send cancel confirmation to user %s", userID), err)
		}
	} else if text == "再送" {
		// Resend datetime picker
		h.sendDateTimePicker(replyToken, userID, linebot.NewTextMessage("リマインド日時を再選択してください。"))
	} else {
		// Invalid input in this state
		quickReply := linebot.NewQuickReplyItems(
			linebot.NewQuickReplyButton("", linebot.NewMessageAction("キャンセル", "キャンセル")),
			linebot.NewQuickReplyButton("", linebot.NewMessageAction("再送", "再送")),
		)
		msg1 := linebot.NewTextMessage("日時選択ボタンからリマインドする日時を選択してください。")
		msg2 := linebot.NewTextMessage("再選択したい場合は「再送」を、キャンセルしたい場合は「キャンセル」を押してください。").WithQuickReplies(quickReply)
		if err := h.lineClient.SendMessages(replyToken, msg1, msg2); err != nil {
			h.log.Error(fmt.Sprintf("Failed to send AwaitingTime prompt to user %s", userID), err)
		}
	}
}

func (h *LineHandler) handleStatusAwaitingCancelMessage(ctx context.Context, replyToken, userID, text string) {
	if text == "キャンセル" {
		// Update status back to initial
		updateStatusReq := dto.UpdateUserStatusRequest{
			UserID: userID,
			Status: constant.StatusInitial,
			TextID: nil,
		}
		if err := h.userService.UpdateStatus(ctx, updateStatusReq); err != nil {
			h.log.Error(fmt.Sprintf("Failed to update user %s status during AwaitingCancel cancel", userID), err) // Log added
			h.replyWithError(replyToken, "キャンセル処理中の状態更新に失敗しました。")
			return
		}
		if err := h.lineClient.SendMessages(replyToken, linebot.NewTextMessage("キャンセルしました")); err != nil {
			h.log.Error(fmt.Sprintf("Failed to send cancel confirmation to user %s", userID), err)
		}
	} else {
		// Assume text is the content to cancel
		cancelReq := dto.CancelReminderRequest{
			UserID:  userID,
			Content: text,
		}
		err := h.reminderService.CancelReminder(ctx, cancelReq)
		if err != nil {
			if errors.Is(err, appErrors.ErrReminderNotFound) {
				quickReply := linebot.NewQuickReplyItems(
					linebot.NewQuickReplyButton("", linebot.NewMessageAction("キャンセル", "キャンセル")),
				)
				msg := linebot.NewTextMessage("入力された内容のリマインダーは見つかりませんでした。\nもう一度入力するか、「キャンセル」してください。").WithQuickReplies(quickReply)
				if err := h.lineClient.SendMessages(replyToken, msg); err != nil {
					h.log.Error(fmt.Sprintf("Failed to send cancel not found message to user %s", userID), err)
				}
			} else {
				h.replyWithError(replyToken, "リマインダーの取り消し処理に失敗しました。")
			}
			return // Keep user in AwaitingCancel status on failure/not found
		}

		// Update status back to initial on successful cancellation
		updateStatusReq := dto.UpdateUserStatusRequest{
			UserID: userID,
			Status: constant.StatusInitial,
			TextID: nil,
		}
		if err := h.userService.UpdateStatus(ctx, updateStatusReq); err != nil {
			// Log error, but cancellation was successful
			h.log.Error(fmt.Sprintf("Failed to update user %s status after successful cancellation", userID), err)
		}

		if err := h.lineClient.SendMessages(replyToken, linebot.NewTextMessage("取り消しが出来ました!")); err != nil {
			h.log.Error(fmt.Sprintf("Failed to send cancel success message to user %s", userID), err)
		}
	}
}

// sendDateTimePicker sends the datetime picker template message.
func (h *LineHandler) sendDateTimePicker(replyToken, userID string, precedingMessage linebot.SendingMessage) {
	now := time.Now()
	// Default initial time: 1 hour from now, rounded to the hour
	initialTime := now.Add(1 * time.Hour).Truncate(time.Hour)
	// Ensure initial time is not in the past
	if initialTime.Before(now) {
		initialTime = now.Add(time.Hour) // Set to 1 hour from now if the rounded hour is past
	}

	action := linebot.NewDatetimePickerAction(
		"日時選択",                                 // Label
		"datetime_postback",                    // Data (can be used to identify the action)
		"datetime",                             // Mode
		initialTime.Format("2006-01-02T15:04"), // Initial
		now.AddDate(1, 0, 0).Format("2006-01-02T15:04"), // Max (1 year from now)
		now.Format("2006-01-02T15:04"),                  // Min (now)
	)
	template := linebot.NewButtonsTemplate(
		"", // No image
		"", // No title
		"リマインド日時を選択してください", // Text
		action,
	)
	quickReply := linebot.NewQuickReplyItems(
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("キャンセル", "キャンセル")),
	)
	templateMessage := linebot.NewTemplateMessage("日時選択", template).WithQuickReplies(quickReply)

	messagesToSend := []linebot.SendingMessage{}
	if precedingMessage != nil {
		messagesToSend = append(messagesToSend, precedingMessage)
	}
	messagesToSend = append(messagesToSend, templateMessage)

	if err := h.lineClient.SendMessages(replyToken, messagesToSend...); err != nil {
		h.log.Error(fmt.Sprintf("Failed to send datetime picker to user %s", userID), err)
		// Attempt to revert status? Difficult state.
	}
}

// replyWithError sends a generic error message.
func (h *LineHandler) replyWithError(replyToken, userMessage string) {
	if err := h.lineClient.SendMessages(replyToken, linebot.NewTextMessage(userMessage)); err != nil {
		h.log.Error(fmt.Sprintf("Failed to send error reply message: %s", userMessage), err)
	}
}
