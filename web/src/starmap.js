import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { CSS2DRenderer, CSS2DObject } from 'three/addons/renderers/CSS2DRenderer.js'
import { STATUS_COLORS, SOL_COLOR } from './constants.js'

const CAMERA_DISTANCE   = 35
const STAR_SIZE         = 4
const DASH_SIZE         = 0.4
const GAP_SIZE          = 0.25
const LINE_COLOR        = 0x66aaff
const RAYCAST_THRESHOLD = 0.5
const AXIS_LENGTH       = 25
const AXIS_COLOR        = 0xffff00

const STATUS_NAMES = {
    human:       'Human-held',
    alien:       'Alien-held',
    contested:   'Contested',
    unknown:     'Unknown',
    uninhabited: 'Uninhabited',
}

export class StarMap {
    constructor(state) {
        this.state = state
        this.ui = null  // set via setUIController() after construction

        this.stars = []
        this.solIndex = 0

        this.scene = null
        this.camera = null
        this.renderer = null
        this.css2DRenderer = null
        this.controls = null

        this.pointsGeometry = null
        this.starPoints = null
        this.planetRingDivs = []  // indexed by star array index

        this.currentHoveredIndex = -1
        this.hoverPopup = null
        this.hoverLines = []

        this.highlightedIndex = -1

        this.renderPending = false

        // Selection mode state (A-5, FR-034, FR-038)
        this.selectionMode = null          // null | 'fleet' | 'construct'
        this.selectionBannerEl = null
        this.destinationFleetId = null
        this.destinationOriginId = null

        // Re-color stars whenever known state changes
        state.on('systemUpdated', () => this.updateStarColors())
        state.on('stateLoaded',   () => this.updateStarColors())
    }

    // Called from main.js after UIController is constructed.
    setUIController(ui) {
        this.ui = ui
    }

    init(stars) {
        this.stars = stars
        this.solIndex = stars.findIndex(s => s.isSol)
        if (this.solIndex < 0) this.solIndex = 0

        this._buildScene()
        this._buildStarPoints()
        this._buildSolLabel()
        this._buildPlanetRings()
        this._buildAxes()
        this._attachEventListeners()

        this.requestRender()
    }

    // -------------------------------------------------------------------------
    // Scene construction
    // -------------------------------------------------------------------------

    _buildScene() {
        this.scene = new THREE.Scene()
        this.scene.background = new THREE.Color(0x000000)

        this.camera = new THREE.PerspectiveCamera(
            60, window.innerWidth / window.innerHeight, 0.01, 500
        )
        this.camera.position.set(CAMERA_DISTANCE, CAMERA_DISTANCE, CAMERA_DISTANCE)
        this.camera.lookAt(0, 0, 0)

        this.renderer = new THREE.WebGLRenderer({ antialias: true })
        this.renderer.setPixelRatio(window.devicePixelRatio)
        this.renderer.setSize(window.innerWidth, window.innerHeight)
        document.body.appendChild(this.renderer.domElement)

        this.css2DRenderer = new CSS2DRenderer()
        this.css2DRenderer.setSize(window.innerWidth, window.innerHeight)
        this.css2DRenderer.domElement.style.position = 'absolute'
        this.css2DRenderer.domElement.style.top = '0'
        this.css2DRenderer.domElement.style.pointerEvents = 'none'
        document.body.appendChild(this.css2DRenderer.domElement)

        this.controls = new OrbitControls(this.camera, this.renderer.domElement)
        this.controls.target.set(0, 0, 0)
        this.controls.enableDamping = false
        this.controls.addEventListener('change', () => this.requestRender())
    }

    _buildStarPoints() {
        const n = this.stars.length
        const positions = new Float32Array(n * 3)
        const colors    = new Float32Array(n * 3)

        for (let i = 0; i < n; i++) {
            const s = this.stars[i]
            positions[i * 3]     = s.x
            positions[i * 3 + 1] = s.y
            positions[i * 3 + 2] = s.z
        }

        this.pointsGeometry = new THREE.BufferGeometry()
        this.pointsGeometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
        this.pointsGeometry.setAttribute('color',    new THREE.BufferAttribute(colors, 3))

        const material = new THREE.PointsMaterial({
            size: STAR_SIZE,
            sizeAttenuation: false,
            vertexColors: true,
        })

        this.starPoints = new THREE.Points(this.pointsGeometry, material)
        this.scene.add(this.starPoints)

        // Fill initial colors (will mostly show 'unknown' until state arrives)
        this.updateStarColors()
    }

    _buildSolLabel() {
        const div = document.createElement('div')
        div.className = 'star-label'
        div.textContent = 'Sol'
        const label = new CSS2DObject(div)
        label.position.set(0, 0, 0)
        this.scene.add(label)
    }

    _buildPlanetRings() {
        this.planetRingDivs = new Array(this.stars.length).fill(null)
        for (let i = 0; i < this.stars.length; i++) {
            if (!this.stars[i].hasPlanets) continue
            const div = document.createElement('div')
            div.className = 'planet-ring'
            this.planetRingDivs[i] = div
            const ring = new CSS2DObject(div)
            ring.position.set(this.stars[i].x, this.stars[i].y, this.stars[i].z)
            this.scene.add(ring)
        }
    }

    _buildAxes() {
        const mat = new THREE.LineBasicMaterial({ color: AXIS_COLOR })
        const pairs = [
            [new THREE.Vector3(-AXIS_LENGTH, 0, 0), new THREE.Vector3(AXIS_LENGTH, 0, 0)],
            [new THREE.Vector3(0, -AXIS_LENGTH, 0), new THREE.Vector3(0, AXIS_LENGTH, 0)],
            [new THREE.Vector3(0, 0, -AXIS_LENGTH), new THREE.Vector3(0, 0, AXIS_LENGTH)],
        ]
        for (const [a, b] of pairs) {
            const geo = new THREE.BufferGeometry().setFromPoints([a, b])
            this.scene.add(new THREE.Line(geo, mat))
        }
    }

    // -------------------------------------------------------------------------
    // Event listeners
    // -------------------------------------------------------------------------

    _attachEventListeners() {
        const raycaster = new THREE.Raycaster()
        raycaster.params.Points = { threshold: RAYCAST_THRESHOLD }
        const mouse = new THREE.Vector2()

        // Mousemove: hover popup + axis lines (FR-021, FR-022)
        window.addEventListener('mousemove', (e) => {
            mouse.x =  (e.clientX / window.innerWidth)  * 2 - 1
            mouse.y = -(e.clientY / window.innerHeight) * 2 + 1

            raycaster.setFromCamera(mouse, this.camera)
            const hits = raycaster.intersectObject(this.starPoints)

            if (hits.length > 0) {
                const idx = hits[0].index
                // Sol has no hover per FR-029 (right-click only)
                if (idx !== this.solIndex && idx !== this.currentHoveredIndex) {
                    this._clearHoverElements()
                    this._showHoverElements(idx)
                    this.currentHoveredIndex = idx
                }
            } else if (this.currentHoveredIndex !== -1) {
                this._clearHoverElements()
                this.currentHoveredIndex = -1
            }

            this.requestRender()
        })

        // Right-click: context menu (FR-029)
        this.renderer.domElement.addEventListener('contextmenu', (e) => {
            e.preventDefault()
            const star = this._getStarAtEvent(e)
            if (star && this.ui) this.ui.showContextMenu(e.clientX, e.clientY, star)
        })

        // Left-click: consumed only in selection mode (A-5, FR-034, FR-038)
        this.renderer.domElement.addEventListener('click', (e) => {
            if (!this.selectionMode) return
            const star = this._getStarAtEvent(e)
            if (star && this.ui) {
                const mode = this.selectionMode
                this.exitSelectionMode()
                if (mode === 'fleet') {
                    this.ui.confirmFleetDestination(star)
                } else {
                    this.ui.showConstructDialog(star)
                }
            }
        })

        window.addEventListener('resize', () => {
            this.camera.aspect = window.innerWidth / window.innerHeight
            this.camera.updateProjectionMatrix()
            this.renderer.setSize(window.innerWidth, window.innerHeight)
            this.css2DRenderer.setSize(window.innerWidth, window.innerHeight)
            this.requestRender()
        })
    }

    _getStarAtEvent(e) {
        const raycaster = new THREE.Raycaster()
        raycaster.params.Points = { threshold: RAYCAST_THRESHOLD }
        const mouse = new THREE.Vector2(
            (e.clientX  / window.innerWidth)  * 2 - 1,
            -(e.clientY / window.innerHeight) * 2 + 1,
        )
        raycaster.setFromCamera(mouse, this.camera)
        const hits = raycaster.intersectObject(this.starPoints)
        if (hits.length === 0) return null
        const idx = hits[0].index
        return { ...this.stars[idx], index: idx }
    }

    // -------------------------------------------------------------------------
    // Hover elements
    // -------------------------------------------------------------------------

    _showHoverElements(idx) {
        const star = this.stars[idx]
        const sys  = this.state.getKnownState(star.id)

        // Extended popup (FR-021)
        const popup = document.createElement('div')
        popup.className = 'star-popup'

        const nameDiv = document.createElement('div')
        nameDiv.className = 'star-name'
        nameDiv.textContent = star.displayName
        popup.appendChild(nameDiv)

        if (sys) {
            const statusName = STATUS_NAMES[sys.knownStatus] ?? sys.knownStatus
            const statusDiv = document.createElement('div')
            statusDiv.className = 'star-status'
            statusDiv.textContent =
                `Status: ${statusName} (as of year ${sys.knownAsOfYear.toFixed(1)})`
            popup.appendChild(statusDiv)

            if (sys.knownEconLevel > 0) {
                const econDiv = document.createElement('div')
                econDiv.className = 'star-econ'
                econDiv.textContent = `Economy: Level ${sys.knownEconLevel}`
                popup.appendChild(econDiv)
            }

            const forcesText = this._formatForces(sys)
            if (forcesText) {
                const forcesDiv = document.createElement('div')
                forcesDiv.className = 'star-forces'
                forcesDiv.textContent = `Forces: ${forcesText}`
                popup.appendChild(forcesDiv)
            }
        }

        const popupAnchor = document.createElement('div')
        popupAnchor.style.cssText = 'width:0;height:0;overflow:visible'
        popupAnchor.appendChild(popup)

        this.hoverPopup = new CSS2DObject(popupAnchor)
        this.hoverPopup.position.set(star.x, star.y, star.z)
        this.scene.add(this.hoverPopup)

        // Axis projection dashed lines (FR-022)
        const foot = new THREE.Vector3(star.x, 0, star.z)
        this.hoverLines = [
            this._makeDashedLine(foot, new THREE.Vector3(star.x, 0, 0)),
            this._makeDashedLine(foot, new THREE.Vector3(0, 0, star.z)),
            this._makeDashedLine(foot, new THREE.Vector3(star.x, star.y, star.z)),
        ]
        for (const line of this.hoverLines) this.scene.add(line)
    }

    _clearHoverElements() {
        if (this.hoverPopup !== null) {
            this.scene.remove(this.hoverPopup)
            this.hoverPopup.element.remove()
            this.hoverPopup = null
        }
        for (const line of this.hoverLines) {
            this.scene.remove(line)
            line.geometry.dispose()
            line.material.dispose()
        }
        this.hoverLines = []
    }

    _makeDashedLine(a, b) {
        const geo  = new THREE.BufferGeometry().setFromPoints([a, b])
        const mat  = new THREE.LineDashedMaterial({
            color: LINE_COLOR, dashSize: DASH_SIZE, gapSize: GAP_SIZE,
        })
        const line = new THREE.Line(geo, mat)
        line.computeLineDistances()
        return line
    }

    _formatForces(sys) {
        const parts = []
        if (sys.knownLocalUnits) {
            for (const [type, count] of Object.entries(sys.knownLocalUnits)) {
                if (count > 0) parts.push(`${count} ${type.replace(/_/g, ' ')}`)
            }
        }
        if (sys.knownFleets) {
            for (const fleet of sys.knownFleets) {
                if (!fleet.inTransit) parts.push(fleet.name)
            }
        }
        return parts.join(', ')
    }

    // -------------------------------------------------------------------------
    // Star colors (FR-020)
    // -------------------------------------------------------------------------

    updateStarColors() {
        if (!this.pointsGeometry) return
        const colors = this.pointsGeometry.attributes.color.array

        for (let i = 0; i < this.stars.length; i++) {
            const star = this.stars[i]
            const hex  = star.isSol
                ? SOL_COLOR
                : (STATUS_COLORS[this.state.getKnownStatus(star.id)] ?? STATUS_COLORS.unknown)
            const c = new THREE.Color(hex)
            colors[i * 3]     = c.r
            colors[i * 3 + 1] = c.g
            colors[i * 3 + 2] = c.b

            // Planet ring border tracks status color (Section 8 design decision)
            if (this.planetRingDivs[i]) {
                this.planetRingDivs[i].style.borderColor = `#${c.getHexString()}`
            }
        }
        this.pointsGeometry.attributes.color.needsUpdate = true

        // Re-apply highlight on top if one is active
        if (this.highlightedIndex >= 0) this._applyHighlight(this.highlightedIndex)

        this.requestRender()
    }

    // -------------------------------------------------------------------------
    // Sidebar highlight (FR-027)
    // -------------------------------------------------------------------------

    highlightStar(systemId) {
        const idx = this.stars.findIndex(s => s.id === systemId)
        if (idx < 0) return
        if (this.highlightedIndex >= 0) this._restoreStarColor(this.highlightedIndex)
        this.highlightedIndex = idx
        this._applyHighlight(idx)
        this.requestRender()
    }

    unhighlightStar() {
        if (this.highlightedIndex < 0) return
        this._restoreStarColor(this.highlightedIndex)
        this.highlightedIndex = -1
        this.requestRender()
    }

    _applyHighlight(idx) {
        const colors = this.pointsGeometry.attributes.color.array
        colors[idx * 3]     = 1.0
        colors[idx * 3 + 1] = 1.0
        colors[idx * 3 + 2] = 1.0
        this.pointsGeometry.attributes.color.needsUpdate = true
    }

    _restoreStarColor(idx) {
        const star = this.stars[idx]
        const hex  = star.isSol
            ? SOL_COLOR
            : (STATUS_COLORS[this.state.getKnownStatus(star.id)] ?? STATUS_COLORS.unknown)
        const c = new THREE.Color(hex)
        const colors = this.pointsGeometry.attributes.color.array
        colors[idx * 3]     = c.r
        colors[idx * 3 + 1] = c.g
        colors[idx * 3 + 2] = c.b
        this.pointsGeometry.attributes.color.needsUpdate = true
    }

    // -------------------------------------------------------------------------
    // Selection mode (A-5, FR-034, FR-038)
    // -------------------------------------------------------------------------

    enterSelectionMode(mode, fleetId = null, originSystemId = null) {
        this.selectionMode       = mode
        this.destinationFleetId  = fleetId
        this.destinationOriginId = originSystemId
        this.renderer.domElement.style.cursor = 'crosshair'

        if (!this.selectionBannerEl) {
            this.selectionBannerEl = document.getElementById('selection-banner')
        }
        if (this.selectionBannerEl) {
            this.selectionBannerEl.textContent = mode === 'construct'
                ? 'Select the system where this order will execute \u2014 Esc to cancel'
                : 'Select a destination system \u2014 Esc to cancel'
            this.selectionBannerEl.style.display = 'block'
        }
    }

    exitSelectionMode() {
        this.selectionMode       = null
        this.destinationFleetId  = null
        this.destinationOriginId = null
        this.renderer.domElement.style.cursor = ''

        if (this.selectionBannerEl) {
            this.selectionBannerEl.style.display = 'none'
        }
    }

    // -------------------------------------------------------------------------
    // Demand rendering (retained from prototype)
    // -------------------------------------------------------------------------

    requestRender() {
        if (!this.renderPending) {
            this.renderPending = true
            requestAnimationFrame(() => this._doRender())
        }
    }

    _doRender() {
        this.renderPending = false
        this.renderer.render(this.scene, this.camera)
        this.css2DRenderer.render(this.scene, this.camera)
    }
}
