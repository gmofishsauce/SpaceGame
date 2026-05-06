package bot

import (
	"context"
	"encoding/json"
	"log"
	"sync"
)

// Local maintains the bot's mirror of its PlayerView by applying SSE frames.
// (FR-65, FR-72)
type Local struct {
	mu      sync.Mutex
	client  *Client
	year    float64
	paused  bool
	gameOver bool
	winner  string
	stars   map[string]Star
	systems map[string]SystemView
	transit map[string]FleetView // in-transit fleets keyed by fleet ID
}

// LocalSnapshot is a safe copy of Local's current state.
type LocalSnapshot struct {
	Year     float64
	Paused   bool
	GameOver bool
	Winner   string
	Stars    map[string]Star
	Systems  map[string]SystemView
	Transit  map[string]FleetView
}

func NewLocal(client *Client) *Local {
	return &Local{
		client:  client,
		stars:   map[string]Star{},
		systems: map[string]SystemView{},
		transit: map[string]FleetView{},
	}
}

// Run fetches stars then streams events, applying each frame to Local's state.
// Returns when ctx is cancelled or the stream drops. (FR-65)
func (l *Local) Run(ctx context.Context) error {
	stars, err := l.client.GetStars(ctx)
	if err != nil {
		return err
	}
	l.mu.Lock()
	for _, s := range stars {
		l.stars[s.ID] = s
	}
	l.mu.Unlock()

	err = l.client.StreamEvents(ctx, func(frame EventFrame) {
		l.mu.Lock()
		defer l.mu.Unlock()
		switch frame.Name {
		case "connected":
			l.applyConnected(frame.Data)
		case "clock_sync":
			l.applyClockSync(frame.Data)
		case "game_event":
			// decorative; state arrives via system_update
			log.Printf("bot: game_event: %s", frame.Data)
		case "system_update":
			l.applySystemUpdate(frame.Data)
		case "game_over":
			l.applyGameOver(frame.Data)
		}
	})
	return err
}

// Snapshot returns a safe copy under the mutex.
func (l *Local) Snapshot() LocalSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	stars := make(map[string]Star, len(l.stars))
	for k, v := range l.stars {
		stars[k] = v
	}
	systems := make(map[string]SystemView, len(l.systems))
	for k, v := range l.systems {
		systems[k] = v
	}
	transit := make(map[string]FleetView, len(l.transit))
	for k, v := range l.transit {
		transit[k] = v
	}
	return LocalSnapshot{
		Year:     l.year,
		Paused:   l.paused,
		GameOver: l.gameOver,
		Winner:   l.winner,
		Stars:    stars,
		Systems:  systems,
		Transit:  transit,
	}
}

func (l *Local) applyConnected(data []byte) {
	var snap StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		log.Printf("bot: decode connected: %v", err)
		return
	}
	l.year = snap.GameYear
	l.paused = snap.Paused
	l.gameOver = snap.GameOver
	l.winner = snap.Winner
	l.systems = make(map[string]SystemView, len(snap.Systems))
	for _, s := range snap.Systems {
		l.systems[s.ID] = s
	}
	l.transit = make(map[string]FleetView, len(snap.FleetsInTransit))
	for _, f := range snap.FleetsInTransit {
		l.transit[f.ID] = f
	}
}

func (l *Local) applyClockSync(data []byte) {
	var cs struct {
		GameYear float64 `json:"gameYear"`
		Paused   bool    `json:"paused"`
	}
	if err := json.Unmarshal(data, &cs); err != nil {
		log.Printf("bot: decode clock_sync: %v", err)
		return
	}
	l.year = cs.GameYear
	l.paused = cs.Paused
}

func (l *Local) applySystemUpdate(data []byte) {
	var sys SystemView
	if err := json.Unmarshal(data, &sys); err != nil {
		log.Printf("bot: decode system_update: %v", err)
		return
	}
	l.systems[sys.ID] = sys
}

func (l *Local) applyGameOver(data []byte) {
	var go_ struct {
		Winner    string `json:"winner"`
		WinReason string `json:"winReason"`
	}
	if err := json.Unmarshal(data, &go_); err != nil {
		log.Printf("bot: decode game_over: %v", err)
		return
	}
	l.gameOver = true
	l.winner = go_.Winner
}

