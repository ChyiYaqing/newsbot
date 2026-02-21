package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	model      string
	username   string
	password   string
	httpClient *http.Client
}

func NewClient(baseURL, model, username, password string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		username:   username,
		password:   password,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// ChatCompletion sends a prompt to the Ollama OpenAI-compatible endpoint
// and returns the assistant's response text.
func (c *Client) ChatCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	req := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		httpReq.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return "", fmt.Errorf("ollama returned %d: %s", resp.StatusCode, buf.String())
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	cleaned := stripCodeFence(chatResp.Choices[0].Message.Content)
	cleaned = sanitizeJSON(cleaned)
	return cleaned, nil
}

var codeFenceRe = regexp.MustCompile("(?s)^```(?:json)?\\s*\n?(.*?)\\s*```$")

// sanitizeJSON replaces Unicode smart quotes and other problematic characters
// that LLMs sometimes produce in JSON output with their ASCII equivalents.
func sanitizeJSON(s string) string {
	s = strings.ReplaceAll(s, "\u201c", "\"") // left double quotation mark
	s = strings.ReplaceAll(s, "\u201d", "\"") // right double quotation mark
	s = strings.ReplaceAll(s, "\u2018", "'")  // left single quotation mark
	s = strings.ReplaceAll(s, "\u2019", "'")  // right single quotation mark
	return s
}

// stripCodeFence removes markdown code fences from LLM responses.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if m := codeFenceRe.FindStringSubmatch(s); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return s
}
