package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chyiyaqing/newsbot/internal/store"
)

type TrendReport struct {
	Trends []Trend `json:"trends"`
}

type Trend struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Articles    []string `json:"articles"`
}

const trendsSystemPrompt = `You are a technology trend analyst. Based on the following list of recently scored tech articles, identify 2-3 macro technology trends.

For each trend, provide:
- A concise title in Chinese (no pinyin, no parenthetical notes)
- A 2-3 sentence description in Chinese explaining the trend
- A list of related article titles from the input (use the exact original English titles)

CRITICAL: Respond ONLY with valid JSON. Do NOT add any text outside JSON string values. Do NOT add pinyin or annotations after closing quotes. Use only standard ASCII double quotes ("), never smart quotes. Example format:
{"trends":[{"title":"中文标题","description":"中文描述...","articles":["Article Title 1"]}]}`

// AnalyzeTrends identifies macro technology trends from scored articles.
func (c *Client) AnalyzeTrends(ctx context.Context, analyses []store.ArticleWithAnalysis) (*TrendReport, error) {
	var sb strings.Builder
	for i, a := range analyses {
		fmt.Fprintf(&sb, "%d. [%s] %s (score: %d, category: %s, keywords: %s)\n",
			i+1, a.Article.BlogDomain, a.Article.Title,
			a.ArticleAnalysis.TotalScore, a.ArticleAnalysis.Category, a.ArticleAnalysis.Keywords)
	}

	resp, err := c.ChatCompletion(ctx, trendsSystemPrompt, sb.String())
	if err != nil {
		return nil, fmt.Errorf("analyze trends: %w", err)
	}

	var report TrendReport
	if err := json.Unmarshal([]byte(resp), &report); err != nil {
		return nil, fmt.Errorf("parse trends: %w (raw: %s)", err, resp)
	}
	return &report, nil
}
