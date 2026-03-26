package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chyiyaqing/newsbot/internal/store"
)

type Server struct {
	db      *store.Store
	emailCl EmailClient
	srv     *http.Server
	cache   *apiCache
}

// apiCache is a simple in-memory cache with TTL for API responses.
type apiCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	data      []byte
	expiresAt time.Time
}

func newAPICache() *apiCache {
	return &apiCache{entries: make(map[string]cacheEntry)}
}

func (c *apiCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return e.data, true
}

func (c *apiCache) set(key string, data []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{data: data, expiresAt: time.Now().Add(ttl)}
}

// Invalidate removes all cache entries (called after pipeline runs).
func (c *apiCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}

// EmailClient is a minimal interface for sending HTML emails.
type EmailClient interface {
	SendHTML(to, subject, body string) error
	SendWelcome(to, token string) error
}

func New(db *store.Store, addr string, emailCl EmailClient) *Server {
	s := &Server{db: db, emailCl: emailCl, cache: newAPICache()}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/articles", s.handleAPIArticles)
	mux.HandleFunc("/api/articles/", s.handleAPIArticleDetail)
	mux.HandleFunc("/api/categories", s.handleAPICategories)
	mux.HandleFunc("/api/stats", s.handleAPIStats)
	mux.HandleFunc("/api/subscribe", s.handleSubscribe)
	mux.HandleFunc("/api/unsubscribe", s.handleUnsubscribe)

	s.srv = &http.Server{
		Addr:    addr,
		Handler: gzipMiddleware(corsMiddleware(mux)),
	}
	return s
}

// Cache returns the server's API cache so callers can invalidate it after pipeline runs.
func (s *Server) Cache() *apiCache {
	return s.cache
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
	s.srv.Shutdown(ctx) //nolint:errcheck
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer io.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.Writer.Write(b)
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		defer gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, Writer: gz}, r)
	})
}
