package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chyiyaqing/newsbot/internal/notify/email"
	"github.com/chyiyaqing/newsbot/internal/store"
)

// JSON response types for the REST API.

type apiArticle struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	TitleCN         string `json:"title_cn,omitempty"`
	URL             string `json:"url"`
	Source          string `json:"source"`
	Summary         string `json:"summary,omitempty"`
	AISummary       string `json:"ai_summary,omitempty"`
	RecommendReason string `json:"recommend_reason,omitempty"`
	Category        string `json:"category,omitempty"`
	Keywords        string `json:"keywords,omitempty"`
	TotalScore      int    `json:"total_score"`
	Relevance       int    `json:"relevance"`
	Quality         int    `json:"quality"`
	Timeliness      int    `json:"timeliness"`
	PublishedAt     string `json:"published_at"`
	AnalyzedAt      string `json:"analyzed_at,omitempty"`
}

type apiListResponse struct {
	Window   string       `json:"window"`
	Count    int          `json:"count"`
	Articles []apiArticle `json:"articles"`
}

type apiDetailResponse struct {
	Article apiArticle `json:"article"`
}

type apiError struct {
	Error string `json:"error"`
}

// GET /api/articles?window=24h|3days|7days&limit=20
func (s *Server) handleAPIArticles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "method not allowed"})
		return
	}

	window := r.URL.Query().Get("window")
	switch window {
	case "24h", "3days", "7days":
	default:
		window = "24h"
	}

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	articles, err := s.db.TopScoredArticles(limit, window)
	if err != nil {
		log.Printf("ERROR: api list articles: %v", err)
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "failed to load articles"})
		return
	}

	items := make([]apiArticle, len(articles))
	for i, a := range articles {
		items[i] = toAPIArticle(a)
	}

	writeJSON(w, http.StatusOK, apiListResponse{
		Window:   window,
		Count:    len(items),
		Articles: items,
	})
}

// GET /api/articles/{id}
func (s *Server) handleAPIArticleDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "method not allowed"})
		return
	}

	// Extract ID from path: /api/articles/123
	idStr := strings.TrimPrefix(r.URL.Path, "/api/articles/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid article id"})
		return
	}

	article, err := s.db.GetArticleWithAnalysis(id)
	if err != nil {
		log.Printf("ERROR: api get article %d: %v", id, err)
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "failed to load article"})
		return
	}
	if article == nil {
		writeJSON(w, http.StatusNotFound, apiError{Error: "article not found"})
		return
	}

	writeJSON(w, http.StatusOK, apiDetailResponse{Article: toAPIArticle(*article)})
}

func toAPIArticle(a store.ArticleWithAnalysis) apiArticle {
	return apiArticle{
		ID:              a.Article.ID,
		Title:           a.Article.Title,
		TitleCN:         a.ArticleAnalysis.TitleCN,
		URL:             a.Article.URL,
		Source:          a.Article.BlogDomain,
		Summary:         a.Article.Summary,
		AISummary:       a.ArticleAnalysis.AISummary,
		RecommendReason: a.ArticleAnalysis.RecommendReason,
		Category:        a.ArticleAnalysis.Category,
		Keywords:        a.ArticleAnalysis.Keywords,
		TotalScore:      a.ArticleAnalysis.TotalScore,
		Relevance:       a.ArticleAnalysis.Relevance,
		Quality:         a.ArticleAnalysis.Quality,
		Timeliness:      a.ArticleAnalysis.Timeliness,
		PublishedAt:     fmtTimeRFC3339(a.Article.PublishedAt),
		AnalyzedAt:      fmtTimeRFC3339(a.ArticleAnalysis.AnalyzedAt),
	}
}

// POST /api/subscribe — {"email":"user@example.com"}
func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "method not allowed"})
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "email is required"})
		return
	}
	if !strings.Contains(body.Email, "@") {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid email"})
		return
	}

	token, err := email.GenerateToken()
	if err != nil {
		log.Printf("ERROR: generate subscribe token: %v", err)
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "internal error"})
		return
	}

	inserted, err := s.db.AddSubscriber(body.Email, token)
	if err != nil {
		log.Printf("ERROR: add subscriber: %v", err)
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "internal error"})
		return
	}

	if inserted {
		log.Printf("New subscriber: %s", body.Email)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "subscribed"})
}

// GET /api/unsubscribe?token=xxx
func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	removed, err := s.db.RemoveSubscriberByToken(token)
	if err != nil {
		log.Printf("ERROR: remove subscriber: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if removed {
		w.Write([]byte(`<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:60px"><h2>已成功取消订阅</h2><p style="color:#64748b">您将不再收到 NewsBot 的邮件通知。</p></body></html>`)) //nolint:errcheck
	} else {
		w.Write([]byte(`<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:60px"><h2>链接已失效</h2><p style="color:#64748b">该取消订阅链接无效或已使用过。</p></body></html>`)) //nolint:errcheck
	}
}

func fmtTimeRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
