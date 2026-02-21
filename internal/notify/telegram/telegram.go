package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client sends messages via the Telegram Bot API.
type Client struct {
	botToken   string
	chatID     string
	httpClient *http.Client
}

// New creates a Telegram notifier. Returns nil if token or chatID is empty.
func New(botToken, chatID string) *Client {
	if botToken == "" || chatID == "" {
		return nil
	}
	return &Client{
		botToken:   botToken,
		chatID:     chatID,
		httpClient: &http.Client{},
	}
}

type sendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

type apiResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

const maxMessageLen = 4096

// Send sends a message to the configured chat. Messages longer than 4096
// characters are split on paragraph boundaries.
func (c *Client) Send(ctx context.Context, title, body string) error {
	text := body
	if title != "" {
		text = "<b>" + escapeHTML(title) + "</b>\n\n" + body
	}

	chunks := splitMessage(text, maxMessageLen)
	for _, chunk := range chunks {
		if err := c.sendRaw(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) sendRaw(ctx context.Context, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.botToken)

	payload, err := json.Marshal(sendMessageRequest{
		ChatID:    c.chatID,
		Text:      text,
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var apiResp apiResponse
		json.Unmarshal(respBody, &apiResp)
		return fmt.Errorf("telegram API %d: %s", resp.StatusCode, apiResp.Description)
	}
	return nil
}

// splitMessage breaks text into chunks of at most maxLen characters,
// splitting on paragraph boundaries ("\n\n") when possible.
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}

		// Find a split point at a paragraph boundary
		cut := maxLen
		if idx := strings.LastIndex(text[:maxLen], "\n\n"); idx > 0 {
			cut = idx
		} else if idx := strings.LastIndex(text[:maxLen], "\n"); idx > 0 {
			cut = idx
		}

		chunks = append(chunks, text[:cut])
		text = strings.TrimLeft(text[cut:], "\n")
	}
	return chunks
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
