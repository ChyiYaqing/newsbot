package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Blog struct {
	ID     int64
	Domain string
	Score  int
	Author string
	Rank   int
}

type Article struct {
	ID          int64
	BlogDomain  string
	Title       string
	URL         string
	Summary     string
	PublishedAt time.Time
	ScrapedAt   time.Time
}

type ArticleAnalysis struct {
	ID              int64
	ArticleID       int64
	Relevance       int
	Quality         int
	Timeliness      int
	TotalScore      int
	Category        string
	Keywords        string
	AISummary       string
	TitleCN         string
	RecommendReason string
	AnalyzedAt      time.Time
	NotifiedAt      *time.Time
}

type ArticleWithAnalysis struct {
	Article
	ArticleAnalysis
}

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite only allows one writer at a time. Limit pool to 1 connection
	// so concurrent goroutines queue at the Go level instead of hitting SQLITE_BUSY.
	db.SetMaxOpenConns(1)

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS blogs (
			id     INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT NOT NULL UNIQUE,
			score  INTEGER NOT NULL DEFAULT 0,
			author TEXT NOT NULL DEFAULT '',
			rank   INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS articles (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			blog_domain  TEXT NOT NULL,
			title        TEXT NOT NULL,
			url          TEXT NOT NULL UNIQUE,
			summary      TEXT NOT NULL DEFAULT '',
			published_at DATETIME,
			scraped_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_articles_blog_domain ON articles(blog_domain);
		CREATE INDEX IF NOT EXISTS idx_articles_published_at ON articles(published_at);

		CREATE TABLE IF NOT EXISTS article_analysis (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			article_id       INTEGER NOT NULL UNIQUE REFERENCES articles(id),
			relevance        INTEGER NOT NULL DEFAULT 0,
			quality          INTEGER NOT NULL DEFAULT 0,
			timeliness       INTEGER NOT NULL DEFAULT 0,
			total_score      INTEGER NOT NULL DEFAULT 0,
			category         TEXT NOT NULL DEFAULT '',
			keywords         TEXT NOT NULL DEFAULT '',
			ai_summary       TEXT NOT NULL DEFAULT '',
			title_cn         TEXT NOT NULL DEFAULT '',
			recommend_reason TEXT NOT NULL DEFAULT '',
			analyzed_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_analysis_total_score ON article_analysis(total_score DESC);
		CREATE INDEX IF NOT EXISTS idx_analysis_article_id ON article_analysis(article_id);
	`)
	if err != nil {
		return err
	}

	// Add notified_at column (ignore error if column already exists).
	s.db.Exec("ALTER TABLE article_analysis ADD COLUMN notified_at DATETIME")

	// Normalize existing published_at timestamps from Go's default format to RFC3339.
	// e.g. "2026-02-17 12:01:45 +0000 UTC" → "2026-02-17T12:01:45Z"
	s.db.Exec(`UPDATE articles SET published_at = REPLACE(SUBSTR(published_at, 1, 19), ' ', 'T') || 'Z' WHERE published_at LIKE '% +0000 UTC'`)

	return nil
}

// SaveBlogs upserts a list of blogs.
func (s *Store) SaveBlogs(blogs []Blog) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO blogs (domain, score, author, rank)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			score  = excluded.score,
			author = excluded.author,
			rank   = excluded.rank
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, b := range blogs {
		if _, err := stmt.Exec(b.Domain, b.Score, b.Author, b.Rank); err != nil {
			return fmt.Errorf("save blog %s: %w", b.Domain, err)
		}
	}
	return tx.Commit()
}

// SaveArticle inserts an article if its URL doesn't already exist.
// Times are stored in RFC3339 UTC format for consistent comparison.
func (s *Store) SaveArticle(a Article) error {
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO articles (blog_domain, title, url, summary, published_at, scraped_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, a.BlogDomain, a.Title, a.URL, a.Summary,
		a.PublishedAt.UTC().Format(time.RFC3339),
		a.ScrapedAt.UTC().Format(time.RFC3339))
	return err
}

// ListBlogs returns all blogs ordered by rank.
func (s *Store) ListBlogs() ([]Blog, error) {
	rows, err := s.db.Query("SELECT id, domain, score, author, rank FROM blogs ORDER BY rank ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blogs []Blog
	for rows.Next() {
		var b Blog
		if err := rows.Scan(&b.ID, &b.Domain, &b.Score, &b.Author, &b.Rank); err != nil {
			return nil, err
		}
		blogs = append(blogs, b)
	}
	return blogs, rows.Err()
}

// LatestArticles returns the most recent articles.
func (s *Store) LatestArticles(limit int) ([]Article, error) {
	rows, err := s.db.Query(`
		SELECT id, blog_domain, title, url, summary, published_at, scraped_at
		FROM articles
		ORDER BY published_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.BlogDomain, &a.Title, &a.URL, &a.Summary, &a.PublishedAt, &a.ScrapedAt); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// ArticlesByTimeWindow returns articles published within the given window.
// Supported windows: "24h", "3days", "7days".
func (s *Store) ArticlesByTimeWindow(window string) ([]Article, error) {
	cutoff, err := windowCutoff(window)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT id, blog_domain, title, url, summary, published_at, scraped_at
		FROM articles
		WHERE published_at >= ?
		ORDER BY published_at DESC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.BlogDomain, &a.Title, &a.URL, &a.Summary, &a.PublishedAt, &a.ScrapedAt); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// UnanalyzedArticles returns articles in the time window that have no analysis yet.
func (s *Store) UnanalyzedArticles(window string) ([]Article, error) {
	cutoff, err := windowCutoff(window)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT a.id, a.blog_domain, a.title, a.url, a.summary, a.published_at, a.scraped_at
		FROM articles a
		LEFT JOIN article_analysis aa ON a.id = aa.article_id
		WHERE a.published_at >= ? AND aa.id IS NULL
		ORDER BY a.published_at DESC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.BlogDomain, &a.Title, &a.URL, &a.Summary, &a.PublishedAt, &a.ScrapedAt); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// GetArticleWithAnalysis returns a single article with its analysis by article ID.
func (s *Store) GetArticleWithAnalysis(id int64) (*ArticleWithAnalysis, error) {
	row := s.db.QueryRow(`
		SELECT a.id, a.blog_domain, a.title, a.url, a.summary, a.published_at, a.scraped_at,
		       aa.id, aa.article_id, aa.relevance, aa.quality, aa.timeliness, aa.total_score,
		       aa.category, aa.keywords, aa.ai_summary, aa.title_cn, aa.recommend_reason, aa.analyzed_at
		FROM articles a
		JOIN article_analysis aa ON a.id = aa.article_id
		WHERE a.id = ?
	`, id)

	var r ArticleWithAnalysis
	err := row.Scan(
		&r.Article.ID, &r.Article.BlogDomain, &r.Article.Title, &r.Article.URL,
		&r.Article.Summary, &r.Article.PublishedAt, &r.Article.ScrapedAt,
		&r.ArticleAnalysis.ID, &r.ArticleAnalysis.ArticleID,
		&r.ArticleAnalysis.Relevance, &r.ArticleAnalysis.Quality, &r.ArticleAnalysis.Timeliness,
		&r.ArticleAnalysis.TotalScore, &r.ArticleAnalysis.Category, &r.ArticleAnalysis.Keywords,
		&r.ArticleAnalysis.AISummary, &r.ArticleAnalysis.TitleCN, &r.ArticleAnalysis.RecommendReason,
		&r.ArticleAnalysis.AnalyzedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// UnsummarizedHighScoreArticles returns articles that have been scored (total >= minScore)
// but are missing a summary, within the given time window.
func (s *Store) UnsummarizedHighScoreArticles(window string, minScore int) ([]ArticleWithAnalysis, error) {
	cutoff, err := windowCutoff(window)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT a.id, a.blog_domain, a.title, a.url, a.summary, a.published_at, a.scraped_at,
		       aa.id, aa.article_id, aa.relevance, aa.quality, aa.timeliness, aa.total_score,
		       aa.category, aa.keywords, aa.ai_summary, aa.title_cn, aa.recommend_reason, aa.analyzed_at
		FROM articles a
		JOIN article_analysis aa ON a.id = aa.article_id
		WHERE a.published_at >= ?
		  AND aa.total_score >= ?
		  AND aa.ai_summary = ''
		ORDER BY aa.total_score DESC
	`, cutoff, minScore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ArticleWithAnalysis
	for rows.Next() {
		var r ArticleWithAnalysis
		if err := rows.Scan(
			&r.Article.ID, &r.Article.BlogDomain, &r.Article.Title, &r.Article.URL,
			&r.Article.Summary, &r.Article.PublishedAt, &r.Article.ScrapedAt,
			&r.ArticleAnalysis.ID, &r.ArticleAnalysis.ArticleID,
			&r.ArticleAnalysis.Relevance, &r.ArticleAnalysis.Quality, &r.ArticleAnalysis.Timeliness,
			&r.ArticleAnalysis.TotalScore, &r.ArticleAnalysis.Category, &r.ArticleAnalysis.Keywords,
			&r.ArticleAnalysis.AISummary, &r.ArticleAnalysis.TitleCN, &r.ArticleAnalysis.RecommendReason,
			&r.ArticleAnalysis.AnalyzedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// SaveArticleAnalysis upserts an analysis result for an article.
func (s *Store) SaveArticleAnalysis(a ArticleAnalysis) error {
	_, err := s.db.Exec(`
		INSERT INTO article_analysis (article_id, relevance, quality, timeliness, total_score, category, keywords, ai_summary, title_cn, recommend_reason, analyzed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(article_id) DO UPDATE SET
			relevance        = excluded.relevance,
			quality          = excluded.quality,
			timeliness       = excluded.timeliness,
			total_score      = excluded.total_score,
			category         = excluded.category,
			keywords         = excluded.keywords,
			ai_summary       = excluded.ai_summary,
			title_cn         = excluded.title_cn,
			recommend_reason = excluded.recommend_reason,
			analyzed_at      = excluded.analyzed_at
	`, a.ArticleID, a.Relevance, a.Quality, a.Timeliness, a.TotalScore, a.Category, a.Keywords, a.AISummary, a.TitleCN, a.RecommendReason, a.AnalyzedAt.UTC().Format(time.RFC3339))
	return err
}

// TopScoredArticles returns top scored articles with their analysis within a time window.
func (s *Store) TopScoredArticles(limit int, window string) ([]ArticleWithAnalysis, error) {
	cutoff, err := windowCutoff(window)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT a.id, a.blog_domain, a.title, a.url, a.summary, a.published_at, a.scraped_at,
		       aa.id, aa.article_id, aa.relevance, aa.quality, aa.timeliness, aa.total_score,
		       aa.category, aa.keywords, aa.ai_summary, aa.title_cn, aa.recommend_reason, aa.analyzed_at
		FROM articles a
		JOIN article_analysis aa ON a.id = aa.article_id
		WHERE a.published_at >= ?
		ORDER BY aa.total_score DESC
		LIMIT ?
	`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ArticleWithAnalysis
	for rows.Next() {
		var r ArticleWithAnalysis
		if err := rows.Scan(
			&r.Article.ID, &r.Article.BlogDomain, &r.Article.Title, &r.Article.URL,
			&r.Article.Summary, &r.Article.PublishedAt, &r.Article.ScrapedAt,
			&r.ArticleAnalysis.ID, &r.ArticleAnalysis.ArticleID,
			&r.ArticleAnalysis.Relevance, &r.ArticleAnalysis.Quality, &r.ArticleAnalysis.Timeliness,
			&r.ArticleAnalysis.TotalScore, &r.ArticleAnalysis.Category, &r.ArticleAnalysis.Keywords,
			&r.ArticleAnalysis.AISummary, &r.ArticleAnalysis.TitleCN, &r.ArticleAnalysis.RecommendReason,
			&r.ArticleAnalysis.AnalyzedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// AnalysesByTimeWindow returns all analyses for articles in the given time window.
func (s *Store) AnalysesByTimeWindow(window string) ([]ArticleWithAnalysis, error) {
	return s.TopScoredArticles(1000, window)
}

// UnnotifiedAnalyses returns top scored articles that have not been notified yet within a time window.
func (s *Store) UnnotifiedAnalyses(window string) ([]ArticleWithAnalysis, error) {
	cutoff, err := windowCutoff(window)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT a.id, a.blog_domain, a.title, a.url, a.summary, a.published_at, a.scraped_at,
		       aa.id, aa.article_id, aa.relevance, aa.quality, aa.timeliness, aa.total_score,
		       aa.category, aa.keywords, aa.ai_summary, aa.title_cn, aa.recommend_reason, aa.analyzed_at
		FROM articles a
		JOIN article_analysis aa ON a.id = aa.article_id
		WHERE a.published_at >= ?
		  AND aa.notified_at IS NULL
		ORDER BY aa.total_score DESC
		LIMIT 20
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ArticleWithAnalysis
	for rows.Next() {
		var r ArticleWithAnalysis
		if err := rows.Scan(
			&r.Article.ID, &r.Article.BlogDomain, &r.Article.Title, &r.Article.URL,
			&r.Article.Summary, &r.Article.PublishedAt, &r.Article.ScrapedAt,
			&r.ArticleAnalysis.ID, &r.ArticleAnalysis.ArticleID,
			&r.ArticleAnalysis.Relevance, &r.ArticleAnalysis.Quality, &r.ArticleAnalysis.Timeliness,
			&r.ArticleAnalysis.TotalScore, &r.ArticleAnalysis.Category, &r.ArticleAnalysis.Keywords,
			&r.ArticleAnalysis.AISummary, &r.ArticleAnalysis.TitleCN, &r.ArticleAnalysis.RecommendReason,
			&r.ArticleAnalysis.AnalyzedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// MarkNotified sets notified_at to current timestamp for the given article IDs.
func (s *Store) MarkNotified(articleIDs []int64) error {
	if len(articleIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(articleIDs))
	args := make([]interface{}, len(articleIDs))
	for i, id := range articleIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		"UPDATE article_analysis SET notified_at = CURRENT_TIMESTAMP WHERE article_id IN (%s)",
		strings.Join(placeholders, ","),
	)
	_, err := s.db.Exec(query, args...)
	return err
}

// windowCutoff returns the UTC cutoff time formatted for SQLite comparison.
func windowCutoff(window string) (string, error) {
	var d time.Duration
	switch window {
	case "24h":
		d = 24 * time.Hour
	case "3days":
		d = 72 * time.Hour
	case "7days":
		d = 168 * time.Hour
	default:
		return "", fmt.Errorf("unsupported time window: %s (use 24h, 3days, or 7days)", window)
	}
	return time.Now().UTC().Add(-d).Format(time.RFC3339), nil
}
