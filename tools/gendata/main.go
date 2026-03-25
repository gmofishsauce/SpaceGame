package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
)

// rawStar holds one parsed CSV row (before grouping).
type rawStar struct {
	raStr       string
	decStr      string
	distStr     string
	distLY      float64
	catalogName string
	commonName  string
	isSol       bool
}

// StarGroup is the output unit: one or more co-located stars merged into
// a single scene marker.
type StarGroup struct {
	X           float64
	Y           float64
	Z           float64
	DisplayName string
	IsSol       bool
}

type groupKey struct {
	ra, dec, dist string
}

type partialGroup struct {
	X, Y, Z float64
	names   []string
}

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
	if raHours < 0 || raHours >= 24 {
		return 0, fmt.Errorf("RA hours out of range: %f", raHours)
	}
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
	if decDeg < -90 || decDeg > 90 {
		return 0, fmt.Errorf("Dec out of range: %f", decDeg)
	}
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
	d := distLY
	ax := d * math.Cos(delta) * math.Cos(alpha)
	ay := d * math.Cos(delta) * math.Sin(alpha)
	az := d * math.Sin(delta)
	return ax, ay, az, nil
}

func loadAndProcessStars(csvPath string) ([]StarGroup, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", csvPath, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true

	// discard header
	if _, err := r.Read(); err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	var rawStars []rawStar
	rowNum := 1
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNum, err)
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
			rawStars = append(rawStars, rawStar{
				isSol:       true,
				catalogName: "Sol",
			})
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
			return nil, fmt.Errorf("row %d: bad distance %q: %w", rowNum-1, distStr, err)
		}

		rawStars = append(rawStars, rawStar{
			raStr:       raStr,
			decStr:      decStr,
			distStr:     distStr,
			distLY:      distLY,
			catalogName: catalogName,
			commonName:  commonName,
			isSol:       false,
		})
	}

	if len(rawStars) < 2 {
		return nil, fmt.Errorf("too few rows parsed: %d", len(rawStars))
	}

	// Group co-located stars
	groupMap := map[groupKey]*partialGroup{}
	groupOrder := []groupKey{}
	var solGroup *StarGroup

	for _, s := range rawStars {
		if s.isSol {
			solGroup = &StarGroup{X: 0, Y: 0, Z: 0, DisplayName: "Sol", IsSol: true}
			continue
		}

		ax, ay, az, err := convertToAstroCartesian(s.raStr, s.decStr, s.distLY)
		if err != nil {
			return nil, fmt.Errorf("star %q: %w", s.catalogName, err)
		}
		// Remap: js_x = astro_x, js_y = astro_z, js_z = -astro_y
		jx := ax
		jy := az
		jz := -ay

		preferred := s.commonName
		if preferred == "" {
			preferred = s.catalogName
		}

		k := groupKey{s.raStr, s.decStr, s.distStr}
		if pg, exists := groupMap[k]; exists {
			pg.names = append(pg.names, preferred)
		} else {
			groupMap[k] = &partialGroup{X: jx, Y: jy, Z: jz, names: []string{preferred}}
			groupOrder = append(groupOrder, k)
		}
	}

	result := []StarGroup{}
	if solGroup != nil {
		result = append(result, *solGroup)
	}
	for _, k := range groupOrder {
		pg := groupMap[k]
		displayName := strings.Join(pg.names, " / ")
		result = append(result, StarGroup{
			X:           pg.X,
			Y:           pg.Y,
			Z:           pg.Z,
			DisplayName: displayName,
			IsSol:       false,
		})
	}

	return result, nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func main() {
	groups, err := loadAndProcessStars("nearest.csv")
	if err != nil {
		log.Fatalf("loadAndProcessStars: %v", err)
	}
	if len(groups) < 2 {
		log.Fatalf("no star groups produced (only Sol or empty)")
	}

	outPath := "proto/src/stardata.js"
	f, err := os.Create(outPath)
	if err != nil {
		log.Fatalf("create %q: %v", outPath, err)
	}
	defer f.Close()

	fmt.Fprintln(f, "// AUTO-GENERATED by tools/gendata — do not edit by hand.")
	fmt.Fprintln(f, "// Regenerate with: go run ./tools/gendata")
	fmt.Fprintln(f)
	fmt.Fprintln(f, "export const STAR_DATA = [")
	for _, g := range groups {
		isSolStr := "false"
		if g.IsSol {
			isSolStr = "true"
		}
		fmt.Fprintf(f, "  { x: %s, y: %s, z: %s, displayName: %s, isSol: %s },\n",
			formatFloat(g.X),
			formatFloat(g.Y),
			formatFloat(g.Z),
			jsonString(g.DisplayName),
			isSolStr,
		)
	}
	fmt.Fprintln(f, "];")

	log.Printf("wrote %d entries to %s", len(groups), outPath)
}
