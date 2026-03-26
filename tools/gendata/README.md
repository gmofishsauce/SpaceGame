# gendata

Build-time data generator for the SpaceGame star map.  Reads two CSV files
from the project root and writes a JavaScript module to `proto/src/stardata.js`.

## Usage

Run from the project root:

```
go run ./tools/gendata
```

Both input files must be present in the working directory.  The output file is
overwritten on every run.

## Inputs

### nearest.csv

The primary star dataset.  Each row describes one star (or one component of a
multi-star system) within roughly 20 light-years of Sol.  The generator reads
approximately 108 entries and groups co-located stars (identical RA, Dec, and
distance strings) into a single scene marker.

Key columns used (0-indexed):

| Column | Field |
|--------|-------|
| 1 | Catalog name (e.g. `GJ 551`) |
| 4 | Right ascension (h m s) |
| 5 | Declination (deg arcmin arcsec) |
| 9 | Distance in light-years |
| 15 | Common name (e.g. `Proxima Centauri`; `-` if absent) |

Stars with missing coordinates or distance are skipped with a warning.  The Sun
(`SUN`) is placed at the origin as `Sol`.

### planets.csv

A supplementary table listing confirmed planets for nearby star systems.
Expected columns (0-indexed):

| Column | Field |
|--------|-------|
| 0 | Star name |
| 3 | Confirmed planets (`;`-separated list, or `None` / `None (...)`) |

A star is considered to have confirmed planets when the Confirmed Planets field
is non-empty and does not begin with `None`.

## Name matching

The generator matches each star in `nearest.csv` against `planets.csv` using
the star's catalog name and common name.  Three normalizations are applied to
improve hit rate:

1. **Case-insensitive** — `alpha Centauri A` matches `Alpha Centauri A`.
2. **Parenthetical alias extraction** — `Luyten 726-8 A (BL Ceti)` also
   registers the alias `BL Ceti`, matching the common name used in
   `nearest.csv`.
3. **`Gliese` → `GJ` substitution** — `Gliese 687` is normalized to `GJ 687`
   to match the catalog-name style used in `nearest.csv`.

### Known mismatches

Two planet-bearing stars are not matched by the current logic and will have
`hasPlanets: false` in the output:

| nearest.csv display name | planets.csv name | Reason |
|--------------------------|------------------|--------|
| GX Andromedae | Groombridge 34 A (GX And) | Alias `GX And` ≠ `GX Andromedae` |
| Ross 780 | Gliese 876 | Normalized `gj 876` ≠ catalog name `gj 876 a` |

## Output

`proto/src/stardata.js` — an ES module exporting a single constant:

```js
export const STAR_DATA = [
  { x: ..., y: ..., z: ..., displayName: "...", isSol: false, hasPlanets: false },
  ...
];
```

Coordinates are in light-years in the Three.js scene frame (y-up).  The
coordinate remapping from the astronomical equatorial frame is:

```
js_x = astro_x
js_y = astro_z   (celestial north = up)
js_z = −astro_y
```
