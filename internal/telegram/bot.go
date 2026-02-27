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

// TelegramUpdate represents a getUpdates response item.
type TelegramUpdate struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		MessageID int `json:"message_id"`
		Chat      *struct {
			ID    int64  `json:"id"`
			Type  string `json:"type"`
			Title string `json:"title"`
		} `json:"chat"`
		From *struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"from"`
		Text string `json:"text"`
	} `json:"message"`
}

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
	return c.SendMessageToChat(ctx, c.chatID, text)
}

// SendMessageToChat sends a message to a specific chat (for /stats reply).
func (c *Client) SendMessageToChat(ctx context.Context, chatID int64, text string) error {
	return c.sendMessageToChat(ctx, chatID, text, nil)
}

// SendMessageWithURLButton sends a message with a single inline URL button.
func (c *Client) SendMessageWithURLButton(ctx context.Context, chatID int64, text, buttonText, buttonURL string) error {
	replyMarkup := map[string]any{
		"inline_keyboard": [][]map[string]string{
			{
				{
					"text": buttonText,
					"url":  buttonURL,
				},
			},
		},
	}
	return c.sendMessageToChat(ctx, chatID, text, replyMarkup)
}

func (c *Client) sendMessageToChat(ctx context.Context, chatID int64, text string, replyMarkup any) error {
	if c.botToken == "" || chatID == 0 {
		return nil
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.botToken)
	body := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if replyMarkup != nil {
		body["reply_markup"] = replyMarkup
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

// GetUpdates fetches new updates (for /stats command handling).
func (c *Client) GetUpdates(ctx context.Context, offset int) ([]TelegramUpdate, error) {
	if c.botToken == "" {
		return nil, nil
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=30&offset=%d", c.botToken, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram getUpdates not ok")
	}
	return out.Result, nil
}

// NotifyOrderAssigned sends a Telegram notification when a boosting request is assigned to you.
func (c *Client) NotifyOrderAssigned(ctx context.Context, requestID, buyerUsername, categoryTitle, gameID string) {
	c.NotifyOrderAssignedWithDetails(ctx, requestID, buyerUsername, categoryTitle, gameID, 0, "", "", "")
}

// NotifyOrderAssignedWithDetails sends a rich notification with offer details.
func (c *Client) NotifyOrderAssignedWithDetails(ctx context.Context, requestID, buyerUsername, categoryTitle, gameID string, offerPrice float64, currentRank, desiredRank, currentRR string) {
	text := "<b>Yeni siparis sana atandi!</b>\n\n"
	text += fmt.Sprintf("Talep ID: <code>%s</code>\n", requestID)
	text += fmt.Sprintf("Alici: %s\n", buyerUsername)
	text += fmt.Sprintf("Kategori: %s\n", categoryTitle)
	text += fmt.Sprintf("Oyun ID: %s\n", gameID)
	if offerPrice > 0 {
		text += fmt.Sprintf("Fiyat: <b>$%.2f</b>\n", offerPrice)
	}
	if currentRank != "" || desiredRank != "" {
		text += fmt.Sprintf("Rank: <b>%s</b> ➜ <b>%s</b>\n", currentRank, desiredRank)
	}
	if currentRR != "" {
		text += fmt.Sprintf("RR: <b>%s</b>\n", currentRR)
	}

	if err := c.SendMessage(ctx, text); err != nil {
		c.log.Errorf("telegram notify error: %v", err)
	}
}
