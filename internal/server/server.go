package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/chyiyaqing/newsbot/internal/store"
)

type Server struct {
	db      *store.Store
	emailCl EmailClient
	srv     *http.Server
}

// EmailClient is a minimal interface for sending HTML emails.
type EmailClient interface {
	SendHTML(to, subject, body string) error
}

func New(db *store.Store, addr string, emailCl EmailClient) *Server {
	s := &Server{db: db, emailCl: emailCl}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/articles", s.handleAPIArticles)
	mux.HandleFunc("/api/articles/", s.handleAPIArticleDetail)
	mux.HandleFunc("/api/subscribe", s.handleSubscribe)
	mux.HandleFunc("/api/unsubscribe", s.handleUnsubscribe)

	s.srv = &http.Server{
		Addr:    addr,
		Handler: corsMiddleware(mux),
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
