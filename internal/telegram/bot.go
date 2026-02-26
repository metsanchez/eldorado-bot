package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"eldorado-bot/internal/logger"
)

type Client struct {
	botToken   string
	chatID     int64
	httpClient *http.Client
	log        *logger.Logger
	enabled    bool
}

func NewClient(botToken string, chatID int64, log *logger.Logger) *Client {
	enabled := botToken != "" && chatID != 0
	return &Client{
		botToken: botToken,
		chatID:   chatID,
		log:      log,
		enabled:  enabled,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) SendMessage(ctx context.Context, text string) error {
	if !c.enabled {
		return nil
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.botToken)
	body := map[string]any{
		"chat_id":    c.chatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage: status %d", resp.StatusCode)
	}

	return nil
}

// NotifyOrderAssigned sends a Telegram notification when a boosting request is assigned to you.
func (c *Client) NotifyOrderAssigned(ctx context.Context, requestID, buyerUsername, categoryTitle, gameID string) {
	text := fmt.Sprintf(
		"<b>Yeni siparis sana atandi!</b>\n\n"+
			"Talep ID: <code>%s</code>\n"+
			"Alici: %s\n"+
			"Kategori: %s\n"+
			"Oyun ID: %s",
		requestID,
		buyerUsername,
		categoryTitle,
		gameID,
	)

	if err := c.SendMessage(ctx, text); err != nil {
		c.log.Errorf("telegram notify error: %v", err)
	}
}
