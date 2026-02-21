package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/chyiyaqing/newsbot/internal/store"
)

var tmpl = template.Must(template.New("articles").Funcs(template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.Format("2006-01-02 15:04")
	},
}).Parse(articlesHTML))

const articlesHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>NewsBot - Articles</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #f5f5f5; color: #333; }
  .container { max-width: 960px; margin: 0 auto; padding: 20px; }
  h1 { margin-bottom: 16px; font-size: 24px; }
  .tabs { display: flex; gap: 8px; margin-bottom: 24px; }
  .tabs a {
    padding: 8px 20px; border-radius: 6px; text-decoration: none;
    background: #e0e0e0; color: #555; font-weight: 500; font-size: 14px;
  }
  .tabs a.active { background: #1a73e8; color: #fff; }
  .tabs a:hover:not(.active) { background: #d0d0d0; }
  .card {
    background: #fff; border-radius: 8px; padding: 16px 20px; margin-bottom: 12px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.08);
  }
  .card-header { display: flex; align-items: baseline; gap: 12px; margin-bottom: 8px; }
  .rank { font-size: 18px; font-weight: 700; color: #1a73e8; min-width: 28px; }
  .score { font-size: 13px; background: #e8f0fe; color: #1a73e8; padding: 2px 8px; border-radius: 4px; }
  .category { font-size: 13px; background: #fce8e6; color: #c5221f; padding: 2px 8px; border-radius: 4px; }
  .title a { font-size: 16px; font-weight: 600; color: #1a1a1a; text-decoration: none; }
  .title a:hover { color: #1a73e8; text-decoration: underline; }
  .title-cn { font-size: 14px; color: #666; margin-top: 4px; }
  .reason { font-size: 13px; color: #888; margin-top: 6px; line-height: 1.5; }
  .meta { font-size: 12px; color: #aaa; margin-top: 8px; }
  .empty { text-align: center; padding: 60px 20px; color: #999; }
</style>
</head>
<body>
<div class="container">
  <h1>NewsBot</h1>
  <div class="tabs">
    <a href="/articles?window=24h" {{if eq .Window "24h"}}class="active"{{end}}>24h</a>
    <a href="/articles?window=3days" {{if eq .Window "3days"}}class="active"{{end}}>3 Days</a>
    <a href="/articles?window=7days" {{if eq .Window "7days"}}class="active"{{end}}>7 Days</a>
  </div>
  {{if .Articles}}
  {{range $i, $a := .Articles}}
  <div class="card">
    <div class="card-header">
      <span class="rank">#{{add $i 1}}</span>
      <span class="score">{{$a.ArticleAnalysis.TotalScore}}</span>
      {{if $a.ArticleAnalysis.Category}}<span class="category">{{$a.ArticleAnalysis.Category}}</span>{{end}}
    </div>
    <div class="title"><a href="{{$a.Article.URL}}" target="_blank" rel="noopener">{{$a.Article.Title}}</a></div>
    {{if $a.ArticleAnalysis.TitleCN}}<div class="title-cn">{{$a.ArticleAnalysis.TitleCN}}</div>{{end}}
    {{if $a.ArticleAnalysis.RecommendReason}}<div class="reason">{{$a.ArticleAnalysis.RecommendReason}}</div>{{end}}
    <div class="meta">{{$a.Article.BlogDomain}} &middot; {{fmtTime $a.Article.PublishedAt}}</div>
  </div>
  {{end}}
  {{else}}
  <div class="empty">No articles found in this time window.</div>
  {{end}}
</div>
</body>
</html>`

type pageData struct {
	Window   string
	Articles []store.ArticleWithAnalysis
}

type Server struct {
	db   *store.Store
	srv  *http.Server
}

func New(db *store.Store, addr string) *Server {
	s := &Server{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/articles", s.handleArticles)

	// REST API
	mux.HandleFunc("/api/articles", s.handleAPIArticles)
	mux.HandleFunc("/api/articles/", s.handleAPIArticleDetail)

	s.srv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return s
}

// Start begins listening. It blocks until the server is shut down.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.srv.Addr, err)
	}
	log.Printf("HTTP server listening on %s", ln.Addr())

	go func() {
		<-ctx.Done()
		s.Shutdown()
	}()

	if err := s.srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.srv.Shutdown(ctx)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/articles?window=24h", http.StatusFound)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleArticles(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	switch window {
	case "24h", "3days", "7days":
	default:
		window = "24h"
	}

	articles, err := s.db.TopScoredArticles(20, window)
	if err != nil {
		http.Error(w, "Failed to load articles", http.StatusInternalServerError)
		log.Printf("ERROR: load articles: %v", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, pageData{Window: window, Articles: articles}); err != nil {
		log.Printf("ERROR: render template: %v", err)
	}
}
