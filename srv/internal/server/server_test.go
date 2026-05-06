package server

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gmofishsauce/SpaceGame/srv/internal/game"
)

// newTestServer builds a Server backed by a minimal game state suitable for
// handler unit tests.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	cat := game.NewStarCatalog([]*game.CatalogEntry{
		{ID: "sol", DisplayName: "Sol", X: 0, Y: 0, Z: 0, DistFromSol: 0, IsSol: true},
	})
	truth := &game.Truth{
		Systems: map[string]*game.TrueSystem{
			"sol": {ID: "sol", Status: game.StatusHuman, EconLevel: 5, Wealth: 64, LocalUnits: map[game.WeaponType]int{}},
		},
		Fleets: map[string]*game.TrueFleet{},
	}
	humanView := &game.PlayerView{
		Systems:   map[string]*game.KnownSystem{"sol": {ID: "sol", Status: game.StatusHuman}},
		Fleets:    map[string]*game.KnownFleet{},
		InTransit: map[string]*game.KnownTransit{},
	}
	alienView := &game.PlayerView{
		Systems:   map[string]*game.KnownSystem{"sol": {ID: "sol", Status: game.StatusHuman}},
		Fleets:    map[string]*game.KnownFleet{},
		InTransit: map[string]*game.KnownTransit{},
	}
	state := game.NewGameStateForTest(cat, truth, humanView, alienView, rand.New(rand.NewSource(1)))
	events := game.NewEventManager()
	engine := game.NewEngine(state, events, rand.New(rand.NewSource(1)))
	return New(engine, events, state)
}

// TestPlayerMiddleware_MissingPlayer verifies that a missing ?player query
// param returns HTTP 400. (FR-37)
func TestPlayerMiddleware_MissingPlayer(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	rec := httptest.NewRecorder()
	srv.buildMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing ?player: got %d, want 400", rec.Code)
	}
}

// TestPlayerMiddleware_InvalidPlayer verifies that an unrecognised ?player
// value returns HTTP 400. (FR-38)
func TestPlayerMiddleware_InvalidPlayer(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/state?player=spectator", nil)
	rec := httptest.NewRecorder()
	srv.buildMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid ?player: got %d, want 400", rec.Code)
	}
}

// TestPlayerMiddleware_ValidPlayers verifies that both recognised player
// values succeed on a route that requires ?player. (FR-36)
func TestPlayerMiddleware_ValidPlayers(t *testing.T) {
	srv := newTestServer(t)
	for _, p := range []string{"human", "alien"} {
		req := httptest.NewRequest(http.MethodGet, "/api/state?player="+p, nil)
		rec := httptest.NewRecorder()
		srv.buildMux().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("?player=%s: got %d, want 200", p, rec.Code)
		}
	}
}

// TestStarsNoPlayerRequired verifies that GET /api/stars succeeds without a
// ?player parameter. (FR-39)
func TestStarsNoPlayerRequired(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/stars", nil)
	rec := httptest.NewRecorder()
	srv.buildMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/api/stars without ?player: got %d, want 200", rec.Code)
	}
}

// TestPause_AlienForbidden verifies that POST /api/pause?player=alien returns
// HTTP 403. (FR-50)
func TestPause_AlienForbidden(t *testing.T) {
	srv := newTestServer(t)
	body := strings.NewReader(`{"paused":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/pause?player=alien", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.buildMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("alien pause: got %d, want 403", rec.Code)
	}
	var resp CommandResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.OK {
		t.Error("expected ok=false for alien pause")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

// TestPause_HumanAllowed verifies that POST /api/pause?player=human is
// accepted. (FR-50)
func TestPause_HumanAllowed(t *testing.T) {
	srv := newTestServer(t)
	body := strings.NewReader(`{"paused":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/pause?player=human", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.buildMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("human pause: got %d, want 200", rec.Code)
	}
}
