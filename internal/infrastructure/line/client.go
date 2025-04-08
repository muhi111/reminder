package line

import (
	"net/http"
	"os"
	"reminder/internal/pkg/logger"
	"sync"

	"github.com/line/line-bot-sdk-go/v7/linebot"
)

// Client wraps the linebot.Client.
type Client struct {
	*linebot.Client
	log logger.Logger
}

var (
	lineClientInstance *Client
	once               sync.Once
)

// NewClient creates a new singleton instance of the LINE Bot client.
// It reads credentials from environment variables.
func NewClient(log logger.Logger) *Client {
	once.Do(func() {
		channelSecret := os.Getenv("CHANNEL_SECRET")
		channelToken := os.Getenv("CHANNEL_ACCESS_TOKEN")

		if channelSecret == "" || channelToken == "" {
			log.Error("🔴 ERROR: CHANNEL_SECRET and CHANNEL_ACCESS_TOKEN environment variables must be set", nil)
			os.Exit(1)
		}

		bot, err := linebot.New(channelSecret, channelToken)
		if err != nil {
			log.Error("🔴 ERROR: Failed to create LINE Bot client", err)
			os.Exit(1)
		}
		log.Info("Successfully created LINE Bot client.")
		lineClientInstance = &Client{
			Client: bot,
			log:    log,
		}
	})
	return lineClientInstance
}

// SendMessages sends one or more messages using the ReplyMessage API.
func (c *Client) SendMessages(replyToken string, messages ...linebot.SendingMessage) error {
	_, err := c.ReplyMessage(replyToken, messages...).Do()
	if err != nil {
		return err // Return the error for the caller to handle
	}
	c.log.Debug("Successfully sent reply message.")
	return nil
}

// PushMessages sends one or more messages using the PushMessage API.
func (c *Client) PushMessages(to string, messages ...linebot.SendingMessage) error {
	_, err := c.PushMessage(to, messages...).Do()
	if err != nil {
		return err // Return the error for the caller to handle
	}
	c.log.Debug("Successfully sent push message.")
	return nil
}

// ParseRequest parses incoming webhook requests.
func (c *Client) ParseRequest(r *http.Request) ([]*linebot.Event, error) {
	return c.Client.ParseRequest(r)
}
