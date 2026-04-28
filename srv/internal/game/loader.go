package game

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Initialize loads nearest.csv and planets.csv, builds and returns the initial
// GameState. (FR-005 through FR-010)
//
// rng is injected (DR-11) so tests can seed deterministically. main.go passes
// a time-seeded RNG to preserve current default behavior.
func Initialize(rng *rand.Rand, nearestCSVPath, planetsCSVPath string) (*GameState, error) {
	hasPlanets, err := loadPlanets(planetsCSVPath)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", planetsCSVPath, err)
	}

	groups, maxDist, err := loadStars(nearestCSVPath, hasPlanets)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", nearestCSVPath, err)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("nearest.csv: no usable star records")
	}

	// 1. Build the immutable star catalog.
	entries := make([]*CatalogEntry, 0, len(groups))
	for _, g := range groups {
		id := toSystemID(g.DisplayName)
		entries = append(entries, &CatalogEntry{
			ID:          id,
			DisplayName: g.DisplayName,
			X:           g.X,
			Y:           g.Y,
			Z:           g.Z,
			DistFromSol: g.DistFromSol,
			HasPlanets:  g.HasPlanets,
			IsSol:       g.IsSol,
		})
	}
	catalog := NewStarCatalog(entries)

	// 2. Build ground truth and the initial player view.
	truth := &Truth{
		Systems: make(map[string]*TrueSystem, len(entries)),
		Fleets:  map[string]*TrueFleet{},
	}
	view := &SolView{
		Systems:   make(map[string]*KnownSystem, len(entries)),
		Fleets:    map[string]*KnownFleet{},
		InTransit: map[string]*KnownTransit{},
	}

	state := &GameState{
		Catalog:     catalog,
		truth:       truth,
		SolView:     view,
		Events:      NewEventLog(),
		PendingCmds: []*PendingCommand{},
		rng:         rng,
	}

	// 3. Populate Truth and seed SolView from initial Truth (G-4).
	for _, g := range groups {
		id := toSystemID(g.DisplayName)
		isSol := g.IsSol

		ts := &TrueSystem{
			ID:         id,
			LocalUnits: map[WeaponType]int{},
		}

		// Determine initial status (FR-006, FR-007)
		if isSol {
			ts.Status = StatusHuman
			ts.EconLevel = 4
			ts.Wealth = 64
			ts.EconGrowthYear = EconGrowthIntervalYears // Sol already at max; won't grow
			ts.LocalUnits[WeaponCommLaser] = 1
		} else if g.HasPlanets {
			ts.Status = StatusHuman
			ts.EconLevel = gaussianEconLevel(rng)
			ts.EconGrowthYear = EconGrowthIntervalYears
		} else if g.DistFromSol <= maxDist/2.0 {
			ts.Status = StatusHuman
			ts.EconLevel = gaussianEconLevel(rng)
			ts.EconGrowthYear = EconGrowthIntervalYears
		} else {
			ts.Status = StatusUninhabited
			ts.EconLevel = 0
		}

		// Tuned for gameplay balance: every human-held system starts with two
		// Reporter ships and (if it has planets) a Comm Laser.
		if ts.Status == StatusHuman {
			fid := state.NewFleetID()
			fname := g.DisplayName + "-1st Fleet"
			tf := &TrueFleet{
				ID:         fid,
				Name:       fname,
				Owner:      HumanOwner,
				Units:      map[WeaponType]int{WeaponReporter: 2},
				LocationID: id,
				InTransit:  false,
			}
			truth.Fleets[fid] = tf
			ts.FleetIDs = append(ts.FleetIDs, fid)
			ts.PrimaryFleetID = fid
			ts.FleetCount = 1

			if g.HasPlanets {
				ts.LocalUnits[WeaponCommLaser] = 1
			}

			// Mirror the human fleet into SolView as a snapshot (DR-4).
			view.Fleets[fid] = &KnownFleet{
				ID:         fid,
				Name:       fname,
				Owner:      HumanOwner,
				Units:      copyUnits(tf.Units),
				LocationID: id,
				AsOfYear:   0,
			}
		}

		truth.Systems[id] = ts

		// Seed SolView from initial Truth (G-4): the player's pre-game view
		// matches ground truth, EXCEPT alien presence will be added below
		// without updating the view (FR-010).
		ks := &KnownSystem{
			ID:         id,
			Status:     ts.Status,
			AsOfYear:   0,
			EconLevel:  ts.EconLevel,
			Wealth:     ts.Wealth,
			LocalUnits: copyUnits(ts.LocalUnits),
			FleetIDs:   append([]string(nil), ts.FleetIDs...),
		}
		view.Systems[id] = ks
	}

	// Record initial human systems for win condition (FR-056)
	for id, sys := range truth.Systems {
		if sys.Status == StatusHuman {
			state.Human.InitialSystemIDs = append(state.Human.InitialSystemIDs, id)
		}
	}

	// 4. Select alien entry points from peripheral systems (FR-009, A-2).
	var peripheral []string
	for id, sys := range truth.Systems {
		_ = sys
		e := catalog.Get(id)
		if e != nil && e.DistFromSol > PeripheryFraction*maxDist {
			peripheral = append(peripheral, id)
		}
	}
	rng.Shuffle(len(peripheral), func(i, j int) { peripheral[i], peripheral[j] = peripheral[j], peripheral[i] })

	count := AlienEntryCount
	if count > len(peripheral) {
		log.Printf("warning: only %d peripheral systems available for %d alien entry points", len(peripheral), count)
		count = len(peripheral)
	}

	for i := 0; i < count; i++ {
		epID := peripheral[i]
		ts := truth.Systems[epID]
		state.Alien.EntryPointIDs = append(state.Alien.EntryPointIDs, epID)

		// Place initial alien fleet at entry point (G-1)
		fleetID := state.NewFleetID()
		fleetName := state.NewFleetName()
		tf := &TrueFleet{
			ID:         fleetID,
			Name:       fleetName,
			Owner:      AlienOwner,
			Units:      copyUnits(AlienInitialComposition),
			LocationID: epID,
			InTransit:  false,
		}
		truth.Fleets[fleetID] = tf
		ts.FleetIDs = append(ts.FleetIDs, fleetID)

		// Ground truth: system is alien-held. SolView intentionally NOT updated
		// — player doesn't know about aliens yet (FR-010). The SolView entry
		// retains its pre-alien Status (StatusUninhabited or StatusHuman as
		// determined above, before we overwrite ts.Status here).
		ts.Status = StatusAlien
		ts.EconLevel = 0
	}

	state.Alien.NextSpawnYear = AlienSpawnIntervalYears

	return state, nil
}

// --- CSV loading helpers ---

// starGroup mirrors the gendata StarGroup but includes DistFromSol.
type starGroup struct {
	X, Y, Z     float64
	DistFromSol float64
	DisplayName string
	IsSol       bool
	HasPlanets  bool
}

// loadPlanets reads planets.csv and returns a set of normalized star names
// that have at least one confirmed planet. Mirrors tools/gendata logic.
func loadPlanets(csvPath string) (map[string]bool, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", csvPath, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true
	if _, err := r.Read(); err != nil { // discard header
		return nil, fmt.Errorf("reading header: %w", err)
	}

	set := map[string]bool{}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading %q: %w", csvPath, err)
		}
		if len(record) < 4 {
			continue
		}
		starName := strings.TrimSpace(record[0])
		confirmed := strings.TrimSpace(record[3])
		hp := confirmed != "" && !strings.EqualFold(confirmed, "none") &&
			!strings.HasPrefix(strings.ToLower(confirmed), "none")

		addName := func(name string) {
			key := normalizeName(name)
			if hp {
				set[key] = true
			} else if _, exists := set[key]; !exists {
				set[key] = false
			}
		}
		addName(starName)
		if i := strings.Index(starName, "("); i >= 0 {
			if j := strings.Index(starName, ")"); j > i {
				addName(strings.TrimSpace(starName[i+1 : j]))
			}
		}
	}
	return set, nil
}

// loadStars reads nearest.csv, groups co-located stars, and returns a sorted
// slice of starGroup entries (Sol first, then by distance) plus the max distance.
func loadStars(csvPath string, hasPlanetsSet map[string]bool) ([]starGroup, float64, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open %q: %w", csvPath, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true
	if _, err := r.Read(); err != nil { // discard header
		return nil, 0, fmt.Errorf("reading header: %w", err)
	}

	type groupKey struct{ ra, dec, dist string }
	type partialGroup struct {
		x, y, z    float64
		distLY     float64
		names      []string
		hasPlanets bool
	}

	groupMap := map[groupKey]*partialGroup{}
	var groupOrder []groupKey
	var solGroup *starGroup

	rowNum := 1
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("row %d: %w", rowNum, err)
		}
		rowNum++

		if len(record) < 16 {
			continue
		}

		catalogName := strings.TrimSpace(record[1])
		isSol := catalogName == "SUN"

		commonName := strings.TrimSpace(record[15])
		if commonName == "-" {
			commonName = ""
		}

		if isSol {
			solGroup = &starGroup{X: 0, Y: 0, Z: 0, DistFromSol: 0, DisplayName: "Sol", IsSol: true}
			continue
		}

		raStr := strings.TrimSpace(record[4])
		decStr := strings.TrimSpace(record[5])
		distStr := strings.TrimSpace(record[9])

		if raStr == "-" || decStr == "-" || distStr == "" {
			log.Printf("warning: row %d (%s): missing coordinate/distance, skipping", rowNum-1, catalogName)
			continue
		}

		distLY, err := strconv.ParseFloat(distStr, 64)
		if err != nil {
			log.Printf("warning: row %d: bad distance %q: %v, skipping", rowNum-1, distStr, err)
			continue
		}

		ax, ay, az, err := convertToAstroCartesian(raStr, decStr, distLY)
		if err != nil {
			log.Printf("warning: row %d (%s): coordinate conversion: %v, skipping", rowNum-1, catalogName, err)
			continue
		}
		// Remap to Three.js: js_x = astro_x, js_y = astro_z, js_z = -astro_y
		jx := ax
		jy := az
		jz := -ay

		preferred := commonName
		if preferred == "" {
			preferred = catalogName
		}

		hp := starHasPlanets(catalogName, commonName, hasPlanetsSet)

		k := groupKey{raStr, decStr, distStr}
		if pg, exists := groupMap[k]; exists {
			pg.names = append(pg.names, preferred)
			if hp {
				pg.hasPlanets = true
			}
		} else {
			groupMap[k] = &partialGroup{x: jx, y: jy, z: jz, distLY: distLY, names: []string{preferred}, hasPlanets: hp}
			groupOrder = append(groupOrder, k)
		}
	}

	// Assemble result slice
	var groups []starGroup
	if solGroup != nil {
		solGroup.HasPlanets = false // Sol's planets are not in planets.csv
		groups = append(groups, *solGroup)
	}
	for _, k := range groupOrder {
		pg := groupMap[k]
		groups = append(groups, starGroup{
			X:           pg.x,
			Y:           pg.y,
			Z:           pg.z,
			DistFromSol: pg.distLY,
			DisplayName: strings.Join(pg.names, " / "),
			IsSol:       false,
			HasPlanets:  pg.hasPlanets,
		})
	}

	if len(groups) < 2 {
		return nil, 0, fmt.Errorf("too few star records parsed: %d", len(groups))
	}

	// Sort non-Sol systems by distance
	solEntry := groups[0]
	rest := groups[1:]
	sort.Slice(rest, func(i, j int) bool {
		return rest[i].DistFromSol < rest[j].DistFromSol
	})
	groups = append([]starGroup{solEntry}, rest...)

	// Compute max distance
	maxDist := 0.0
	for _, g := range groups {
		if g.DistFromSol > maxDist {
			maxDist = g.DistFromSol
		}
	}

	return groups, maxDist, nil
}

// --- Coordinate helpers (mirrored from tools/gendata) ---

func parseRA(raStr string) (float64, error) {
	parts := strings.Fields(raStr)
	if len(parts) != 3 {
		return 0, fmt.Errorf("expected 3 parts in RA %q, got %d", raStr, len(parts))
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("bad RA hours %q: %w", parts[0], err)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("bad RA minutes %q: %w", parts[1], err)
	}
	s, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, fmt.Errorf("bad RA seconds %q: %w", parts[2], err)
	}
	raHours := float64(h) + float64(m)/60.0 + s/3600.0
	return raHours * (math.Pi / 12.0), nil
}

func parseDec(decStr string) (float64, error) {
	parts := strings.Fields(decStr)
	if len(parts) != 3 {
		return 0, fmt.Errorf("expected 3 parts in Dec %q, got %d", decStr, len(parts))
	}
	deg, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("bad Dec degrees %q: %w", parts[0], err)
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("bad Dec minutes %q: %w", parts[1], err)
	}
	sec, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, fmt.Errorf("bad Dec seconds %q: %w", parts[2], err)
	}
	sign := 1.0
	if deg < 0 {
		sign = -1.0
	}
	decDeg := float64(deg) + sign*(float64(min)/60.0+sec/3600.0)
	return decDeg * (math.Pi / 180.0), nil
}

func convertToAstroCartesian(raStr, decStr string, distLY float64) (float64, float64, float64, error) {
	alpha, err := parseRA(raStr)
	if err != nil {
		return 0, 0, 0, err
	}
	delta, err := parseDec(decStr)
	if err != nil {
		return 0, 0, 0, err
	}
	ax := distLY * math.Cos(delta) * math.Cos(alpha)
	ay := distLY * math.Cos(delta) * math.Sin(alpha)
	az := distLY * math.Sin(delta)
	return ax, ay, az, nil
}

// normalizeName lowercases and normalises "Gliese " to "GJ " for matching.
func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.ReplaceAll(s, "gliese ", "gj ")
}

func starHasPlanets(catalogName, commonName string, set map[string]bool) bool {
	if set[normalizeName(commonName)] {
		return true
	}
	if set[normalizeName(catalogName)] {
		return true
	}
	return false
}

// toSystemID converts a display name to a stable lowercase ID.
// "GJ 551" → "gj-551"; "Sol" → "sol"; "Alpha Centauri A / Alpha Centauri B" → "alpha-centauri-a-alpha-centauri-b"
func toSystemID(displayName string) string {
	s := strings.ToLower(displayName)
	s = strings.ReplaceAll(s, " / ", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

// gaussianEconLevel samples an economic level from N(mean, stddev), clamped to [1, 5].
func gaussianEconLevel(rng *rand.Rand) int {
	// Box-Muller transform
	u1 := rng.Float64()
	u2 := rng.Float64()
	for u1 == 0 {
		u1 = rng.Float64()
	}
	z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	v := EconLevelMean + EconLevelStddev*z
	iv := int(math.Round(v))
	if iv < 1 {
		return 1
	}
	if iv > 4 {
		return 4
	}
	return iv
}
