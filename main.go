package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chyiyaqing/newsbot/internal/ai"
	"github.com/chyiyaqing/newsbot/internal/config"
	"github.com/chyiyaqing/newsbot/internal/hnpopular"
	"github.com/chyiyaqing/newsbot/internal/notify/telegram"
	"github.com/chyiyaqing/newsbot/internal/scheduler"
	"github.com/chyiyaqing/newsbot/internal/scraper"
	"github.com/chyiyaqing/newsbot/internal/server"
	"github.com/chyiyaqing/newsbot/internal/store"
)

const dbPath = "data/newsbot.db"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cfg, err := config.Load("newsbot.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	switch os.Args[1] {
	case "fetch-blogs":
		cmdFetchBlogs(db)
	case "scrape":
		cmdScrape(db)
	case "analyze":
		window := "24h"
		if len(os.Args) > 2 {
			window = os.Args[2]
		}
		cmdAnalyze(db, cfg, window)
	case "report":
		window := "24h"
		if len(os.Args) > 2 {
			window = os.Args[2]
		}
		cmdReport(db, cfg, window)
	case "notify":
		window := "24h"
		if len(os.Args) > 2 {
			window = os.Args[2]
		}
		cmdNotify(db, cfg, window)
	case "run":
		cmdRun(db, cfg)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: newsbot <command>

Commands:
  fetch-blogs          Fetch top blogs from HN Popularity and store them
  scrape               Scrape latest articles from all stored blogs
  analyze [24h|3days|7days]  Score and summarize articles with AI
  report  [24h|3days|7days]  Generate trend report from analyzed articles
  notify  [24h|3days|7days]  Send report via Telegram
  run     [cron-expr]        Start scheduler (cron mode)
`)
}

func cmdFetchBlogs(db *store.Store) {
	blogs, err := hnpopular.FetchTopBlogs(100)
	if err != nil {
		log.Fatalf("Failed to fetch blogs: %v", err)
	}

	if err := db.SaveBlogs(blogs); err != nil {
		log.Fatalf("Failed to save blogs: %v", err)
	}

	log.Printf("Saved %d blogs", len(blogs))
	for _, b := range blogs {
		fmt.Printf("#%d %s (score: %d, author: %s)\n", b.Rank, b.Domain, b.Score, b.Author)
	}
}

func cmdScrape(db *store.Store) {
	blogs, err := db.ListBlogs()
	if err != nil {
		log.Fatalf("Failed to list blogs: %v", err)
	}

	if len(blogs) == 0 {
		log.Fatal("No blogs in database. Run 'newsbot fetch-blogs' first.")
	}

	ctx := context.Background()
	if err := scraper.ScrapeBlogs(ctx, blogs, db); err != nil {
		log.Fatalf("Scrape failed: %v", err)
	}

	articles, err := db.LatestArticles(20)
	if err != nil {
		log.Fatalf("Failed to list articles: %v", err)
	}

	fmt.Printf("\nLatest %d articles:\n", len(articles))
	for _, a := range articles {
		fmt.Printf("  [%s] %s\n    %s\n", a.BlogDomain, a.Title, a.URL)
	}
}

func cmdAnalyze(db *store.Store, cfg *config.Config, window string) {
	articles, err := db.ArticlesByTimeWindow(window)
	if err != nil {
		log.Fatalf("Failed to get articles: %v", err)
	}

	if len(articles) == 0 {
		log.Fatalf("No articles found in %s window. Run 'newsbot scrape' first.", window)
	}

	client := ai.NewClient(cfg.Ollama.Address, cfg.Ollama.Model, cfg.Ollama.Username, cfg.Ollama.Password)
	ctx := context.Background()

	log.Printf("Analyzing %d articles from %s window...", len(articles), window)

	scored := 0
	summarized := 0
	for i, article := range articles {
		log.Printf("[%d/%d] Scoring: %s", i+1, len(articles), article.Title)

		scoreResult, err := client.ScoreArticle(ctx, article)
		if err != nil {
			log.Printf("  WARNING: skip scoring: %v", err)
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

		log.Printf("  Generating summary...")
		summaryResult, err := client.SummarizeArticle(ctx, article)
		if err != nil {
			log.Printf("  WARNING: skip summary: %v", err)
		} else {
			analysis.AISummary = summaryResult.Summary
			analysis.TitleCN = summaryResult.TitleCN
			analysis.RecommendReason = summaryResult.RecommendReason
			summarized++
		}

		if err := db.SaveArticleAnalysis(analysis); err != nil {
			log.Printf("  WARNING: save analysis: %v", err)
			continue
		}
		scored++

		fmt.Printf("  [%d] %s (%s) — %s\n",
			totalScore, article.Title, scoreResult.Category, strings.Join(scoreResult.Keywords, ", "))
	}

	log.Printf("Done: %d scored, %d summarized", scored, summarized)

	// Retry summaries for high-score articles that failed previously
	unsummarized, err := db.UnsummarizedHighScoreArticles(window, 0)
	if err != nil {
		log.Printf("WARNING: get unsummarized articles: %v", err)
		return
	}
	if len(unsummarized) > 0 {
		log.Printf("Retrying summaries for %d high-score articles...", len(unsummarized))
		for _, item := range unsummarized {
			log.Printf("  Summarizing: %s (score %d)", item.Article.Title, item.ArticleAnalysis.TotalScore)
			summaryResult, err := client.SummarizeArticle(ctx, item.Article)
			if err != nil {
				log.Printf("  WARNING: retry summarize: %v", err)
				continue
			}
			item.ArticleAnalysis.AISummary = summaryResult.Summary
			item.ArticleAnalysis.TitleCN = summaryResult.TitleCN
			item.ArticleAnalysis.RecommendReason = summaryResult.RecommendReason
			if err := db.SaveArticleAnalysis(item.ArticleAnalysis); err != nil {
				log.Printf("  WARNING: save: %v", err)
			} else {
				summarized++
			}
		}
		log.Printf("Retry done, total summarized: %d", summarized)
	}
}

func cmdReport(db *store.Store, cfg *config.Config, window string) {
	analyses, err := db.AnalysesByTimeWindow(window)
	if err != nil {
		log.Fatalf("Failed to get analyses: %v", err)
	}

	if len(analyses) == 0 {
		log.Fatalf("No analyzed articles in %s window. Run 'newsbot analyze %s' first.", window, window)
	}

	// Print top articles
	fmt.Printf("\n=== Top Articles (%s) ===\n\n", window)
	limit := 20
	if len(analyses) < limit {
		limit = len(analyses)
	}
	for i := 0; i < limit; i++ {
		a := analyses[i]
		fmt.Printf("%d. [Score: %d | %s] %s\n", i+1, a.ArticleAnalysis.TotalScore, a.ArticleAnalysis.Category, a.Article.Title)
		if a.ArticleAnalysis.TitleCN != "" {
			fmt.Printf("   中文: %s\n", a.ArticleAnalysis.TitleCN)
		}
		if a.ArticleAnalysis.RecommendReason != "" {
			fmt.Printf("   推荐: %s\n", a.ArticleAnalysis.RecommendReason)
		}
		if a.ArticleAnalysis.AISummary != "" {
			fmt.Printf("   摘要: %s\n", a.ArticleAnalysis.AISummary)
		}
		fmt.Printf("   链接: %s\n\n", a.Article.URL)
	}

	// Generate trend report
	client := ai.NewClient(cfg.Ollama.Address, cfg.Ollama.Model, cfg.Ollama.Username, cfg.Ollama.Password)
	ctx := context.Background()

	log.Println("Generating trend report...")
	report, err := client.AnalyzeTrends(ctx, analyses)
	if err != nil {
		log.Fatalf("Failed to analyze trends: %v", err)
	}

	fmt.Printf("\n=== 技术趋势总结 (%s) ===\n\n", window)
	for i, t := range report.Trends {
		fmt.Printf("%d. %s\n", i+1, t.Title)
		fmt.Printf("   %s\n", t.Description)
		if len(t.Articles) > 0 {
			fmt.Printf("   相关文章: %s\n", strings.Join(t.Articles, "; "))
		}
		fmt.Println()
	}

	// Auto-send to Telegram if configured (only unnotified articles)
	if tg := telegram.New(cfg.Telegram.BotToken, cfg.Telegram.ChatID); tg != nil {
		newArticles, err := db.UnnotifiedAnalyses(window)
		if err != nil {
			log.Printf("WARNING: get unnotified analyses: %v", err)
		} else if len(newArticles) == 0 {
			log.Println("No new articles to send to Telegram")
		} else {
			msg := telegram.FormatReport(newArticles, report, window)
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
				log.Printf("Report sent to Telegram (%d new articles)", len(newArticles))
			}
		}
	}
}

func cmdNotify(db *store.Store, cfg *config.Config, window string) {
	tg := telegram.New(cfg.Telegram.BotToken, cfg.Telegram.ChatID)
	if tg == nil {
		log.Fatal("Telegram not configured. Set TG_BOT_TOKEN and TG_CHAT_ID in .env")
	}

	newArticles, err := db.UnnotifiedAnalyses(window)
	if err != nil {
		log.Fatalf("Failed to get unnotified analyses: %v", err)
	}
	if len(newArticles) == 0 {
		log.Printf("No new articles to notify in %s window.", window)
		return
	}

	client := ai.NewClient(cfg.Ollama.Address, cfg.Ollama.Model, cfg.Ollama.Username, cfg.Ollama.Password)
	ctx := context.Background()

	log.Printf("Generating trend report for %d new articles...", len(newArticles))
	report, err := client.AnalyzeTrends(ctx, newArticles)
	if err != nil {
		log.Fatalf("Failed to analyze trends: %v", err)
	}

	msg := telegram.FormatReport(newArticles, report, window)
	if err := tg.Send(ctx, "", msg); err != nil {
		log.Fatalf("Failed to send Telegram notification: %v", err)
	}

	ids := make([]int64, len(newArticles))
	for i, a := range newArticles {
		ids[i] = a.Article.ID
	}
	if err := db.MarkNotified(ids); err != nil {
		log.Printf("WARNING: mark notified: %v", err)
	}
	log.Printf("Notified %d new articles via Telegram", len(newArticles))
}

func cmdRun(db *store.Store, cfg *config.Config) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	schedule := ""
	httpAddr := ":8080"

	// Parse optional args: [cron-expr] [--addr=:8080]
	for _, arg := range os.Args[2:] {
		if strings.HasPrefix(arg, "--addr=") {
			httpAddr = strings.TrimPrefix(arg, "--addr=")
		} else {
			schedule = arg
		}
	}

	// Start HTTP server in background
	srv := server.New(db, httpAddr)
	go func() {
		if err := srv.Start(ctx); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Start cron scheduler (blocks until ctx is cancelled)
	if err := scheduler.Run(ctx, db, cfg, schedule); err != nil {
		log.Fatalf("Scheduler error: %v", err)
	}
}
