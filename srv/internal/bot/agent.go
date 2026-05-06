package bot

import (
	"context"
	"log"
	"math/rand"
	"time"
)

const (
	BotDecisionInterval  = 2 * time.Second
	ReporterSweepCadence = 5  // every Nth decide tick
	EscortCost           = 8  // must match server WeaponDefs[escort].Cost
	ReporterCost         = 4  // must match server WeaponDefs[reporter].Cost
	MinWealthReserve     = 16 // keep this much wealth back for non-escort builds
)

// Agent implements the v1 alien strategy. (FR-70, FR-71, FR-72)
type Agent struct {
	client     *Client
	local      *Local
	debugTruth bool
	logger     *log.Logger
	rng        *rand.Rand
	homeID     string
	tickCount  int
}

func NewAgent(c *Client, l *Local, debugTruth bool, logger *log.Logger) *Agent {
	return &Agent{
		client:     c,
		local:      l,
		debugTruth: debugTruth,
		logger:     logger,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run blocks, deciding on a cadence, until ctx is cancelled. (FR-70)
func (a *Agent) Run(ctx context.Context) error {
	// Resolve home ID from the star catalogue.
	snap := a.local.Snapshot()
	for id, s := range snap.Stars {
		if s.DisplayName == "61 Ursae Majoris" {
			a.homeID = id
			break
		}
	}
	if a.homeID == "" {
		a.logger.Printf("bot: agent: alien home star not found in catalogue; waiting")
	}

	ticker := time.NewTicker(BotDecisionInterval)
	defer ticker.Stop()

	var debugTicker *time.Ticker
	if a.debugTruth {
		debugTicker = time.NewTicker(10 * time.Second)
		defer debugTicker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if a.homeID == "" {
				// Retry home lookup — Local may have received the connected frame by now.
				snap2 := a.local.Snapshot()
				for id, s := range snap2.Stars {
					if s.DisplayName == "61 Ursae Majoris" {
						a.homeID = id
						a.logger.Printf("bot: agent: resolved home = %s", id)
						break
					}
				}
			}
			a.decide(ctx)
		case <-func() <-chan time.Time {
			if debugTicker != nil {
				return debugTicker.C
			}
			return nil
		}():
			ds, err := a.client.GetDebugState(ctx)
			if err != nil {
				a.logger.Printf("bot: debug/state: %v", err)
			} else {
				a.logger.Printf("bot: debug year=%.2f events=%d", ds.GameYear, len(ds.Events))
			}
		}
	}
}

// decide runs one strategy cycle. (FR-70)
func (a *Agent) decide(ctx context.Context) {
	defer func() { a.tickCount++ }()

	snap := a.local.Snapshot()
	if snap.Paused || snap.GameOver {
		return
	}
	if a.homeID == "" {
		return
	}

	home, ok := snap.Systems[a.homeID]
	if !ok {
		return
	}

	// FR-70.1 — replenish escorts at home if we have wealth to spare.
	if home.KnownWealth >= EscortCost+MinWealthReserve {
		resp, err := a.client.PostCommand(ctx, CommandRequest{
			Type:       "construct",
			SystemID:   a.homeID,
			WeaponType: "escort",
			Quantity:   1,
		})
		if err != nil {
			a.logger.Printf("bot: construct escort: %v", err)
		} else if !resp.OK {
			a.logger.Printf("bot: construct escort rejected: %s", resp.Error)
		}
	}

	// FR-70.2 — dispatch a fleet at the most-recently-known human system.
	target := mostRecentHumanSystem(snap)
	if target != "" {
		fleet := pickReadyFleet(snap, a.homeID, a.client.Player)
		if fleet != "" {
			resp, err := a.client.PostCommand(ctx, CommandRequest{
				Type:     "move",
				SystemID: a.homeID,
				FleetID:  fleet,
				DestID:   target,
			})
			if err != nil {
				a.logger.Printf("bot: move fleet: %v", err)
			} else if !resp.OK {
				a.logger.Printf("bot: move fleet rejected: %s", resp.Error)
			}
		}
	}

	// FR-70.3 — slow reporter sweep: build a reporter every Nth tick and send it to
	// the oldest-unobserved non-home system.
	if a.tickCount%ReporterSweepCadence == 0 {
		sweepTarget := oldestUnobservedSystem(snap, a.homeID)
		if sweepTarget != "" && home.KnownWealth >= ReporterCost {
			resp, err := a.client.PostCommand(ctx, CommandRequest{
				Type:       "construct",
				SystemID:   a.homeID,
				WeaponType: "reporter",
				Quantity:   1,
			})
			if err != nil {
				a.logger.Printf("bot: construct reporter: %v", err)
			} else if !resp.OK {
				a.logger.Printf("bot: construct reporter rejected: %s", resp.Error)
			} else {
				// Move next tick via pickReadyFleet; for now just log intent.
				a.logger.Printf("bot: reporter sweep → %s", sweepTarget)
			}
		}
	}
}

// mostRecentHumanSystem returns the ID of the human-status system with the
// highest KnownAsOfYear, or "" if none is known.
func mostRecentHumanSystem(snap LocalSnapshot) string {
	best := ""
	var bestYear float64 = -1
	for id, sys := range snap.Systems {
		if sys.KnownStatus == "human" && sys.KnownAsOfYear > bestYear {
			best = id
			bestYear = sys.KnownAsOfYear
		}
	}
	return best
}

// pickReadyFleet returns the ID of a stationed fleet owned by player at sysID
// that contains at least one escort or battleship, or "" if none.
func pickReadyFleet(snap LocalSnapshot, sysID, player string) string {
	sys, ok := snap.Systems[sysID]
	if !ok {
		return ""
	}
	for _, f := range sys.KnownFleets {
		if f.Owner != player || f.InTransit {
			continue
		}
		if f.Units["escort"] > 0 || f.Units["battleship"] > 0 {
			return f.ID
		}
	}
	return ""
}

// oldestUnobservedSystem returns the ID of the non-home system with the
// lowest KnownAsOfYear (i.e. we have the stalest information), or "".
func oldestUnobservedSystem(snap LocalSnapshot, homeID string) string {
	best := ""
	var bestYear float64 = 1e18
	for id, sys := range snap.Systems {
		if id == homeID {
			continue
		}
		if sys.KnownAsOfYear < bestYear {
			best = id
			bestYear = sys.KnownAsOfYear
		}
	}
	return best
}
