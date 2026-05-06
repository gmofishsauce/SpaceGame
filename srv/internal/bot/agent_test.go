package bot

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testLocalFromSnap builds a Local pre-loaded with the given snapshot.
func testLocalFromSnap(snap LocalSnapshot) *Local {
	l := &Local{
		year:     snap.Year,
		paused:   snap.Paused,
		gameOver: snap.GameOver,
		stars:    snap.Stars,
		systems:  snap.Systems,
		transit:  snap.Transit,
	}
	return l
}

// captureCommands starts a fake server that records all PostCommand bodies
// and returns a channel that emits them.
func captureCommands(t *testing.T) (*httptest.Server, <-chan CommandRequest) {
	t.Helper()
	ch := make(chan CommandRequest, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/command"):
			var cmd CommandRequest
			if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
				http.Error(w, "bad body", 400)
				return
			}
			ch <- cmd
			json.NewEncoder(w).Encode(CommandResponse{OK: true})
		case strings.HasPrefix(r.URL.Path, "/api/debug/state"):
			json.NewEncoder(w).Encode(DebugSnapshot{})
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, ch
}

// TestAgent_ReplenishEscorts verifies FR-70.1: when the home system has
// enough wealth and no combat-ready units, the agent constructs an escort.
func TestAgent_ReplenishEscorts(t *testing.T) {
	srv, cmds := captureCommands(t)
	defer srv.Close()

	homeID := "alien-home"
	snap := LocalSnapshot{
		Year:   10,
		Paused: false,
		Stars:  map[string]Star{homeID: {ID: homeID, DisplayName: "61 Ursae Majoris"}},
		Systems: map[string]SystemView{
			homeID: {
				ID:          homeID,
				KnownStatus: "alien",
				KnownWealth: EscortCost + MinWealthReserve + 1, // enough
				KnownFleets: nil,
			},
		},
		Transit: map[string]FleetView{},
	}

	c := NewClient(srv.URL, "alien")
	l := testLocalFromSnap(snap)
	a := &Agent{
		client:   c,
		local:    l,
		homeID:   homeID,
		logger:   noopLogger(),
		tickCount: 0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	a.decide(ctx)

	select {
	case cmd := <-cmds:
		if cmd.Type != "construct" || cmd.WeaponType != "escort" {
			t.Errorf("got command type=%q weapon=%q, want construct escort", cmd.Type, cmd.WeaponType)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for construct escort command")
	}
}

// TestAgent_DispatchFleet verifies FR-70.2: when a human system is known
// and a combat-ready fleet exists at home, the agent issues a move command.
func TestAgent_DispatchFleet(t *testing.T) {
	srv, cmds := captureCommands(t)
	defer srv.Close()

	homeID := "alien-home"
	fleetID := "f-1"
	snap := LocalSnapshot{
		Year:   10,
		Paused: false,
		Stars:  map[string]Star{homeID: {ID: homeID, DisplayName: "61 Ursae Majoris"}},
		Systems: map[string]SystemView{
			homeID: {
				ID:          homeID,
				KnownStatus: "alien",
				KnownWealth: 5, // not enough to build escort (below MinWealthReserve+EscortCost)
				KnownFleets: []FleetView{
					{ID: fleetID, Owner: "alien", Units: map[string]int{"escort": 2}},
				},
			},
			"human-sys": {
				ID:            "human-sys",
				KnownStatus:   "human",
				KnownAsOfYear: 5.0,
			},
		},
		Transit: map[string]FleetView{},
	}

	c := NewClient(srv.URL, "alien")
	l := testLocalFromSnap(snap)
	a := &Agent{
		client:   c,
		local:    l,
		homeID:   homeID,
		logger:   noopLogger(),
		tickCount: 0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	a.decide(ctx)

	select {
	case cmd := <-cmds:
		if cmd.Type != "move" {
			t.Errorf("got command type=%q, want move", cmd.Type)
		}
		if cmd.FleetID != fleetID {
			t.Errorf("FleetID=%q, want %q", cmd.FleetID, fleetID)
		}
		if cmd.DestID != "human-sys" {
			t.Errorf("DestID=%q, want human-sys", cmd.DestID)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for move command")
	}
}

// TestAgent_ReporterSweep verifies FR-70.3: on Nth tick the agent constructs
// a reporter when the oldest unobserved system exists and wealth permits.
func TestAgent_ReporterSweep(t *testing.T) {
	srv, cmds := captureCommands(t)
	defer srv.Close()

	homeID := "alien-home"
	snap := LocalSnapshot{
		Year:   10,
		Paused: false,
		Stars:  map[string]Star{homeID: {ID: homeID, DisplayName: "61 Ursae Majoris"}},
		Systems: map[string]SystemView{
			homeID: {
				ID:          homeID,
				KnownStatus: "alien",
				KnownWealth: ReporterCost + 1,
			},
			"dark-sys": {
				ID:            "dark-sys",
				KnownStatus:   "uninhabited",
				KnownAsOfYear: 0,
			},
		},
		Transit: map[string]FleetView{},
	}

	c := NewClient(srv.URL, "alien")
	l := testLocalFromSnap(snap)
	a := &Agent{
		client:    c,
		local:     l,
		homeID:    homeID,
		logger:    noopLogger(),
		tickCount: 0, // 0 % ReporterSweepCadence == 0 → sweep fires
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	a.decide(ctx)

	select {
	case cmd := <-cmds:
		if cmd.Type != "construct" || cmd.WeaponType != "reporter" {
			t.Errorf("got command type=%q weapon=%q, want construct reporter", cmd.Type, cmd.WeaponType)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for construct reporter command")
	}
}

// TestAgent_SkipsWhenPaused verifies that decide() is a no-op when the game
// is paused (FR-67).
func TestAgent_SkipsWhenPaused(t *testing.T) {
	srv, cmds := captureCommands(t)
	defer srv.Close()

	snap := LocalSnapshot{
		Paused: true, // game is paused
		Stars:  map[string]Star{"h": {ID: "h", DisplayName: "61 Ursae Majoris"}},
		Systems: map[string]SystemView{
			"h": {ID: "h", KnownStatus: "alien", KnownWealth: 10000},
		},
		Transit: map[string]FleetView{},
	}

	c := NewClient(srv.URL, "alien")
	l := testLocalFromSnap(snap)
	a := &Agent{client: c, local: l, homeID: "h", logger: noopLogger()}
	a.decide(context.Background())

	select {
	case cmd := <-cmds:
		t.Errorf("expected no command when paused, got %+v", cmd)
	default:
		// good — no command issued
	}
}

// noopLogger returns a *log.Logger that discards all output.
func noopLogger() *log.Logger { return log.New(io.Discard, "", 0) }
