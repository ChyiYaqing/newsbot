package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/chyiyaqing/newsbot/internal/store"
)

type SummaryResult struct {
	Summary         string `json:"summary"`
	TitleCN         string `json:"title_cn"`
	RecommendReason string `json:"recommend_reason"`
}

const summarySystemPrompt = `You are a bilingual (English/Chinese) tech content summarizer.
For the given article, produce:
1. A structured summary in English (4-6 sentences covering the key points)
2. A Chinese translation of the article title
3. A recommendation reason in Chinese (1-2 sentences explaining why this article is worth reading)

Respond ONLY with valid JSON using standard ASCII double quotes. No other text:
{"summary":"...","title_cn":"...","recommend_reason":"..."}`

const maxRetries = 3

// SummarizeArticle generates a structured summary, Chinese title, and recommendation reason.
// Retries up to 3 times on failure.
func (c *Client) SummarizeArticle(ctx context.Context, article store.Article) (*SummaryResult, error) {
	userPrompt := fmt.Sprintf("Title: %s\nSource: %s\nContent: %s",
		article.Title, article.BlogDomain, article.Summary)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := c.ChatCompletion(ctx, summarySystemPrompt, userPrompt)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: %w", attempt, err)
			log.Printf("  Summarize retry %d/%d for %q: %v", attempt, maxRetries, article.Title, err)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}

		var result SummaryResult
		if err := json.Unmarshal([]byte(resp), &result); err != nil {
			lastErr = fmt.Errorf("attempt %d parse: %w (raw: %s)", attempt, err, resp)
			log.Printf("  Summarize retry %d/%d for %q: JSON parse error", attempt, maxRetries, article.Title)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}
		return &result, nil
	}
	return nil, fmt.Errorf("summarize %q failed after %d attempts: %w", article.Title, maxRetries, lastErr)
}
