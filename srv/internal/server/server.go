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

// playerKey is the context key for the resolved player Owner.
type playerKey struct{}

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
	subFS, err := fs.Sub(webui.DistFS, "dist")
	if err != nil {
		return err
	}
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

	// Routes that do NOT require ?player identification.
	mux.HandleFunc("/api/stars", s.recoverMiddleware(s.handleStars))
	mux.HandleFunc("/api/debug/state", s.recoverMiddleware(s.handleDebugState))

	// Routes that require ?player=human|alien. (FR-36, FR-37, FR-38)
	mux.HandleFunc("/api/state", s.recoverMiddleware(s.playerMiddleware(s.handleState)))
	mux.HandleFunc("/api/events", s.recoverMiddleware(s.playerMiddleware(s.handleEvents)))
	mux.HandleFunc("/api/command", s.recoverMiddleware(s.playerMiddleware(s.handleCommand)))
	mux.HandleFunc("/api/pause", s.recoverMiddleware(s.playerMiddleware(s.handlePause)))

	if spaFS != nil {
		mux.Handle("/", http.FileServerFS(spaFS))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "SPA not built yet — run 'cd web && npm run build'", http.StatusNotFound)
		})
	}

	return mux
}

// playerMiddleware reads ?player, validates it is "human" or "alien", and
// stores the resolved Owner in the request context. Rejects missing or
// invalid values with HTTP 400. (FR-36, FR-37, FR-38)
func (s *Server) playerMiddleware(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("player")
		var player game.Owner
		switch raw {
		case "human":
			player = game.HumanOwner
		case "alien":
			player = game.AlienOwner
		default:
			writeError(w, http.StatusBadRequest, "unknown player")
			return
		}
		ctx := context.WithValue(r.Context(), playerKey{}, player)
		h(w, r.WithContext(ctx))
	}
}

// playerOf extracts the resolved player Owner from the request context.
// Panics if playerMiddleware was not applied (programming error).
func playerOf(r *http.Request) game.Owner {
	return r.Context().Value(playerKey{}).(game.Owner)
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
