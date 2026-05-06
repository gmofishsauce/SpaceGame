package bot

import (
	"encoding/json"
	"testing"
)

func TestLocal_ApplyConnected(t *testing.T) {
	l := NewLocal(nil)
	snap := StateSnapshot{
		GameYear: 5.0,
		Paused:   true,
		Systems: []SystemView{
			{ID: "sol", DisplayName: "Sol", KnownStatus: "human", KnownEconLevel: 5},
			{ID: "alpha-centauri", DisplayName: "Alpha Centauri", KnownStatus: "human"},
		},
		FleetsInTransit: []FleetView{
			{ID: "f-1", Name: "Fleet 1", Owner: "human", InTransit: true},
		},
	}
	data, _ := json.Marshal(snap)
	l.mu.Lock()
	l.applyConnected(data)
	l.mu.Unlock()

	got := l.Snapshot()
	if got.Year != 5.0 {
		t.Errorf("Year=%.1f, want 5.0", got.Year)
	}
	if !got.Paused {
		t.Error("Paused should be true")
	}
	if _, ok := got.Systems["sol"]; !ok {
		t.Error("sol missing from systems after applyConnected")
	}
	if _, ok := got.Systems["alpha-centauri"]; !ok {
		t.Error("alpha-centauri missing from systems")
	}
	if _, ok := got.Transit["f-1"]; !ok {
		t.Error("in-transit fleet f-1 missing from transit map")
	}
}

func TestLocal_ApplySystemUpdate(t *testing.T) {
	l := NewLocal(nil)
	// Seed an initial system.
	l.mu.Lock()
	l.systems["sol"] = SystemView{ID: "sol", KnownStatus: "human", KnownWealth: 100}
	l.mu.Unlock()

	// Apply an update that changes status and wealth.
	updated := SystemView{ID: "sol", KnownStatus: "alien", KnownWealth: 50}
	data, _ := json.Marshal(updated)
	l.mu.Lock()
	l.applySystemUpdate(data)
	l.mu.Unlock()

	got := l.Snapshot()
	sys, ok := got.Systems["sol"]
	if !ok {
		t.Fatal("sol missing after system_update")
	}
	if sys.KnownStatus != "alien" {
		t.Errorf("KnownStatus=%q, want alien", sys.KnownStatus)
	}
	if sys.KnownWealth != 50 {
		t.Errorf("KnownWealth=%.1f, want 50", sys.KnownWealth)
	}
}

func TestLocal_ApplyGameOver(t *testing.T) {
	l := NewLocal(nil)
	data := []byte(`{"winner":"human","winReason":"captured home"}`)
	l.mu.Lock()
	l.applyGameOver(data)
	l.mu.Unlock()

	got := l.Snapshot()
	if !got.GameOver {
		t.Error("GameOver should be true after game_over frame")
	}
	if got.Winner != "human" {
		t.Errorf("Winner=%q, want human", got.Winner)
	}
}

func TestLocal_ApplyClockSync(t *testing.T) {
	l := NewLocal(nil)
	data := []byte(`{"gameYear":42.5,"paused":false}`)
	l.mu.Lock()
	l.applyClockSync(data)
	l.mu.Unlock()

	got := l.Snapshot()
	if got.Year != 42.5 {
		t.Errorf("Year=%.1f, want 42.5", got.Year)
	}
	if got.Paused {
		t.Error("Paused should be false after clock_sync{paused:false}")
	}
}
