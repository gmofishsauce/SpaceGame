# Design: Star Tank Prototype

---

## 1. Overview

Star Tank is a single-page web application that renders an interactive 3D visualization of the 108 nearest star systems to Earth in the style of a science fiction "star tank" holographic display. All prototype frontend files live under `proto/`. A Go data-generator program (`tools/gendata/main.go`) — kept at the project root so it can be reused by any future product build — reads `nearest.csv` and writes `proto/src/stardata.js`. Vite's built-in development server (run from `proto/`) serves the application on port 5173, eliminating the need for a separate HTTP server for the prototype. All rendering is performed client-side using Three.js. The application is a prototype used by a solo developer to evaluate visual suitability as a game foundation.

---

## 2. Requirements Summary

### Functional Requirements

#### 2.1 Web Server

- **FR-001**: The system includes a server that listens on port 8080 and serves the application. *(See Section 3 — this requirement is superseded by a deliberate design decision to use Vite's dev server on port 5173 for the prototype.)*
- **FR-002**: The server serves a single HTML page (with CSS and JavaScript) at the root path `/`.
- **FR-003**: The server has no external Go dependencies; only the standard library is used. *(N/A for the prototype — no Go server is used; see Section 3.)*

#### 2.2 Star Data

- **FR-004**: The star dataset (108 entries from `nearest.csv`) is embedded directly in the JavaScript source. The client makes no runtime fetch of a data file.
- **FR-005**: Each star's position is converted from equatorial spherical coordinates (RA in hours/minutes/seconds, Dec in degrees/arcminutes/arcseconds, distance in light-years) to Cartesian (x, y, z) using the standard equatorial-to-Cartesian formula, with Sol at the origin (0, 0, 0).
- **FR-006**: Stars that occupy identical computed positions (co-located pairs or groups, e.g., binary systems represented by a single point in the CSV) are treated as a single marker. All co-located star names are associated with that marker.
- **FR-007**: For each star the system stores: Cartesian position, catalog name, common name (if present), a derived `displayName`, and a boolean `hasPlanets` indicating whether at least one confirmed exoplanet is known in that system.

#### 2.3 Scene Rendering

- **FR-008**: The scene renders on a black background.
- **FR-009**: Sol is rendered at (0, 0, 0) with a permanent visible label "Sol". No mouseover interaction is required for Sol.
- **FR-010**: Each non-Sol star (or co-located group) is represented by a marker rendered as a small, fixed-size screen-space dot (white, approximately 4 CSS pixels in diameter). The dot does not change apparent size as the camera zooms in or out.
- **FR-021**: Stars with `hasPlanets: true` additionally display a fixed-size screen-space ring centered on the dot. The ring does not change apparent size as the camera zooms in or out.
- **FR-022**: The planet ring is visually distinct from the dot but shares the same anchor point in 3D space.
- **FR-011**: The scene has no continuous time-driven animation. All motion is driven solely by user input (camera controls) or mouseover events.
- **FR-020**: Three permanent, solid yellow axis lines extend through the origin, one along each scene axis (x, y, z), each spanning ±25 light-years. They are rendered at startup and remain visible at all times; mouseover events do not affect them. No labels, tick marks, or scale indicators appear on the lines.

#### 2.4 Camera and Navigation

- **FR-019**: On load, the camera is positioned outside the full extent of the star dataset, along a diagonal with positive x, positive y, and positive z values (approximately equal magnitudes), at a distance greater than the farthest star, looking toward the origin.
- **FR-012**: The user can orbit the scene by clicking and dragging.
- **FR-013**: The user can zoom in and out with the scroll wheel.
- **FR-014**: The user can pan with right-click drag.

#### 2.5 Mouseover Interaction

- **FR-015**: Hovering over a star marker displays the star's name adjacent to the marker. The common name is shown if it exists; otherwise the catalog name. For co-located groups, all applicable names are shown.
- **FR-016**: Hovering over a star marker renders three dotted lines (in astronomical coordinate terms):
  - From the star's projection onto the z=0 equatorial plane — (x, y, 0) — to (x, 0, 0) on the x-axis.
  - From (x, y, 0) to (0, y, 0) on the y-axis.
  - From (x, y, 0) vertically to the star's actual position (x, y, z).
- **FR-017**: Moving the mouse off a star marker immediately removes the name label and all dotted lines.
- **FR-018**: No more than one star's label and projection lines are visible at a time.

### Non-Functional Requirements

- **NFR-001**: The application runs in a modern desktop browser (Chrome, Firefox, or Safari, current versions) with no plugins.
- **NFR-002**: The Go server starts and is ready to serve within 2 seconds on a typical developer laptop.
- **NFR-003**: Mouseover detection and label/line rendering are imperceptible in latency on a modern laptop.
- **NFR-004**: The application requires no internet connection at runtime; all assets including Three.js are served locally.

### Integration Requirements

- **IR-001**: The client uses Three.js for 3D rendering and OrbitControls for camera navigation. Both are served locally from the project; no CDN access is required at runtime.

---

## 3. Requirements Issues

### Ambiguities

- **FR-006 example is incorrect in the source data.** The requirements cite "Alpha Centauri A and B" (GJ 559 A and GJ 559 B) as an example of co-located stars. In `nearest.csv`, GJ 559 A and GJ 559 B have slightly different RA and Dec values ("14 39 36.5" / "-60 50 02" vs. "14 39 35.1" / "-60 50 14"), so they will produce distinct Cartesian positions and will **not** be grouped as a single marker by this design's algorithm. The actual co-located pairs in the dataset are GJ 244 A/B (Sirius and Sirius B, identical RA/Dec/distance) and GJ 65 A/B (BL Ceti and UV Ceti, identical RA/Dec/distance). **My design assumes exact string equality of the RA, Dec, and distance fields to determine co-location; GJ 559 A/B are separate markers.**

- **FR-015 display for co-located groups**: "For co-located groups with multiple names, all names shall be displayed" could mean: (a) all catalog names, (b) all common names, or (c) each star's preferred name (common if it exists, else catalog). **This design assumes interpretation (c): for each star in the group, show its common name if available, else its catalog name, joined with " / ".**

- **NFR-003 "appear instantaneous"** is not directly testable. **This design treats it as a requirement to perform raycasting and update the scene within a single animation frame (≤16 ms at 60 fps) on a modern laptop.**

### Gaps

- **Source file location not specified for runtime.** Section 5.1 of the requirements gives the source path as `/Users/Jeff/nearest.csv`. **This design specifies that the data generator reads `nearest.csv` from the current working directory.** The developer must run the generator from the project root.

- **FR-016 coordinate frame vs. Three.js convention.** The requirements define projection lines in terms of astronomical coordinates (x, y, z) where z is the north celestial pole (up). Three.js uses y-up conventions. This design applies a coordinate remapping at data generation time and restates FR-016 in Three.js coordinate terms. See Section 6.2.

### Contradictions

- **FR-001 vs. prototype architecture**: FR-001 specifies a Go HTTP server on port 8080; FR-003 specifies it uses only the Go standard library. The stakeholder has explicitly directed that Vite's built-in development server (port 5173) replaces the Go server for the prototype, making FR-001 and FR-003 moot. FR-002 (serving the HTML page at `/`) is satisfied by Vite. If a production deployment server is needed in the future, FR-001 and FR-003 can be revisited at that time.

### Untestable Requirements

- **NFR-003** ("appear instantaneous") — addressed above by substituting a concrete latency bound.

---

## 4. Constraints and Assumptions

### Constraints

- Data generator language: Go, standard library only (no third-party packages).
- Frontend rendering: Three.js only. No other 3D frameworks.
- Dev server: Vite on port 5173 (default). No separate HTTP server for the prototype.
- Prototype only: no authentication, HTTPS, or production hardening.
- All Three.js files must be available locally at runtime; Vite resolves them from `node_modules/` and serves them directly in dev mode.

### Assumptions

- The developer runs `npm run dev` locally and accesses the app at `http://localhost:5173`. No HTTPS or CORS handling is needed.
- `nearest.csv` is located in the project root directory (same directory from which the data generator is invoked).
- "No animation" means no `requestAnimationFrame`-driven scene updates. The design uses demand-triggered rendering: the WebGL scene re-renders only on OrbitControls change events, mousemove events, and window resize events.
- Floating-point coordinates computed from identical raw CSV strings will be bitwise identical (same Go `math` operations on the same inputs), making exact `float64` equality sufficient for co-location detection.
- The coordinate remapping described in Section 6.2 (astronomical z → Three.js y, astronomical y → Three.js −z) is acceptable for prototype evaluation purposes.
- Three.js ^0.168.0 is installed via `npm install three`. Vite resolves `three/addons/*` automatically via the Three.js package.json exports map; no import-specifier patching is needed.
- The developer uses a terminal and a modern text editor; no IDE-specific tooling is assumed.
- Node.js and npm are available on the developer's machine. Vite is already installed on the host.

---

## 5. Architecture

### 5.1 Component Overview

```
┌─────────────────────────────────────────────────────────────────┐
│  Developer's Browser  (http://localhost:5173)                   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ proto/index.html (served by Vite dev server)            │   │
│  │  • <script type="module" src="/src/main.js">            │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │                                       │
│  ┌──────────────────────▼──────────────────────────────────┐   │
│  │ proto/src/main.js (transformed on-the-fly by Vite)      │   │
│  │  • Three.js (resolved from proto/node_modules/three)    │   │
│  │  • OrbitControls (resolved from three/addons/*)         │   │
│  │  • CSS2DRenderer (resolved from three/addons/*)         │   │
│  │  • STAR_DATA (resolved from proto/src/stardata.js)      │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
         ▲ HTTP (Vite dev server, HMR websocket)
         │
┌────────┴──────────────────────────────────────────────────────┐
│  Vite Dev Server  (cd proto && npm run dev → localhost:5173)  │
│                                                               │
│  • Serves proto/index.html and proto/src/* from source tree  │
│  • Resolves bare imports (e.g. 'three') from proto/node_modules/ │
│  • Transforms ES modules on demand (no pre-bundling step)    │
│  • Hot Module Replacement (HMR) for rapid iteration          │
└───────────────────────────────────────────────────────────────┘

┌───────────────────────────────────────────────────────────────┐
│  Pre-dev step (run from project root, once before dev)        │
│                                                               │
│  go run ./tools/gendata                                       │
│    reads:  nearest.csv         (project root)                 │
│    writes: proto/src/stardata.js  (ES module, STAR_DATA)      │
└───────────────────────────────────────────────────────────────┘
         │
┌────────▼──────────────────────────────────────────────────────┐
│  nearest.csv  (project root)                                  │
└───────────────────────────────────────────────────────────────┘
```

### 5.2 What Is New vs. Existing

- **NEW**: Everything except `nearest.csv` and `requirements.md`. The existing repository contains only `README.md`, `requirements.md`, and `nearest.csv`.
- **`tools/`** lives at the project root (not under `proto/`) so it can be consumed by future product build pipelines.
- **`proto/`** contains everything needed to run the prototype: `index.html`, `src/`, `package.json`, `vite.config.js`. It is self-contained; `npm install` and `npm run dev` are both run from inside `proto/`.

---

## 6. Detailed Design

### 6.1 Component: Vite Development Server

**Purpose**: Serves the application during development. Vite's built-in dev server replaces a hand-written HTTP server for the prototype. It serves `index.html` and all source files directly from the project tree, resolves bare npm imports (`three`, `three/addons/*`) from `node_modules/`, and provides Hot Module Replacement for rapid iteration.

**Satisfies**: FR-002, NFR-002, NFR-004, IR-001
*(FR-001 and FR-003 are superseded — see Section 3 Contradictions.)*

**Interface**:
```
Invocation:  cd proto && npm run dev
Listens on:  http://localhost:5173  (Vite default)
Routes:
  GET /            → proto/index.html
  GET /src/*       → proto/src/* files, transformed on demand
  GET /node_modules/* → resolved by Vite's module graph (not exposed as raw paths)
```

**Behavior**: No configuration beyond `proto/vite.config.js` is required. Vite starts in under 2 seconds on a modern developer laptop (satisfying NFR-002 in the absence of the Go server). The developer runs `go run ./tools/gendata` from the project root once before starting the dev server to ensure `proto/src/stardata.js` exists; Vite will hot-reload it if it is regenerated while the server is running.

**Error handling**:
- If `proto/src/stardata.js` does not exist when Vite starts, the browser will show a module resolution error in the console. Fix: run `go run ./tools/gendata` from the project root and reload.
- If port 5173 is already in use, Vite automatically tries the next available port and prints the actual URL to the terminal.

**Dependencies**: `vite` (devDependency in `package.json`), Node.js runtime.

---

### 6.2 Component: Go Data Generator (`tools/gendata/main.go`)

**Purpose**: Reads `nearest.csv`, converts equatorial coordinates to Cartesian, detects and merges co-located stars, and writes `proto/src/stardata.js` — a static ES-module file with a named `STAR_DATA` export. The generator lives outside `proto/` so it can be reused by any future product build pipeline. It is run from the project root.

**Satisfies**: FR-004, FR-005, FR-006, FR-007

#### Coordinate Conversion

The requirements define the astronomical coordinate system as:
- x → RA = 0h, Dec = 0°
- y → RA = 6h, Dec = 0°
- z → north celestial pole

Three.js uses a y-up right-handed coordinate system and its default `OrbitControls` orbits around the y-axis. To align the scene so that the equatorial plane (astronomical z = 0) appears as the horizontal "floor" of the tank and OrbitControls works naturally, the following remapping is applied **at data generation time** (in Go, before writing the JS constant):

```
js_x =  astro_x
js_y =  astro_z   (north celestial pole → Three.js up)
js_z = -astro_y   (negation preserves right-handedness)
```

The astronomical Cartesian values are:
```
α      = RA_decimal_hours × (π / 12)           [radians]
δ      = Dec_decimal_degrees × (π / 180)        [radians]
astro_x = d × cos(δ) × cos(α)
astro_y = d × cos(δ) × sin(α)
astro_z = d × sin(δ)
```

After remapping, **FR-016's "z=0 plane"** corresponds to **Three.js y=0 plane**. The three projection lines for a star at Three.js position (X, Y, Z) are:
- Foot point on y=0 plane: F = (X, 0, Z)
- Line 1: F → (X, 0, 0)   [toward astronomical x-axis]
- Line 2: F → (0, 0, Z)   [toward astronomical y-axis]
- Line 3: F → (X, Y, Z)   [vertical, toward north celestial pole]

#### Data Types

```go
// rawStar holds one parsed CSV row (before grouping).
type rawStar struct {
    raStr      string  // raw RA field string, e.g. "14 29 43.0"; "-" for Sol
    decStr     string  // raw Dec field string, e.g. "-62 40 46"; "-" for Sol
    distLY     float64 // light-years; 0.0 for Sol
    catalogName string
    commonName  string // empty string if source field is "-" or empty
    isSol       bool
}

// StarGroup is the output unit: one or more co-located stars merged into
// a single scene marker.
type StarGroup struct {
    X, Y, Z     float64  // Three.js scene coordinates (remapped from astro)
    DisplayName string   // text shown on hover (or permanently for Sol)
    IsSol       bool
    HasPlanets  bool     // true if any star in the group has confirmed exoplanets
}
```

#### Interface

```go
// loadPlanets reads the CSV file at csvPath and returns a set of normalized
// star names that have at least one confirmed planet.  Parenthetical aliases
// (e.g. "BL Ceti" from "Luyten 726-8 A (BL Ceti)") and "Gliese "→"GJ "
// normalization are applied to maximise match rate against nearest.csv names.
func loadPlanets(csvPath string) (map[string]bool, error)

// loadAndProcessStars reads the CSV file at csvPath, parses all rows,
// converts coordinates, groups co-located stars, and returns the ordered
// []StarGroup to be written into proto/src/stardata.js.
// Sol (the Sun) is always the first element (index 0).
// hasPlanetsSet is the output of loadPlanets; each star's catalog and common
// names are checked against it to set HasPlanets on the resulting StarGroup.
// Returns a non-nil error if the file cannot be opened, if any required
// field is unparseable, or if fewer than 2 rows are found.
func loadAndProcessStars(csvPath string, hasPlanetsSet map[string]bool) ([]StarGroup, error)
```

#### Behavior (pseudocode)

```
func loadAndProcessStars(csvPath):
  open file; wrap with encoding/csv.NewReader
  set csv.Reader.LazyQuotes = true  // header has quoted field with embedded quotes

  read and discard header row

  rawStars = []
  for each record in csv:
    skip rows where len(record) < 16

    isSol = (record[1] == "SUN")

    catalogName = strings.TrimSpace(record[1])
    commonName  = strings.TrimSpace(record[15])
    if commonName == "-" { commonName = "" }

    if isSol:
      append rawStar{isSol: true, catalogName: "Sol", distLY: 0.0}
      continue

    raStr  = strings.TrimSpace(record[4])
    decStr = strings.TrimSpace(record[5])
    distStr = strings.TrimSpace(record[9])

    if raStr == "-" or decStr == "-" or distStr == "":
      skip row (log warning)
      continue

    distLY, err = strconv.ParseFloat(distStr, 64)
    if err: return nil, fmt.Errorf("row %d: bad distance %q: %w", ...)

    append rawStar{raStr, decStr, distLY, catalogName, commonName, false}

  // Group co-located stars.
  // Key: "<raStr>|<decStr>|<distStr>" — identical strings → identical position.
  type groupKey = struct{ ra, dec, dist string }
  groupMap = map[groupKey]*partialGroup{}
  solGroup  = nil

  for each rawStar s:
    if s.isSol:
      solGroup = &StarGroup{X:0, Y:0, Z:0, DisplayName:"Sol", IsSol:true}
      continue

    astro_x, astro_y, astro_z = convertToAstroCartesian(s.raStr, s.decStr, s.distLY)
    js_x =  astro_x
    js_y =  astro_z
    js_z = -astro_y

    preferred = s.commonName if s.commonName != "" else s.catalogName

    key = groupKey{s.raStr, s.decStr, distStr}
    if key exists in groupMap:
      append preferred to groupMap[key].names
    else:
      groupMap[key] = &partialGroup{X:js_x, Y:js_y, Z:js_z, names:[preferred]}

  // Build result: Sol first, then all groups in insertion order.
  result = []StarGroup{}
  if solGroup != nil: result = append(result, *solGroup)
  for each group in groupMap (insertion order):
    displayName = strings.Join(group.names, " / ")
    result = append(result, StarGroup{group.X, group.Y, group.Z, displayName, false})

  return result, nil
```

**Coordinate parsing sub-procedures**:

```
func parseRA(raStr string) (float64, error):
  // raStr example: "14 29 43.0"
  parts = strings.Fields(raStr)  // ["14", "29", "43.0"]
  if len(parts) != 3: return error
  h = Atoi(parts[0])
  m = Atoi(parts[1])
  s = ParseFloat(parts[2])
  raHours = float64(h) + float64(m)/60.0 + s/3600.0
  if raHours < 0 or raHours >= 24: return error
  return raHours * (math.Pi / 12.0)  // radians

func parseDec(decStr string) (float64, error):
  // decStr example: "-62 40 46" or "+04 41 36"
  parts = strings.Fields(decStr)  // ["-62", "40", "46"]
  if len(parts) != 3: return error
  deg = Atoi(parts[0])  // includes sign (Atoi handles "+" and "-" prefixes)
  min = Atoi(parts[1])
  sec = ParseFloat(parts[2])
  sign = 1.0
  if deg < 0: sign = -1.0
  decDeg = float64(deg) + sign*(float64(min)/60.0 + sec/3600.0)
  if decDeg < -90 or decDeg > 90: return error
  return decDeg * (math.Pi / 180.0)  // radians

func convertToAstroCartesian(raStr, decStr string, distLY float64)
    (astro_x, astro_y, astro_z float64, err error):
  α, err = parseRA(raStr)
  δ, err = parseDec(decStr)
  d = distLY
  astro_x = d * math.Cos(δ) * math.Cos(α)
  astro_y = d * math.Cos(δ) * math.Sin(α)
  astro_z = d * math.Sin(δ)
  return
```

#### Output File Format

The generator writes `proto/src/stardata.js` with the following structure:

```javascript
// AUTO-GENERATED by tools/gendata — do not edit by hand.
// Regenerate with: go run ./tools/gendata

export const STAR_DATA = [
  { x: 0.000000, y: 0.000000, z: 0.000000, displayName: "Sol", isSol: true, hasPlanets: false },
  { x: -1.651234, y: -3.787456, z: 1.408234, displayName: "Proxima Centauri", isSol: false, hasPlanets: true },
  ...
];
```

Floats are formatted with `strconv.FormatFloat(v, 'f', 6, 64)` (6 decimal places). String values are JSON-escaped using `encoding/json` Marshal or equivalent manual escaping of backslash and double-quote. Booleans are `true` / `false`.

**Error handling**:
- Unparseable RA, Dec, or distance on any row → `log.Fatalf` (generator exits with message).
- Row with RA="-" or Dec="-" but not Sol → log a warning and skip the row (do not fatal; allows rows with missing data to be tolerated).
- Distance field empty for non-Sol row → same: warn and skip.
- If zero StarGroups (excluding Sol) are produced → `log.Fatalf`.
- If `proto/src/stardata.js` cannot be written → `log.Fatalf`.

The generator reads `nearest.csv` from the current working directory and writes to `proto/src/stardata.js` (relative to cwd). It must be run from the project root.

**Dependencies**: Go standard library (`encoding/csv`, `encoding/json`, `math`, `strconv`, `strings`, `fmt`, `os`).

---

### 6.3 Component: Vite Entry-Point HTML (`proto/index.html`)

**Purpose**: Static HTML file that serves as Vite's entry point. References `src/main.js` directly; Vite resolves all imports at build time. No Go template rendering occurs.

**Satisfies**: FR-002, FR-004, FR-008, NFR-001, NFR-004

**File content**:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Star Tank</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { background: #000; overflow: hidden; }
    canvas { display: block; }
    .star-label {
      color: #ffffff;
      font-family: 'Courier New', monospace;
      font-size: 13px;
      padding: 2px 6px;
      pointer-events: none;
      white-space: pre;
      text-shadow: 0 0 4px #000;
    }
  </style>
</head>
<body>
  <script type="module" src="/src/main.js"></script>
</body>
</html>
```

Note: There is no inline `<script>` block and no template substitution. Star data reaches the browser entirely through the ES module import chain (`proto/src/main.js` → `proto/src/stardata.js`). The URL path `/src/main.js` in the `<script>` tag is resolved by Vite relative to the `proto/` root.

**Dependencies**: None (static file). Consumed by Vite when `npm run dev` (or `npm run build`) is run from `proto/`.

---

### 6.4 Component: Three.js Application (`proto/src/main.js`)

**Purpose**: Client-side JavaScript that constructs and manages the Three.js scene, handles user input, implements mouseover detection, and renders the star tank visualization.

**Satisfies**: FR-008, FR-009, FR-010, FR-011, FR-012, FR-013, FR-014, FR-015, FR-016, FR-017, FR-018, FR-019, FR-020, FR-021, FR-022, NFR-001, NFR-003, IR-001

**Interface**: None (module with no exports). Imports `STAR_DATA` from `./stardata.js` (i.e., `proto/src/stardata.js`). Appends a `<canvas>` and a CSS2D overlay `<div>` to `document.body`.

#### 6.4.1 Module-Level Imports

```javascript
import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { CSS2DRenderer, CSS2DObject } from 'three/addons/renderers/CSS2DRenderer.js';
import { STAR_DATA } from './stardata.js';
```

Three.js r152+ exports `three/addons/*` via its package.json `exports` map; Vite resolves these automatically from `node_modules/three`. No import-specifier patching is required.

#### 6.4.2 Scene Initialization

```
CAMERA_DISTANCE   = 35       // units = light-years; > max dataset dist (~22.7 LY)
STAR_SIZE         = 4        // dot diameter in CSS pixels (sizeAttenuation: false)
PLANET_RING_SIZE  = 14       // ring diameter in CSS pixels
STAR_COLOR        = 0xffffff // white
DASH_SIZE         = 0.4      // LY, for LineDashedMaterial
GAP_SIZE          = 0.25     // LY, for LineDashedMaterial
LINE_COLOR        = 0x66aaff // light blue for projection lines
RAYCAST_THRESHOLD = 0.5      // LY, Points raycasting tolerance
AXIS_LENGTH       = 25       // LY; extends beyond farthest star (~22.7 LY)
AXIS_COLOR        = 0xffff00 // yellow

scene    = new THREE.Scene()
scene.background = new THREE.Color(0x000000)

camera = new THREE.PerspectiveCamera(
    60,                             // fov degrees
    window.innerWidth / window.innerHeight,
    0.01, 500)
camera.position.set(CAMERA_DISTANCE, CAMERA_DISTANCE, CAMERA_DISTANCE)
camera.lookAt(0, 0, 0)

renderer = new THREE.WebGLRenderer({ antialias: true })
renderer.setPixelRatio(window.devicePixelRatio)
renderer.setSize(window.innerWidth, window.innerHeight)
document.body.appendChild(renderer.domElement)

css2DRenderer = new CSS2DRenderer()
css2DRenderer.setSize(window.innerWidth, window.innerHeight)
css2DRenderer.domElement.style.position = 'absolute'
css2DRenderer.domElement.style.top = '0'
css2DRenderer.domElement.style.pointerEvents = 'none'
document.body.appendChild(css2DRenderer.domElement)

controls = new OrbitControls(camera, renderer.domElement)
controls.target.set(0, 0, 0)
controls.enableDamping = false      // disable so no RAF loop is needed
controls.addEventListener('change', requestRender)
```

#### 6.4.3 Star Marker Construction

Stars are rendered as a single `THREE.Points` object (one GL point per star).  Using `sizeAttenuation: false` makes the points render at a fixed pixel size regardless of camera distance, satisfying FR-021.

```
// Build a single BufferGeometry with one position per star.
positions = Float32Array of length STAR_DATA.length * 3
for i, entry in STAR_DATA:
  positions[i*3]   = entry.x
  positions[i*3+1] = entry.y
  positions[i*3+2] = entry.z

pointsGeometry = new THREE.BufferGeometry()
pointsGeometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))

pointsMaterial = new THREE.PointsMaterial({
  color: STAR_COLOR,
  size: STAR_SIZE,
  sizeAttenuation: false,   // fixed screen-space size; satisfies FR-021
})

starPoints = new THREE.Points(pointsGeometry, pointsMaterial)
scene.add(starPoints)
```

Raycasting uses `raycaster.intersectObject(starPoints)`.  The intersection result's `.index` field is the index into `STAR_DATA` for the nearest hit point.  The raycaster threshold (`raycaster.params.Points.threshold`) is set to `RAYCAST_THRESHOLD` (world units).  Because the points are fixed screen-size, the effective hit area in pixels shrinks slightly as the user zooms out; this is acceptable for the prototype.

Sol's permanent label is attached directly to the scene as a `CSS2DObject` at position (0, 0, 0):

```
solDiv = document.createElement('div')
solDiv.className = 'star-label'
solDiv.textContent = 'Sol'
solLabel = new CSS2DObject(solDiv)
solLabel.position.set(0, 0, 0)
scene.add(solLabel)
```

For each star with `hasPlanets: true`, a CSS2DObject carrying a ring `<div>` is attached to the scene at the star's position (satisfying FR-022).  The ring is a hollow circle sized with CSS in pixels, so it is inherently fixed screen-space:

```
for i, entry in STAR_DATA:
  if not entry.hasPlanets: continue
  ringDiv = document.createElement('div')
  ringDiv.className = 'planet-ring'
  ringObj = new CSS2DObject(ringDiv)
  ringObj.position.set(entry.x, entry.y, entry.z)
  scene.add(ringObj)
```

CSS for the ring (added to `index.html`):

```css
.planet-ring {
  width:  14px;          /* PLANET_RING_SIZE */
  height: 14px;
  border: 1px solid #ffffff;
  border-radius: 50%;
  transform: translate(-50%, -50%);
  pointer-events: none;
}
```

The `transform: translate(-50%, -50%)` centres the ring on the CSS2DRenderer's anchor point, which is the projected screen position of the star.

#### 6.4.4 Axis Line Construction

Three `THREE.Line` objects are added to the scene at startup — one per coordinate axis. They use `THREE.LineBasicMaterial` (solid, not dashed) in `AXIS_COLOR` (yellow). Each line spans from −`AXIS_LENGTH` to +`AXIS_LENGTH` along its respective axis, passing through the origin. All three lines share a single `axisMaterial` instance.

```
axisMaterial = new THREE.LineBasicMaterial({ color: AXIS_COLOR })
axes = [
  [Vector3(-AXIS_LENGTH, 0,           0          ), Vector3(AXIS_LENGTH, 0,           0          )],  // X-axis
  [Vector3(0,            -AXIS_LENGTH, 0          ), Vector3(0,           AXIS_LENGTH, 0          )],  // Y-axis (astro z, north celestial pole direction)
  [Vector3(0,            0,           -AXIS_LENGTH), Vector3(0,           0,           AXIS_LENGTH)],  // Z-axis (astro −y direction)
]
for each [a, b] in axes:
  geo = new THREE.BufferGeometry().setFromPoints([a, b])
  scene.add(new THREE.Line(geo, axisMaterial))
```

The axis lines are permanent scene objects. They are constructed once and never removed or modified. No geometry or material disposal is required because the lines persist for the entire page lifetime. Mouseover and raycasting do not interact with them (they are not the `starPoints` object passed to the raycaster).

**Satisfies**: FR-020

---

#### 6.4.5 Raycasting / Mouseover

Module-level state:
```javascript
let currentHoveredIndex = -1   // STAR_DATA index, -1 = none
let hoverLabel = null          // CSS2DObject or null
let hoverLines = []            // array of up to 3 THREE.Line objects
```

```
raycaster = new THREE.Raycaster()
raycaster.params.Points.threshold = RAYCAST_THRESHOLD
mouse = new THREE.Vector2()

window.addEventListener('mousemove', (event) => {
  // Convert to normalized device coordinates
  mouse.x =  (event.clientX / window.innerWidth)  * 2 - 1
  mouse.y = -(event.clientY / window.innerHeight) * 2 + 1

  raycaster.setFromCamera(mouse, camera)
  intersects = raycaster.intersectObject(starPoints)

  if intersects.length > 0:
    newIndex = intersects[0].index   // index into STAR_DATA
    if newIndex is Sol index: skip   // Sol has no mouseover per FR-009
    if newIndex !== currentHoveredIndex:
      clearHoverElements()
      showHoverElements(newIndex)
      currentHoveredIndex = newIndex
  else:
    if currentHoveredIndex !== -1:
      clearHoverElements()
      currentHoveredIndex = -1

  requestRender()
})
```

#### 6.4.6 `showHoverElements(starIndex)`

```
star = STAR_DATA[starIndex]

// Label
div = document.createElement('div')
div.className = 'star-label'
div.textContent = star.displayName   // may contain " / " for co-located groups
hoverLabel = new CSS2DObject(div)
hoverLabel.position.set(star.x, star.y, star.z)
scene.add(hoverLabel)

// Projection lines
foot = new THREE.Vector3(star.x, 0, star.z)   // projection onto y=0 plane

hoverLines = [
  makeDashedLine(foot, new THREE.Vector3(star.x, 0, 0)),    // Line 1: foot → x-axis
  makeDashedLine(foot, new THREE.Vector3(0,      0, star.z)),// Line 2: foot → z-axis (astro y)
  makeDashedLine(foot, new THREE.Vector3(star.x, star.y, star.z)) // Line 3: foot → star
]
for line of hoverLines: scene.add(line)
```

#### 6.4.7 `clearHoverElements()`

```
if hoverLabel !== null:
  scene.remove(hoverLabel)
  hoverLabel.element.remove()   // remove DOM node
  hoverLabel = null

for line of hoverLines:
  scene.remove(line)
  line.geometry.dispose()
  line.material.dispose()
hoverLines = []
```

#### 6.4.8 `makeDashedLine(pointA, pointB)`

```
function makeDashedLine(a, b):
  geometry = new THREE.BufferGeometry().setFromPoints([a, b])
  material = new THREE.LineDashedMaterial({
    color: LINE_COLOR,
    dashSize: DASH_SIZE,
    gapSize: GAP_SIZE
  })
  line = new THREE.Line(geometry, material)
  line.computeLineDistances()   // required for LineDashedMaterial to render
  return line
```

#### 6.4.9 Demand Rendering (FR-011)

```javascript
let renderPending = false

function requestRender() {
  if (!renderPending) {
    renderPending = true
    requestAnimationFrame(doRender)
  }
}

function doRender() {
  renderPending = false
  renderer.render(scene, camera)
  css2DRenderer.render(scene, camera)
}
```

Event sources that trigger re-render:
1. `controls` `'change'` event (orbit, pan, zoom)
2. `mousemove` handler (after updating hover state)
3. `window` `'resize'` handler
4. Initial call to `requestRender()` at module bottom

```javascript
window.addEventListener('resize', () => {
  camera.aspect = window.innerWidth / window.innerHeight
  camera.updateProjectionMatrix()
  renderer.setSize(window.innerWidth, window.innerHeight)
  css2DRenderer.setSize(window.innerWidth, window.innerHeight)
  requestRender()
})

// Kick off initial render
requestRender()
```

**Error handling**:
- If `STAR_DATA` is an empty array or import fails: `for each entry in STAR_DATA` iterates zero times; the scene renders a black screen with only Sol (or no markers). No additional handling required for a prototype.
- If Three.js bundle is missing: browser console error; black screen. No in-page error UI required (prototype only).

**Dependencies**: `three` (npm), `three/addons/controls/OrbitControls.js`, `three/addons/renderers/CSS2DRenderer.js` (all resolved by Vite from `proto/node_modules/three`); `./stardata.js` (i.e., `proto/src/stardata.js`, generated by `tools/gendata`).

---

### 6.5 Component: npm and Vite Configuration

**Purpose**: Defines project metadata, scripts, and dependencies for the prototype frontend. Both files live in `proto/`; all npm commands are run from that directory.

**Satisfies**: IR-001, NFR-004

#### `package.json`

```json
{
  "name": "startank",
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "three": "^0.168.0"
  },
  "devDependencies": {
    "vite": "*"
  }
}
```

Note: `vite` is listed as `"*"` because it is already installed on the host and the version is not yet known. The developer should replace `"*"` with the actual installed version after running `vite --version`.

The `gendata` step is not included in `package.json` scripts because the generator lives outside `proto/` and must be run from the project root (`go run ./tools/gendata`). There is no automatic pre-dev hook. The developer runs the generator manually before the first `npm run dev`, and again whenever `nearest.csv` changes.

#### `vite.config.js`

```javascript
import { defineConfig } from 'vite'

export default defineConfig({
  build: {
    outDir: 'dist',
  },
})
```

---

## 7. Data Model

### 7.1 CSV Source Fields Used

From `nearest.csv` (0-indexed columns):

| Col | Header | Used | Purpose |
|-----|--------|------|---------|
| 1 | Name | ✓ | Catalog name (`catalogName`) |
| 4 | RA (2000.0) | ✓ | Right ascension, e.g. "14 29 43.0" |
| 5 | DEC (2000.0) | ✓ | Declination, e.g. "-62 40 46" |
| 9 | *(unnamed)* | ✓ | Distance in light-years |
| 15 | common name | ✓ | Common name, "-" or blank if none |
| 0,2,3,6,7,8,10,11,12,13,14 | various | ✗ | Not used |

### 7.2 JavaScript `STAR_DATA` Array

Written to `proto/src/stardata.js` as a named ES module export. Served directly by the Vite dev server; no separate bundle step is required for the prototype. Each element is an object:

| Field | JS type | Description |
|-------|---------|-------------|
| `x` | `number` | Three.js scene X (= astronomical x) |
| `y` | `number` | Three.js scene Y (= astronomical z, north celestial pole direction) |
| `z` | `number` | Three.js scene Z (= negative astronomical y) |
| `displayName` | `string` | Text shown on hover (or "Sol" permanently). For co-located groups: names joined with " / ". Uses common name if available, else catalog name, for each constituent star. |
| `isSol` | `boolean` | `true` only for the Sol entry |

**Ordering**: Sol is always element 0. Remaining entries follow CSV row order (by grouping key first-occurrence order).

**Volume**: ≤109 entries (108 stars + Sol, minus any merged co-located groups). In practice ~107 entries given 2 known co-located pairs.

### 7.3 In-Memory Structures (Go, during generator run only)

`rawStar` and `StarGroup` as defined in Section 6.2. Discarded after `proto/src/stardata.js` is written. No persistent storage. No database.

### 7.4 Data Flow

```
nearest.csv  (project root)
  → (encoding/csv, tools/gendata/main.go) → []rawStar
  → (coordinate conversion + grouping) → []StarGroup
  → (file write) → proto/src/stardata.js  (ES module, STAR_DATA export)
  → (Vite dev server, on-demand transform) → served to browser
  → (JavaScript ES module parse of STAR_DATA) → in-memory JS array
  → (Three.js) → rendered scene
```

---

## 8. Key Design Decisions

| Decision | Alternatives Considered | Choice | Rationale |
|----------|------------------------|--------|-----------|
| Prototype server | (A) Go `net/http` file server on :8080 serving `dist/`; (B) Vite dev server on :5173 serving source directly | **Vite dev server** | For a prototype there is no need to write, compile, and run a separate server. Vite already serves the app with HMR during development. A Go server adds complexity without benefit until a production deployment is required. FR-001 and FR-003 are superseded by this decision. |
| Prototype directory layout | (A) Prototype files at project root alongside tools/; (B) Prototype files under `proto/`, tools at root | **`proto/` subdirectory** | Keeps prototype frontend files isolated from the data generation tool and any future product source. The generator can be reused by product builds without being entangled with prototype npm configuration. |
| Frontend build tool | (A) No build tool — manual Three.js download + sed patching; (B) Vite | **Vite** | Eliminates manual Three.js file management; clean npm imports; HMR dev server for rapid iteration; standard in the ecosystem. |
| Three.js acquisition | Manual curl + sed patch of import specifiers | **npm install three** | One command; correct version pinning; no import specifier patching needed; Vite resolves `three/addons/*` automatically via Three.js package.json exports map. |
| Star data delivery | (A) Go text/template injects data into HTML at request time; (B) Go generator writes static `proto/src/stardata.js` before dev server starts | **Go generator pre-dev** | Vite cannot consume runtime-templated HTML. Pre-generating a static ES module keeps the data path simple, satisfies FR-004, and is simpler than a hybrid server. |
| Where coordinate logic lives | (A) `star.go` + `main.go`; (B) `tools/gendata/main.go` (single tool) | **`tools/gendata/main.go`** | The HTTP server no longer needs to know about stars at all; consolidating all data logic in the generator makes responsibilities clear. |
| Where to perform coordinate conversion | (A) Go at build time (in generator); (B) JavaScript at page load | **Go at build time** | Keeps JS simpler; Go math library handles trig with no dependencies; conversion runs once per build, not per page load. |
| Coordinate system remapping | (A) Use astronomical (x,y,z) directly (z-up in Three.js); (B) Remap astronomical z→jsY, astronomical y→−jsZ | **Remap** | Three.js OrbitControls orbits around the y-axis. Using z-up without remapping causes OrbitControls gimbal-lock-like behavior. Remapping gives natural orbit-the-equatorial-plane navigation. |
| Text label rendering | (A) CSS2DRenderer (HTML divs in 3D); (B) Canvas-texture Sprites; (C) Three.js TextGeometry | **CSS2DRenderer** | Labels remain crisp at any zoom; trivial text updates; standard Three.js add-on; no font loading required. TextGeometry requires a font JSON file. Canvas sprites need texture rebuilding on each label change. |
| Raycasting target | (A) Raycast against individual sphere meshes; (B) Use `THREE.Points` with point threshold | **Individual sphere meshes** | More natural "hit area" around each star; simpler code; 108 meshes is negligible performance cost; `THREE.Points` threshold behavior is less predictable at varying zoom levels. |
| Render-on-demand vs. render loop | (A) Continuous `requestAnimationFrame` loop; (B) Render only on events | **Render only on events** | FR-011 explicitly prohibits continuous animation. Event-driven render satisfies the requirement and reduces CPU/GPU usage when the user is not interacting. |

---

## 9. File and Directory Plan

### File Structure

```
SpaceGame/
├── tools/
│   └── gendata/
│       └── main.go                # Go generator: nearest.csv → proto/src/stardata.js
├── nearest.csv                    # Source star data (existing, unchanged)
├── requirements.md                # Prototype requirements (root-level; informs future game)
└── proto/
    ├── design.md                  # This document — prototype-specific design
    ├── index.html                 # Vite entry-point HTML (plain HTML, no Go template)
    ├── src/
    │   ├── main.js                # Three.js application (ES module)
    │   └── stardata.js            # AUTO-GENERATED — do not hand-edit
    ├── package.json               # npm config: three + vite (run npm from proto/)
    └── vite.config.js             # Minimal Vite config
```

### Files to Create

| Path | Action | Description |
|------|--------|-------------|
| `tools/gendata/main.go` | **CREATE** | Go data generator: CSV parsing, coordinate conversion, co-location grouping, writes `proto/src/stardata.js` |
| `proto/index.html` | **CREATE** | Vite entry-point HTML; references `/src/main.js`; no template logic |
| `proto/src/main.js` | **CREATE** | Three.js application: scene setup, markers, orbit controls, mouseover, demand render |
| `proto/src/stardata.js` | **GENERATED** | Auto-generated by `go run ./tools/gendata` from project root; do not edit by hand |
| `proto/package.json` | **CREATE** | npm config: `three` dependency, `vite` devDependency, dev/build scripts |
| `proto/vite.config.js` | **CREATE** | Minimal Vite config (no `outDir` override needed for dev) |

### Files Already Present (Unchanged)

| Path | Status |
|------|--------|
| `nearest.csv` | Unchanged — read by `tools/gendata` at project root |
| `requirements.md` | Unchanged — lives at project root; describes the prototype vision and informs future game design |
| `README.md` | Unchanged |
| `proto/design.md` | This document — moved from project root into `proto/` because it describes the prototype implementation only |

### Setup and Run

```bash
# Step 1 — Generate star data (run from project root)
go run ./tools/gendata
#   reads:  nearest.csv
#   writes: proto/src/stardata.js

# Step 2 — Install frontend dependencies (run from proto/)
cd proto
npm install

# Step 3 — Start dev server (run from proto/)
npm run dev     # → http://localhost:5173
```

No `go.mod` changes are required beyond the existing module declaration (standard library only). There is no production build step for the prototype; `npm run build` (Vite static bundle) is available if needed later but is not part of the normal prototype workflow.

No `go.mod` changes are required beyond the existing module declaration (standard library only).

---

## 10. Requirement Traceability

| Requirement | Design Section | Files |
|-------------|---------------|-------|
| FR-001 | Superseded — see Section 3 Contradictions | N/A (no Go server in prototype) |
| FR-002 | 6.1 Vite Dev Server | Vite serves `proto/index.html` at `/` |
| FR-003 | Superseded — see Section 3 Contradictions | N/A (no Go server in prototype) |
| FR-004 | 6.2, 6.3 | `tools/gendata/main.go`, `proto/src/stardata.js` |
| FR-005 | 6.2 Coordinate Conversion | `tools/gendata/main.go` |
| FR-006 | 6.2 Grouping | `tools/gendata/main.go` |
| FR-007 | 6.2 Data Types, 7.2 | `tools/gendata/main.go` |
| FR-008 | 6.4.2 Scene Initialization | `proto/src/main.js` |
| FR-009 | 6.4.3 Star Marker Construction | `proto/src/main.js` |
| FR-010 | 6.4.3 | `proto/src/main.js` |
| FR-011 | 6.4.9 Demand Rendering | `proto/src/main.js` |
| FR-012 | 6.4.2 (OrbitControls left-drag) | `proto/src/main.js` |
| FR-013 | 6.4.2 (OrbitControls scroll) | `proto/src/main.js` |
| FR-014 | 6.4.2 (OrbitControls right-drag) | `proto/src/main.js` |
| FR-015 | 6.4.6 showHoverElements | `proto/src/main.js` |
| FR-016 | 6.4.6, 6.4.8 | `proto/src/main.js` |
| FR-017 | 6.4.7 clearHoverElements | `proto/src/main.js` |
| FR-018 | 6.4.5 Raycasting (one-at-a-time guard) | `proto/src/main.js` |
| FR-019 | 6.4.2 Camera position | `proto/src/main.js` |
| FR-020 | 6.4.4 Axis Line Construction | `proto/src/main.js` |
| FR-021 | 6.4.3 Star Marker Construction (PointsMaterial sizeAttenuation: false) | `proto/src/main.js` |
| FR-022 | 6.4.3 Star Marker Construction (CSS2D planet ring) | `proto/src/main.js`, `proto/index.html` |
| NFR-001 | 6.4 (ES modules, standard APIs) | `proto/src/main.js`, `proto/index.html` |
| NFR-002 | 6.1 (Vite dev server startup) | `proto/package.json` (`npm run dev`) |
| NFR-003 | 6.4.5 (single-frame update; no async in hover path) | `proto/src/main.js` |
| NFR-004 | 6.5 Three.js served locally by Vite from node_modules | `proto/package.json`, `proto/node_modules/` |
| IR-001 | 6.4.1 imports, 6.5 | `proto/src/main.js`, `proto/package.json` |

---

## 11. Testing Strategy

### Unit Tests (Go)

**`tools/gendata/main.go` — `parseRA`**:
- RA = "00 00 00.0" → 0.0 radians
- RA = "06 00 00.0" → π/2 radians
- RA = "12 00 00.0" → π radians
- RA = "18 00 00.0" → 3π/2 radians
- RA = "23 59 59.9" → just below 2π
- RA = "-" → error

**`tools/gendata/main.go` — `parseDec`**:
- Dec = "+00 00 00" → 0.0 radians
- Dec = "+90 00 00" → π/2 radians
- Dec = "-90 00 00" → −π/2 radians
- Dec = "-62 40 46" → -(62 + 40/60 + 46/3600) × π/180
- Dec = "-" → error

**`tools/gendata/main.go` — `convertToAstroCartesian`**:
- Known value: GJ 551 (Proxima Centauri): RA="14 29 43.0", Dec="-62 40 46", dist=4.24217987904012 LY. Verify computed distance from origin equals dist LY within 1e-6 LY (i.e., sqrt(x²+y²+z²) ≈ 4.242).
- Sol (special case): position = (0,0,0).

**`tools/gendata/main.go` — `loadAndProcessStars`**:
- With the actual `nearest.csv`: verify output length is between 100 and 109.
- Verify Sol is element 0, `IsSol=true`, position (0,0,0).
- Verify Sirius (GJ 244 A) and Sirius B (GJ 244 B) are merged into a single entry whose `DisplayName` contains both "Sirius" and "Sirius B".
- Verify BL Ceti (GJ 65 A) and UV Ceti (GJ 65 B) are merged similarly.
- Verify GJ 559 A and GJ 559 B are **not** merged (they have distinct RA/Dec in the CSV).
- Verify Barnard's Star has `DisplayName` = "Barnard's Star".
- Verify a star with common name "-" (e.g., GJ 1061) has `DisplayName` = "GJ 1061".

### Manual / Visual Tests

| Test | Expected result | Satisfies |
|------|----------------|-----------|
| Load `http://localhost:5173` | Black background, ~107 white dots, "Sol" label at center, three yellow axis lines crossing at origin | FR-008, FR-009, FR-010, FR-020 |
| Page loads with no internet connection | Scene renders fully (no CDN errors in browser console) | NFR-004 |
| Click-drag on the scene | Scene orbits around origin | FR-012 |
| Scroll wheel | Scene zooms in and out | FR-013 |
| Right-click drag | Scene pans | FR-014 |
| Hover over a star dot | Star name appears; 3 dotted lines appear from foot of star to x-axis, to y-axis (Three.js z-axis), and from foot to star | FR-015, FR-016 |
| Move mouse off a star | Label and lines disappear immediately | FR-017 |
| Hover over one star then immediately another | Previous label/lines gone before new ones appear; never two sets visible | FR-018 |
| Hover over a star with no common name (e.g., GJ 1061) | Catalog name "GJ 1061" displayed | FR-015 |
| Hover over a Sirius-family star (if visible; single merged dot) | "Sirius / Sirius B" displayed | FR-015, FR-006 |
| Initial page load, no interaction | Camera is positioned so all stars are visible; scene is seen at an oblique angle | FR-019 |
| Dev server start time | Time from `npm run dev` to "ready" message in terminal is < 2 seconds | NFR-002 |
| No spontaneous motion | After loading, scene is static unless user input occurs | FR-011 |
| Hover over a star | Three yellow axis lines remain continuously visible; they are not affected by the hover state | FR-020 |
| Orbit, zoom, or pan | Three yellow axis lines remain continuously visible throughout all camera movements | FR-020 |

### Edge Cases

- Star at exactly RA=0, Dec=0 (if any): should appear on the positive x-axis.
- Star at Dec = +90° (north celestial pole): should appear on the positive Three.js y-axis.
- Stars with very small z (near-equatorial): projection lines should still render; Line 3 will be very short.
- Co-located stars with one having a common name and one not: both names must appear (one common, one catalog).
- Hover detection at high zoom (star fills most of the screen): hover should still trigger on the sphere.
- Hover detection at maximum zoom-out (stars are tiny): threshold `RAYCAST_THRESHOLD = 0.3` provides a reasonable pick region.

---

## 12. Open Questions

**OQ-Three.js-Version**: The design specifies Three.js `^0.168.0` as the npm version range. The developer should pin an exact version in `package.json` after running `npm install` and confirming compatibility. Versions r152+ are required for the `three/addons/*` exports map to work correctly with Vite. **Resolved at developer discretion; no implementation blocker.**

All other questions from `requirements.md` are resolved:
- **OQ-001**: Resolved in requirements.md as FR-019.
- **OQ-002**: Deferred by design (game genre question); not relevant to this prototype.
