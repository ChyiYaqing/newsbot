package telegram

import (
	"fmt"
	"strings"

	"github.com/chyiyaqing/newsbot/internal/ai"
	"github.com/chyiyaqing/newsbot/internal/store"
)

// FormatReport builds an HTML-formatted Telegram message from analyzed articles and trends.
func FormatReport(articles []store.ArticleWithAnalysis, trends *ai.TrendReport, window string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("<b>📡 Newsbot (%d new articles)</b>\n\n", len(articles)))

	// Top articles
	limit := 20
	if len(articles) < limit {
		limit = len(articles)
	}

	if limit > 0 {
		sb.WriteString("<b>Top Articles</b>\n\n")
	}

	for i := 0; i < limit; i++ {
		a := articles[i]
		sb.WriteString(fmt.Sprintf("<b>%d.</b> [%d | %s] %s\n",
			i+1,
			a.ArticleAnalysis.TotalScore,
			escapeHTML(a.ArticleAnalysis.Category),
			escapeHTML(a.Article.Title)))

		if a.ArticleAnalysis.TitleCN != "" {
			sb.WriteString(fmt.Sprintf("   中文: %s\n", escapeHTML(a.ArticleAnalysis.TitleCN)))
		}
		if a.ArticleAnalysis.RecommendReason != "" {
			sb.WriteString(fmt.Sprintf("   推荐: %s\n", escapeHTML(a.ArticleAnalysis.RecommendReason)))
		}
		sb.WriteString(fmt.Sprintf("   🔗 %s\n\n", a.Article.URL))
	}

	// Trends
	if trends != nil && len(trends.Trends) > 0 {
		sb.WriteString("<b>技术趋势</b>\n\n")
		for i, t := range trends.Trends {
			sb.WriteString(fmt.Sprintf("<b>%d. %s</b>\n", i+1, escapeHTML(t.Title)))
			sb.WriteString(fmt.Sprintf("   %s\n", escapeHTML(t.Description)))
			if len(t.Articles) > 0 {
				sb.WriteString(fmt.Sprintf("   相关: %s\n", escapeHTML(strings.Join(t.Articles, "; "))))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
