package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chyiyaqing/newsbot/internal/store"
)

type ScoreResult struct {
	Relevance  int      `json:"relevance"`
	Quality    int      `json:"quality"`
	Timeliness int      `json:"timeliness"`
	Category   string   `json:"category"`
	Keywords   []string `json:"keywords"`
}

const scoreSystemPrompt = `You are a tech news analyst. Score this article on three dimensions (1-10):
- relevance: how relevant to software engineers and tech professionals
- quality: writing quality, depth, and informativeness
- timeliness: how current and timely the topic is

Also classify into one category (e.g. "AI/ML", "Systems", "Web", "Security", "DevOps", "Programming", "Data", "Cloud", "Open Source", "Career") and extract 3-5 keywords.

Respond ONLY with valid JSON using standard ASCII double quotes. No other text:
{"relevance":N,"quality":N,"timeliness":N,"category":"...","keywords":["...","..."]}`

// ScoreArticle sends article info to the LLM for scoring and classification.
func (c *Client) ScoreArticle(ctx context.Context, article store.Article) (*ScoreResult, error) {
	userPrompt := fmt.Sprintf("Title: %s\nSource: %s\nSummary: %s",
		article.Title, article.BlogDomain, article.Summary)

	resp, err := c.ChatCompletion(ctx, scoreSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("score article %q: %w", article.Title, err)
	}

	var result ScoreResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("parse score for %q: %w (raw: %s)", article.Title, err, resp)
	}
	return &result, nil
}
