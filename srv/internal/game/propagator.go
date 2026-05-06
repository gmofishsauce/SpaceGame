package game

import (
	"log"
)

// Propagator is the single bridge between EventLog and SolView. It is the
// only writer of SolView (DR-2). It pops every matured Event from the log,
// applies it to SolView via applyEventToView, and (for non-internal events)
// emits SSE frames to clients.
//
// Caller must hold the engine write lock for the entire Propagate call.
type Propagator struct {
	Events *EventManager // SSE broadcast plumbing
}

// NewPropagator returns a Propagator that publishes via the given EventManager.
func NewPropagator(em *EventManager) *Propagator {
	return &Propagator{Events: em}
}

// Propagate pops every matured Event from each player's heap,
// applies each to that player's view, and broadcasts non-internal events
// over that player's SSE channel. Caller must hold state.mu (write lock).
// (FR-14, FR-16, FR-18)
func (p *Propagator) Propagate(state *GameState) {
	for _, player := range []Owner{HumanOwner, AlienOwner} {
		view := state.Views[player]
		if view == nil {
			continue
		}
		for _, evt := range state.Events.PopMatured(state.Clock, player) {
			p.applyEventToView(view, state.Catalog, evt, player)
			evt.AppliedToView[player] = true
			if !evt.Internal {
				p.Events.broadcastEvent(player, evt)
				p.Events.broadcastSystemUpdate(player, state, evt.SystemID)
				evt.Broadcast[player] = true
			}
		}
	}
}

// applyEventToView mutates a player's view based on a single matured Event.
// player is the owner of view — used for perspective-sensitive cases such as
// combat losses and conquest status. This is the only function that writes to
// a PlayerView (the loader excepted).
//
// Each case is defensive: if a referenced KnownSystem is missing, a single
// log line warns and the event is skipped. The propagator never panics —
// it must keep the engine alive.
func (p *Propagator) applyEventToView(view *PlayerView, cat *StarCatalog, evt *Event, player Owner) {
	if view == nil || evt == nil {
		return
	}

	sys := view.System(evt.SystemID)
	if sys == nil {
		// Some events (e.g. EventAlienExhausted) target "sol" but otherwise
		// have no per-system effect. If the system is genuinely unknown,
		// warn and skip.
		log.Printf("propagator: event %q references unknown system %q", evt.ID, evt.SystemID)
		return
	}

	// Always advance the system's "as of" year to the event's origin year.
	if evt.EventYear > sys.AsOfYear {
		sys.AsOfYear = evt.EventYear
	}

	switch evt.Type {

	case EventCombatOccurred:
		d, ok := evt.Details.(*CombatDetails)
		if !ok {
			return
		}
		if d.HumanWon {
			sys.Status = StatusHuman
		} else if d.AlienWon {
			sys.Status = StatusAlien
		} else if d.Draw {
			sys.Status = StatusContested
		}
		// Apply losses for the viewing player's side. Events are routed to
		// the precombat owner, so player == precombat owner == defender.
		var myLosses map[WeaponType]int
		if player == HumanOwner {
			myLosses = d.HumanLosses
		} else {
			myLosses = d.AlienLosses
		}
		// Apply non-mobile (local-unit) losses.
		if sys.LocalUnits == nil {
			sys.LocalUnits = map[WeaponType]int{}
		}
		for wt, n := range myLosses {
			if WeaponDefs[wt].CanMove {
				continue
			}
			if sys.LocalUnits[wt] >= n {
				sys.LocalUnits[wt] -= n
			} else {
				sys.LocalUnits[wt] = 0
			}
		}
		// Apply mobile (fleet) losses across this system's known fleets.
		applyMobileLossesToFleets(view, sys, myLosses)

	case EventSystemCaptured:
		sys.Status = StatusAlien

	case EventSystemRetaken:
		sys.Status = StatusHuman

	case EventSystemConquered:
		sys.Status = statusOf(player) // player is the conqueror (conquest events route to fleet owner)
		sys.EconLevel = 0
		sys.Wealth = 0

	case EventConstructionDone:
		d, ok := evt.Details.(*ConstructionDetails)
		if !ok {
			return
		}
		def := WeaponDefs[d.WeaponType]
		// DR-6: wealth deduction is reportable.
		sys.Wealth -= def.Cost * float64(d.Quantity)
		if sys.Wealth < 0 {
			sys.Wealth = 0
		}
		if !def.CanMove {
			if sys.LocalUnits == nil {
				sys.LocalUnits = map[WeaponType]int{}
			}
			sys.LocalUnits[d.WeaponType] += d.Quantity
			break
		}
		// DR-5: mobile-unit construction is reportable. Apply to the
		// fleet whose ID was recorded by the truth-side ExecuteConstruct.
		applyMobileConstructionToFleet(view, sys, d, evt.EventYear)

	case EventFleetArrival:
		d, ok := evt.Details.(*FleetArrivalDetails)
		if !ok {
			return
		}
		// DR-4: snapshot. Replace any existing KnownFleet for this ID.
		view.Fleets[d.FleetID] = &KnownFleet{
			ID:         d.FleetID,
			Name:       d.FleetName,
			Owner:      d.Owner,
			Units:      copyUnits(d.Units),
			LocationID: evt.SystemID,
			AsOfYear:   evt.EventYear,
		}
		sys.FleetIDs = appendIfMissing(sys.FleetIDs, d.FleetID)
		delete(view.InTransit, d.FleetID)

	case EventFleetDeparted:
		d, ok := evt.Details.(*FleetDepartureDetails)
		if !ok {
			return
		}
		// Remove from source system's known fleet list.
		src := view.System(d.SourceID)
		if src != nil {
			src.FleetIDs = removeString(src.FleetIDs, d.FleetID)
		}
		// Remove from the stationed snapshot map; the fleet is now in transit.
		delete(view.Fleets, d.FleetID)
		// Insert into the in-transit map.
		if view.InTransit == nil {
			view.InTransit = map[string]*KnownTransit{}
		}
		view.InTransit[d.FleetID] = &KnownTransit{
			FleetID:     d.FleetID,
			Name:        d.FleetName,
			Owner:       d.Owner,
			Units:       copyUnits(d.Units),
			SourceID:    d.SourceID,
			DestID:      d.DestID,
			DepartYear:  d.DepartYear,
			ArrivalYear: d.ArrivalYear,
			AsOfYear:    evt.EventYear,
		}

	case EventEconGrowth:
		d, ok := evt.Details.(*EconGrowthDetails)
		if !ok {
			return
		}
		sys.EconLevel = d.NewLevel

	case EventCommandArrived, EventCommandExecuted, EventCommandFailed:
		// No SolView mutation beyond AsOfYear (already advanced above).

	case EventReporterReturn:
		// Decorative for the player; no SolView mutation.

	case EventGameOver:
		// No SolView mutation; broadcast occurs through a separate path.

	default:
		log.Printf("propagator: unhandled event type %q for event %q", evt.Type, evt.ID)
	}

	// Silence unused-variable for cat in this revision; the catalog will be
	// consulted by future cases (e.g. distance-aware logic) without changing
	// the function signature.
	_ = cat
}

// applyMobileConstructionToFleet applies a completed mobile-unit construction
// to SolView. The truth-side ExecuteConstruct records the chosen FleetID on
// ConstructionDetails so the propagator mirrors that exact decision. (DR-5)
func applyMobileConstructionToFleet(view *PlayerView, sys *KnownSystem, d *ConstructionDetails, eventYear float64) {
	if d.FleetID == "" {
		// Defensive: a mobile construction event without a recorded fleet
		// ID indicates a truth-side bug. Warn and skip rather than panic.
		log.Printf("propagator: mobile EventConstructionDone at %q has empty FleetID", sys.ID)
		return
	}
	f, ok := view.Fleets[d.FleetID]
	if !ok {
		f = &KnownFleet{
			ID:         d.FleetID,
			Name:       d.FleetName,
			Owner:      d.Owner,
			Units:      map[WeaponType]int{},
			LocationID: sys.ID,
			AsOfYear:   eventYear,
		}
		view.Fleets[d.FleetID] = f
		sys.FleetIDs = appendIfMissing(sys.FleetIDs, d.FleetID)
	}
	if f.Units == nil {
		f.Units = map[WeaponType]int{}
	}
	f.Units[d.WeaponType] += d.Quantity
	if eventYear > f.AsOfYear {
		f.AsOfYear = eventYear
	}
}

// applyMobileLossesToFleets distributes the mobile entries of humanLosses
// across the known fleets at sys, proportionally to each fleet's current
// snapshot count for that weapon type. Fleets reduced to zero total units
// are removed from view.Fleets and from sys.FleetIDs.
func applyMobileLossesToFleets(view *PlayerView, sys *KnownSystem, humanLosses map[WeaponType]int) {
	if len(humanLosses) == 0 || len(sys.FleetIDs) == 0 {
		return
	}
	for wt, lost := range humanLosses {
		if lost <= 0 || !WeaponDefs[wt].CanMove {
			continue
		}
		// Total of this weapon type across known fleets at sys.
		total := 0
		for _, fid := range sys.FleetIDs {
			if f := view.Fleets[fid]; f != nil {
				total += f.Units[wt]
			}
		}
		if total <= 0 {
			continue
		}
		remaining := lost
		// First pass: proportional integer apportionment.
		for _, fid := range sys.FleetIDs {
			f := view.Fleets[fid]
			if f == nil || f.Units[wt] == 0 {
				continue
			}
			share := lost * f.Units[wt] / total
			if share > f.Units[wt] {
				share = f.Units[wt]
			}
			f.Units[wt] -= share
			remaining -= share
		}
		// Second pass: drop any rounding remainder onto the first fleet
		// that still has units of this type.
		for _, fid := range sys.FleetIDs {
			if remaining <= 0 {
				break
			}
			f := view.Fleets[fid]
			if f == nil || f.Units[wt] == 0 {
				continue
			}
			take := remaining
			if take > f.Units[wt] {
				take = f.Units[wt]
			}
			f.Units[wt] -= take
			remaining -= take
		}
	}
	// Remove fleets that have been reduced to zero total units.
	kept := sys.FleetIDs[:0]
	for _, fid := range sys.FleetIDs {
		f := view.Fleets[fid]
		if f == nil {
			continue
		}
		if fleetIsEmpty(f) {
			delete(view.Fleets, fid)
			continue
		}
		kept = append(kept, fid)
	}
	sys.FleetIDs = kept
}

// fleetIsEmpty reports whether a KnownFleet has no remaining units.
func fleetIsEmpty(f *KnownFleet) bool {
	for _, n := range f.Units {
		if n > 0 {
			return false
		}
	}
	return true
}
