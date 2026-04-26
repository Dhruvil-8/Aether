package api

import (
	"context"
	"embed"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aether/internal/node"
)

// Server is the HTTP API server that wraps the Aether node for end-user access.
type Server struct {
	cfg      node.Config
	store    *node.Store
	peers    *node.PeerBook
	hub      *StreamHub
	mux      *http.ServeMux
	srv      *http.Server
	staticFS fs.FS
}

// NewServer creates an API server. Pass webAssets to serve the embedded web UI.
func NewServer(cfg node.Config, store *node.Store, peers *node.PeerBook, webAssets *embed.FS) *Server {
	s := &Server{
		cfg:   cfg,
		store: store,
		peers: peers,
		hub:   NewStreamHub(store, 2*time.Second),
		mux:   http.NewServeMux(),
	}

	if webAssets != nil {
		sub, err := fs.Sub(webAssets, "web")
		if err == nil {
			s.staticFS = sub
		}
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/timeline", s.corsWrap(s.handleTimeline))
	s.mux.HandleFunc("/api/timeline/stream", s.corsWrap(s.handleTimelineStream))
	s.mux.HandleFunc("/api/post", s.corsWrap(s.handlePost))
	s.mux.HandleFunc("/api/status", s.corsWrap(s.handleStatus))
	s.mux.HandleFunc("/api/peers", s.corsWrap(s.handlePeers))

	if s.staticFS != nil {
		s.mux.Handle("/", http.FileServer(http.FS(s.staticFS)))
	}
}

func (s *Server) corsWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			w.Header().Add("Vary", "Origin")
			if !s.originAllowed(r, origin) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func (s *Server) originAllowed(r *http.Request, origin string) bool {
	if sameOrigin(r, origin) {
		return true
	}
	for _, allowed := range s.cfg.UIAllowedOrigins {
		if strings.EqualFold(strings.TrimSpace(allowed), origin) {
			return true
		}
	}
	return false
}

func sameOrigin(r *http.Request, origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return parsed.Scheme == scheme && strings.EqualFold(parsed.Host, r.Host)
}

// ListenAndServe starts the API server and blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	s.srv = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	go s.hub.Run(ctx)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
	}()

	return s.srv.Serve(ln)
}
