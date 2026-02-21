package scheduler

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/chyiyaqing/newsbot/internal/ai"
	"github.com/chyiyaqing/newsbot/internal/config"
	"github.com/chyiyaqing/newsbot/internal/hnpopular"
	"github.com/chyiyaqing/newsbot/internal/notify/telegram"
	"github.com/chyiyaqing/newsbot/internal/scraper"
	"github.com/chyiyaqing/newsbot/internal/store"
	"github.com/robfig/cron/v3"
)

// Run executes the full pipeline immediately, then starts a cron scheduler
// to repeat it periodically. It blocks until ctx is cancelled.
func Run(ctx context.Context, db *store.Store, cfg *config.Config, schedule string) error {
	if schedule == "" {
		schedule = "0 */6 * * *" // every 6 hours
	}

	// Run pipeline immediately on startup.
	log.Println("Running initial pipeline...")
	runPipeline(ctx, db, cfg)

	c := cron.New()

	_, err := c.AddFunc(schedule, func() {
		runPipeline(ctx, db, cfg)
	})
	if err != nil {
		return err
	}

	c.Start()
	log.Printf("Scheduler started with schedule: %s", schedule)

	<-ctx.Done()
	c.Stop()
	return nil
}

func runPipeline(ctx context.Context, db *store.Store, cfg *config.Config) {
	// Step 1: Fetch blogs
	log.Println("Pipeline: fetching blogs...")
	blogs, err := hnpopular.FetchTopBlogs(100)
	if err != nil {
		log.Printf("ERROR: fetch blogs: %v", err)
		return
	}
	if err := db.SaveBlogs(blogs); err != nil {
		log.Printf("ERROR: save blogs: %v", err)
		return
	}

	// Step 2: Scrape articles
	log.Println("Pipeline: scraping articles...")
	if err := scraper.ScrapeBlogs(ctx, blogs, db); err != nil {
		log.Printf("ERROR: scrape: %v", err)
	}

	// Step 3: Score and summarize (only articles not yet analyzed, 7days window)
	log.Println("Pipeline: analyzing articles...")
	articles, err := db.UnanalyzedArticles("7days")
	if err != nil {
		log.Printf("ERROR: get articles: %v", err)
		return
	}

	client := ai.NewClient(cfg.Ollama.Address, cfg.Ollama.Model, cfg.Ollama.Username, cfg.Ollama.Password)
	for _, article := range articles {
		scoreResult, err := client.ScoreArticle(ctx, article)
		if err != nil {
			log.Printf("WARNING: score %q: %v", article.Title, err)
			continue
		}

		totalScore := scoreResult.Relevance + scoreResult.Quality + scoreResult.Timeliness
		analysis := store.ArticleAnalysis{
			ArticleID:  article.ID,
			Relevance:  scoreResult.Relevance,
			Quality:    scoreResult.Quality,
			Timeliness: scoreResult.Timeliness,
			TotalScore: totalScore,
			Category:   scoreResult.Category,
			Keywords:   strings.Join(scoreResult.Keywords, ", "),
			AnalyzedAt: time.Now(),
		}

		summaryResult, err := client.SummarizeArticle(ctx, article)
		if err != nil {
			log.Printf("WARNING: summarize %q: %v", article.Title, err)
		} else {
			analysis.AISummary = summaryResult.Summary
			analysis.TitleCN = summaryResult.TitleCN
			analysis.RecommendReason = summaryResult.RecommendReason
		}

		if err := db.SaveArticleAnalysis(analysis); err != nil {
			log.Printf("WARNING: save analysis: %v", err)
		}
	}

	// Step 3b: Retry summaries for high-score articles that failed previously
	unsummarized, err := db.UnsummarizedHighScoreArticles("7days", 0)
	if err != nil {
		log.Printf("WARNING: get unsummarized articles: %v", err)
	} else if len(unsummarized) > 0 {
		log.Printf("Pipeline: retrying summaries for %d high-score articles...", len(unsummarized))
		for _, item := range unsummarized {
			summaryResult, err := client.SummarizeArticle(ctx, item.Article)
			if err != nil {
				log.Printf("WARNING: retry summarize %q: %v", item.Article.Title, err)
				continue
			}
			item.ArticleAnalysis.AISummary = summaryResult.Summary
			item.ArticleAnalysis.TitleCN = summaryResult.TitleCN
			item.ArticleAnalysis.RecommendReason = summaryResult.RecommendReason
			if err := db.SaveArticleAnalysis(item.ArticleAnalysis); err != nil {
				log.Printf("WARNING: save retry analysis: %v", err)
			}
		}
	}

	// Step 4: Send Telegram notification (only new/unnotified articles)
	tg := telegram.New(cfg.Telegram.BotToken, cfg.Telegram.ChatID)
	if tg != nil {
		newArticles, err := db.UnnotifiedAnalyses("7days")
		if err != nil {
			log.Printf("WARNING: get unnotified analyses: %v", err)
		} else if len(newArticles) == 0 {
			log.Println("Pipeline: no new articles to notify")
		} else {
			report, err := client.AnalyzeTrends(ctx, newArticles)
			if err != nil {
				log.Printf("WARNING: trend analysis for notification: %v", err)
			} else {
				msg := telegram.FormatReport(newArticles, report, "7days")
				if err := tg.Send(ctx, "", msg); err != nil {
					log.Printf("WARNING: telegram send: %v", err)
				} else {
					ids := make([]int64, len(newArticles))
					for i, a := range newArticles {
						ids[i] = a.Article.ID
					}
					if err := db.MarkNotified(ids); err != nil {
						log.Printf("WARNING: mark notified: %v", err)
					}
					log.Printf("Pipeline: notified %d new articles via Telegram", len(newArticles))
				}
			}
		}
	}

	log.Println("Pipeline: done")
}
