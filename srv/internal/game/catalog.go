package game

import "math"

// StarCatalog holds static, never-changing star data (positions, distances,
// names, planet flag) separately from anything that can vary. Both Truth and
// SolView reference the catalog by ID; star geometry is the same in both
// views. Built once at load time and immutable thereafter.
type StarCatalog struct {
	Order   []string                 // ID order: "sol" first, then by distance
	Entries map[string]*CatalogEntry // keyed by system ID

	maxDist float64 // computed at construction
}

// CatalogEntry is one immutable star/system descriptor.
type CatalogEntry struct {
	ID          string
	DisplayName string
	X, Y, Z     float64
	DistFromSol float64
	HasPlanets  bool
	IsSol       bool
}

// NewStarCatalog builds a catalog from an ordered slice of entries. The
// caller is responsible for ordering (sol first, then by distance from Sol).
// Computes MaxDist once.
func NewStarCatalog(entries []*CatalogEntry) *StarCatalog {
	c := &StarCatalog{
		Order:   make([]string, 0, len(entries)),
		Entries: make(map[string]*CatalogEntry, len(entries)),
	}
	for _, e := range entries {
		c.Order = append(c.Order, e.ID)
		c.Entries[e.ID] = e
		if e.DistFromSol > c.maxDist {
			c.maxDist = e.DistFromSol
		}
	}
	return c
}

// Get returns the CatalogEntry for id, or nil if id is not in the catalog.
func (c *StarCatalog) Get(id string) *CatalogEntry {
	if c == nil {
		return nil
	}
	return c.Entries[id]
}

// MaxDist returns the largest DistFromSol over all entries.
func (c *StarCatalog) MaxDist() float64 {
	if c == nil {
		return 0
	}
	return c.maxDist
}

// Distance returns the Euclidean distance in light-years between two systems
// in the catalog. Returns 0 if either id is unknown.
func (c *StarCatalog) Distance(id1, id2 string) float64 {
	a := c.Get(id1)
	b := c.Get(id2)
	if a == nil || b == nil {
		return 0
	}
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
