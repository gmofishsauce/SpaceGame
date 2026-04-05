// Package server implements the HTTP server for SpaceGame.
package server

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"time"

	webui "github.com/gmofishsauce/SpaceGame/web"
	"github.com/gmofishsauce/SpaceGame/srv/internal/game"
)

// Server is the HTTP server for SpaceGame.
type Server struct {
	engine  *game.Engine
	events  *game.EventManager
	state   *game.GameState
	httpSrv *http.Server
}

// New creates a Server wired to the given game components.
func New(engine *game.Engine, events *game.EventManager, state *game.GameState) *Server {
	s := &Server{engine: engine, events: events, state: state}
	s.httpSrv = &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: s.buildMux(),
	}
	return s
}

// ListenAndServe starts the server on 127.0.0.1:8080 and blocks until ctx is
// cancelled or a fatal error occurs. On ctx cancellation the server shuts down
// gracefully with a 5-second timeout. (FR-001, NFR-002)
func (s *Server) ListenAndServe(ctx context.Context) error {
	// Build embedded SPA file server.
	subFS, err := fs.Sub(webui.DistFS, "dist")
	if err != nil {
		return err
	}
	// The file server is registered as the final catch-all in buildMux, but we
	// need the sub-FS to be known at route time. We set it here after the mux
	// is already built by embedding it in the default-catch handler. The mux
	// was built with a placeholder; swap it out.
	s.httpSrv.Handler = s.buildMuxWithFS(subFS)

	errCh := make(chan error, 1)
	go func() {
		log.Printf("server: listening on http://127.0.0.1:8080")
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpSrv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// buildMux constructs the route mux without an SPA file server (used by New).
func (s *Server) buildMux() http.Handler {
	return s.buildMuxWithFS(nil)
}

// buildMuxWithFS builds the full route mux with an optional embedded SPA file server.
func (s *Server) buildMuxWithFS(spaFS fs.FS) http.Handler {
	mux := http.NewServeMux()

	// API routes (registered before the catch-all so they take priority).
	mux.HandleFunc("/api/stars", s.recoverMiddleware(s.handleStars))
	mux.HandleFunc("/api/state", s.recoverMiddleware(s.handleState))
	mux.HandleFunc("/api/events", s.recoverMiddleware(s.handleEvents))
	mux.HandleFunc("/api/command", s.recoverMiddleware(s.handleCommand))
	mux.HandleFunc("/api/pause", s.recoverMiddleware(s.handlePause))
	mux.HandleFunc("/api/debug/state", s.recoverMiddleware(s.handleDebugState))

	// SPA catch-all: serve from embedded FS. If spaFS is nil serve a bare 404.
	if spaFS != nil {
		mux.Handle("/", http.FileServerFS(spaFS))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "SPA not built yet — run 'cd web && npm run build'", http.StatusNotFound)
		})
	}

	return mux
}

// recoverMiddleware wraps h, recovering any panics and returning 500.
func (s *Server) recoverMiddleware(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("server: handler panic: %v", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		h(w, r)
	}
}
