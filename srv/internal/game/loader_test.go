package game

import (
	"math/rand"
	"os"
	"testing"
)

// writeTemp writes content to a temp file and returns its path. The test
// registers cleanup to delete it.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", name)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

// CSV constants mirror the HYG nearest.csv schema (16 columns, SUN sentinel).
//
// Column layout (0-indexed):
//   0=Rank 1=Name 2=Obj# 3=LHS 4=RA 5=DEC 6=PM 7=Angle 8=Parallax
//   9=dist(LY, column header is empty) 10=Error 11=Spectral 12=V 13=Mv 14=Mass 15=common_name
//
// The header uses ,, between Parallax and Error, creating an unnamed column 9.
// Sol row uses - for columns 3-8 and 10; column 9 (dist) is empty; column 15 is empty.
// Data rows fill column 9 with the actual LY distance.

const hdrNearest = "Rank,Name,Obj#,LHS,RA (2000.0),DEC (2000.0),PM,Angle,Parallax,,Error,Spectral Type,V,Mv,Mass,common name\n"

// solRow: 16 fields — col9 (dist) empty, col15 (common name) empty (trailing comma).
// Field breakdown: 0,SUN,1, -(LHS), -(RA), -(DEC), -(PM), -(Angle), -(Parallax), [empty dist], -(Error), G2, -26, 4.85, 1, [empty cname]
const solRow = "0,SUN,1,-,-,-,-,-,-,,-,G2,-26,4.85,1,\n"

// minimalHumanCSV: Sol (sentinel) + Alpha Centauri (planet-bearing, dist 4.24 LY).
const minimalHumanCSV = hdrNearest + solRow +
	"1,GJ 551,-,49,14 29 43.0,-62 40 46,3.8,281,0.769,4.24,0.0003,M5,11,15,0.11,Alpha Centauri\n"

// minimalPlanetsCSV marks Alpha Centauri as planet-bearing.
const minimalPlanetsCSV = "Star Name,Distance (ly),System Status,Confirmed Planets\n" +
	"Alpha Centauri,4.24,Known,Alpha b\n"

// minimalAlienCSV: no SUN sentinel; 61 UMa (home, planet-bearing) + one uninhabited.
const minimalAlienCSV = hdrNearest +
	"1,HD 95128,-,49,11 00 24.0,+40 25 48,0.4,271,0.071,46.98,0.001,G1,5.0,4.3,1.02,61 Ursae Majoris\n" +
	"2,GJ 447,-,49,11 47 44.4,+08 06 22,1.2,180,0.097,33.60,0.001,M4,10.0,13.5,0.35,\n"

// minimalAlienPlanetsCSV marks 61 UMa as planet-bearing.
const minimalAlienPlanetsCSV = "Star Name,Distance (ly),System Status,Confirmed Planets\n" +
	"61 Ursae Majoris,46.98,Known,61 UMa b\n"

func testInitialize(t *testing.T) *GameState {
	t.Helper()
	human := writeTemp(t, "nearest*.csv", minimalHumanCSV)
	planets := writeTemp(t, "planets*.csv", minimalPlanetsCSV)
	alien := writeTemp(t, "alien-nearest*.csv", minimalAlienCSV)
	alienPlanets := writeTemp(t, "alien-planets*.csv", minimalAlienPlanetsCSV)
	rng := rand.New(rand.NewSource(42))
	st, err := Initialize(rng, human, planets, alien, alienPlanets)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return st
}

// TestLoader_HomesSeeded verifies that both home systems are present in Truth
// and are seeded with EconLevel 5, Wealth 64, and a comm laser. (FR-4, FR-5)
func TestLoader_HomesSeeded(t *testing.T) {
	st := testInitialize(t)

	for owner, homeID := range st.Homes {
		sys := st.truth.Systems[homeID]
		if sys == nil {
			t.Errorf("%s home %q not found in truth", owner, homeID)
			continue
		}
		if sys.EconLevel != 5 {
			t.Errorf("%s home EconLevel=%d, want 5", owner, sys.EconLevel)
		}
		if sys.Wealth != 64 {
			t.Errorf("%s home Wealth=%.1f, want 64", owner, sys.Wealth)
		}
		if sys.LocalUnits[WeaponCommLaser] != 1 {
			t.Errorf("%s home comm lasers=%d, want 1", owner, sys.LocalUnits[WeaponCommLaser])
		}
	}
}

// TestLoader_BothViewsMirroredAtT0 verifies that every system visible in
// truth is present in both player views with matching status and econ level
// at year 0. (FR-17)
func TestLoader_BothViewsMirroredAtT0(t *testing.T) {
	st := testInitialize(t)

	for id, ts := range st.truth.Systems {
		for _, player := range []Owner{HumanOwner, AlienOwner} {
			ks := st.Views[player].Systems[id]
			if ks == nil {
				t.Errorf("player %s: system %q missing from view", player, id)
				continue
			}
			if ks.Status != ts.Status {
				t.Errorf("player %s system %q: view status=%v truth status=%v", player, id, ks.Status, ts.Status)
			}
			if ks.EconLevel != ts.EconLevel {
				t.Errorf("player %s system %q: view econLevel=%d truth econLevel=%d", player, id, ks.EconLevel, ts.EconLevel)
			}
			if ks.AsOfYear != 0 {
				t.Errorf("player %s system %q: AsOfYear=%.1f, want 0", player, id, ks.AsOfYear)
			}
		}
	}
}

// TestLoader_InitialSystemIDsPopulated verifies that Faction.InitialSystemIDs
// is populated for both sides and contains at least the home system. (FR-8)
func TestLoader_InitialSystemIDsPopulated(t *testing.T) {
	st := testInitialize(t)

	for owner, fac := range st.Factions {
		if len(fac.InitialSystemIDs) == 0 {
			t.Errorf("%s: InitialSystemIDs is empty", owner)
			continue
		}
		homeID := st.Homes[owner]
		found := false
		for _, id := range fac.InitialSystemIDs {
			if id == homeID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: home %q not in InitialSystemIDs %v", owner, homeID, fac.InitialSystemIDs)
		}
	}
}

// TestLoader_DuplicateIDFails verifies that Initialize fails fast when a
// system ID appears in both the human and alien CSV files. (FR-31)
func TestLoader_DuplicateIDFails(t *testing.T) {
	// Alien CSV reuses "Alpha Centauri" which is already in the human CSV.
	dupAlienCSV := hdrNearest +
		"1,GJ DUP,-,49,14 29 43.0,-62 40 46,3.8,281,0.769,4.24,0.0003,M5,11,15,0.11,Alpha Centauri\n" +
		"2,HD 95128,-,49,11 00 24.0,+40 25 48,0.4,271,0.071,46.98,0.001,G1,5.0,4.3,1.02,61 Ursae Majoris\n"
	human := writeTemp(t, "nearest*.csv", minimalHumanCSV)
	planets := writeTemp(t, "planets*.csv", minimalPlanetsCSV)
	alien := writeTemp(t, "alien-nearest*.csv", dupAlienCSV)
	alienPlanets := writeTemp(t, "alien-planets*.csv", minimalAlienPlanetsCSV)
	rng := rand.New(rand.NewSource(0))
	_, err := Initialize(rng, human, planets, alien, alienPlanets)
	if err == nil {
		t.Fatal("expected error for duplicate system ID, got nil")
	}
}

// TestLoader_MissingAlienHomeFails verifies that Initialize fails when the
// alien home star is not found in the alien CSV. (AS-2)
func TestLoader_MissingAlienHomeFails(t *testing.T) {
	noHomeCSV := hdrNearest +
		"1,GJ VEGA,-,49,18 36 56.3,+38 47 01,0.3,35,0.129,25.30,0.001,A0,0.03,-0.57,2.13,Vega\n"
	human := writeTemp(t, "nearest*.csv", minimalHumanCSV)
	planets := writeTemp(t, "planets*.csv", minimalPlanetsCSV)
	alien := writeTemp(t, "alien-nearest*.csv", noHomeCSV)
	alienPlanets := writeTemp(t, "alien-planets*.csv", "Star Name,Distance (ly),System Status,Confirmed Planets\n")
	rng := rand.New(rand.NewSource(0))
	_, err := Initialize(rng, human, planets, alien, alienPlanets)
	if err == nil {
		t.Fatal("expected error for missing alien home, got nil")
	}
}
