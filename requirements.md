# Requirements: Star Tank Prototype

## 1. Overview

Star Tank is a single-page web application that renders an interactive 3D visualization of the 108 nearest star systems to Earth, in the style of the "star tank" display found on the bridge of a starship in science fiction. The primary user is a solo developer evaluating whether the visual result is suitable as the foundation for a game. The application is served by a minimal Go web server. There is no backend logic beyond file serving; all rendering is performed client-side using Three.js.

---

## 2. Users and Roles

| Role | Description |
|------|-------------|
| **Developer / Evaluator** | Single user running the server locally. Views the star map, navigates the scene, and assesses visual suitability. No authentication or multi-user support is required. |

---

## 3. Functional Requirements

### 3.1 Web Server

- **FR-001:** The system shall include a Go HTTP server that listens on port 8080.
- **FR-002:** The server shall serve a single HTML page (with embedded or bundled CSS and JavaScript) at the root path `/`.
- **FR-003:** The server shall require no external dependencies beyond the Go standard library.

### 3.2 Star Data

- **FR-004:** The star dataset (108 entries derived from `nearest.csv`) shall be embedded directly in the JavaScript source; no runtime data file fetch is required.
- **FR-005:** The system shall convert each star's position from equatorial spherical coordinates (right ascension in hours/minutes/seconds, declination in degrees/arcminutes/arcseconds, distance in light-years) to Cartesian (x, y, z) coordinates using the standard equatorial-to-Cartesian conversion, with Earth/Sol at the origin (0, 0, 0).
- **FR-006:** Stars that share identical coordinates (co-located pairs or groups, e.g. Alpha Centauri A and B) shall be treated as a single marker. The names of all co-located stars shall be associated with that single marker.
- **FR-007:** For each star the system shall store: Cartesian position, catalog name (GJ designation or equivalent), and common name if one exists in the source data.

### 3.3 Scene Rendering

- **FR-008:** The scene shall render on a black background.
- **FR-009:** Sol (Earth's sun) shall be rendered at position (0, 0, 0) and shall be permanently labeled with the text "Sol". No mouseover interaction is required for Sol.
- **FR-010:** Each non-Sol star (or co-located group) shall be represented by a marker. All markers shall be visually identical (same size, same color, same shape).
- **FR-011:** The scene shall not include any continuous animation. All motion in the scene shall be driven solely by user input (camera controls) or mouseover events.

### 3.4 Camera and Navigation

- **FR-019:** The default (initial) camera position shall be placed outside the full extent of the star dataset, along a diagonal that produces an upper-left to lower-right perspective when the page first loads. Specifically, the camera shall be positioned at a point with positive x, positive y, and positive z values (e.g. approximately equal magnitudes on all three axes) at a distance from the origin greater than the farthest star in the dataset, looking toward the origin (0, 0, 0). This ensures all stars are visible in the initial view and the z=0 reference plane is seen at an oblique angle rather than face-on or edge-on.

- **FR-012:** The user shall be able to orbit the scene by clicking and dragging.
- **FR-013:** The user shall be able to zoom in and out using the scroll wheel.
- **FR-014:** The user shall be able to pan the scene using right-click drag (standard Three.js OrbitControls behavior).

### 3.5 Mouseover Interaction

- **FR-015:** When the user moves the mouse pointer over a star marker, the system shall display the star's name adjacent to the marker. If a common name exists for that star (or group), the common name shall be displayed; otherwise the catalog name shall be displayed. For co-located groups with multiple names, all names shall be displayed.
- **FR-016:** When the user moves the mouse pointer over a star marker, the system shall render three dotted lines:
  - A dotted line from the star's projected position on the z=0 plane — (x, y, 0) — to the point (x, 0, 0) on the x-axis.
  - A dotted line from the star's projected position on the z=0 plane — (x, y, 0) — to the point (0, y, 0) on the y-axis.
  - A dotted line from the star's projected position on the z=0 plane — (x, y, 0) — vertically to the star's actual position (x, y, z).
- **FR-017:** When the user moves the mouse pointer off a star marker, the name label and all dotted lines associated with that marker shall be removed from the scene immediately.
- **FR-018:** No more than one star's label and projection lines shall be visible at a time.

---

## 4. Non-Functional Requirements

- **NFR-001:** The application shall run entirely in a modern desktop web browser (Chrome, Firefox, or Safari, current versions) with no browser plugins required.
- **NFR-002:** The Go server shall start and be ready to serve requests within 2 seconds of launch on a typical developer laptop.
- **NFR-003:** Mouseover detection and label/line rendering shall appear instantaneous to the user (no perceptible lag on a modern laptop).
- **NFR-004:** The application shall not require an internet connection at runtime; all assets (including the Three.js library) shall be either embedded or served locally.

---

## 5. Data Requirements

### 5.1 Source File
- File: `/Users/Jeff/nearest.csv`
- 108 star records (plus Sol)
- Fields used: RA (HH MM SS.s), Dec (±DD MM SS), Distance (light-years), catalog name, common name

### 5.2 Derived Data (embedded in JS)
Each record stored in JavaScript shall contain:

| Field | Type | Notes |
|-------|------|-------|
| `x, y, z` | float | Light-years, computed from RA/Dec/distance |
| `catalogName` | string | GJ designation or equivalent |
| `commonName` | string or null | Human-readable name, if present in CSV |
| `displayName` | string | Common name if available, else catalog name; for co-located groups, all applicable names |

### 5.3 Volume
- ~108 star records. No database or persistence layer is needed.

---

## 6. Integration Requirements

- **IR-001:** The client page shall use the **Three.js** library for 3D rendering and OrbitControls for camera navigation. The library shall be served locally (bundled or copied into the project) so no CDN access is required at runtime.
- No other external integrations are required.

---

## 7. Constraints and Assumptions

### Constraints
- Server language: Go, standard library only (no third-party Go packages).
- Frontend rendering: Three.js (no other 3D framework).
- Server port: 8080 (fixed).
- This is a **prototype only** — no production hardening, authentication, or scalability measures are required.

### Assumptions
- The developer runs the server locally; there is no need for HTTPS or CORS handling.
- The CSV file is a one-time data source; the data is baked into the JS at development time and does not need to be re-read at runtime.
- "No animation" means no time-driven scene updates; OrbitControls-driven camera movement and mouseover-driven element appearance/disappearance are not considered animation for this purpose.
- The coordinate system orientation (x toward RA=0h Dec=0°, z toward north celestial pole) is acceptable for prototype evaluation purposes.

---

## 8. MVP Scope

All requirements listed above constitute the MVP. The full set is:

`FR-001` through `FR-018`, `NFR-001` through `NFR-004`, `IR-001`

The prototype is intentionally minimal. The following are explicitly **out of scope** for this phase:
- Varying star appearance by spectral type, magnitude, or mass
- Any legend, grid, axis labels, or scale indicators
- Clickable stars or any interaction beyond hover
- Mobile or touch support
- Multiple views or UI controls (no buttons, sliders, etc.)

---

## 9. Open Questions

- **OQ-001:** ~~Resolved.~~ See FR-019.
- **OQ-002:** If the prototype evaluation result is "yes" — what game genre/mechanic is envisioned? (Deferred by design; captured here for continuity.)

---

## 10. Glossary

| Term | Definition |
|------|------------|
| **Tank** | In science fiction, a three-dimensional holographic or physical display showing nearby space, ship positions, and star systems. |
| **RA (Right Ascension)** | The celestial equivalent of longitude, measured in hours, minutes, and seconds eastward along the celestial equator. |
| **Dec (Declination)** | The celestial equivalent of latitude, measured in degrees north (+) or south (−) of the celestial equator. |
| **Equatorial-to-Cartesian conversion** | The standard transformation from (RA, Dec, distance) spherical coordinates to (x, y, z) Cartesian coordinates, with x pointing toward RA=0h/Dec=0°, y toward RA=6h/Dec=0°, and z toward the north celestial pole. |
| **Co-located stars** | Two or more stars (typically a binary pair) whose RA, Dec, and distance are identical in the source data, resulting in the same computed Cartesian position. |
| **OrbitControls** | A Three.js add-on that allows the user to orbit, zoom, and pan a 3D scene using mouse input. |
| **Sol** | The proper name for Earth's sun, used in this application as the permanent label for the star at the origin. |
| **Light-year** | The distance light travels in one year, approximately 9.461 × 10¹² km. Used as the unit of distance throughout this application. |
| **Prototype** | A minimal working implementation built solely to evaluate visual suitability, not intended for production use. |
