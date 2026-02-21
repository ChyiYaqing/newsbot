package scraper

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chyiyaqing/newsbot/internal/store"
	"github.com/mmcdole/gofeed"
)

const (
	maxConcurrency = 10
	httpTimeout    = 15 * time.Second
	maxArticles    = 10 // max articles per blog
)

// ScrapeBlogs fetches the latest articles from a list of blogs concurrently.
func ScrapeBlogs(ctx context.Context, blogs []store.Blog, db *store.Store) error {
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for _, blog := range blogs {
		wg.Add(1)
		go func(b store.Blog) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			articles, err := scrapeBlog(ctx, b.Domain)
			if err != nil {
				log.Printf("WARN: scrape %s: %v", b.Domain, err)
				return
			}

			for _, a := range articles {
				a.BlogDomain = b.Domain
				a.ScrapedAt = time.Now()
				if err := db.SaveArticle(a); err != nil {
					log.Printf("WARN: save article from %s: %v", b.Domain, err)
				}
			}
			log.Printf("Scraped %d articles from %s", len(articles), b.Domain)
		}(blog)
	}

	wg.Wait()
	return nil
}

func scrapeBlog(ctx context.Context, domain string) ([]store.Article, error) {
	// Try common feed paths.
	feedPaths := []string{"/feed", "/rss", "/atom.xml", "/feed.xml", "/rss.xml", "/index.xml", "/feeds/all.atom.xml"}

	for _, path := range feedPaths {
		url := "https://" + domain + path
		articles, err := parseFeed(ctx, url)
		if err == nil && len(articles) > 0 {
			return articles, nil
		}
	}

	// Try root URL for feeds embedded in HTML or auto-discovery.
	articles, err := parseFeed(ctx, "https://"+domain)
	if err == nil && len(articles) > 0 {
		return articles, nil
	}

	return nil, fmt.Errorf("no feed found for %s", domain)
}

func parseFeed(ctx context.Context, url string) ([]store.Article, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	fp := gofeed.NewParser()
	fp.Client = &http.Client{Timeout: httpTimeout}

	feed, err := fp.ParseURLWithContext(url, ctx)
	if err != nil {
		return nil, err
	}

	var articles []store.Article
	for i, item := range feed.Items {
		if i >= maxArticles {
			break
		}

		var publishedAt time.Time
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			publishedAt = *item.UpdatedParsed
		}

		summary := item.Description
		if summary == "" && item.Content != "" {
			summary = item.Content
		}
		summary = truncate(stripTags(summary), 500)

		link := item.Link
		if link == "" && len(item.Links) > 0 {
			link = item.Links[0]
		}

		articles = append(articles, store.Article{
			Title:       item.Title,
			URL:         link,
			Summary:     summary,
			PublishedAt: publishedAt,
		})
	}

	return articles, nil
}

func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
