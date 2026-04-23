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
const ARROW_COLOR       = 0xFFA500
const ARROW_HEAD_LENGTH = 0.8
const ARROW_HEAD_WIDTH  = 0.4
const ARROW_HIT_RADIUS  = 0.3

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
        this.sidebarHoverActive = false

        this.highlightedIndex = -1

        // Amber arrows for pending commands / in-transit fleets
        this.arrows = new Map()   // key: arrow id → { arrowHelper, hitMesh, description, endYear }
        this.arrowHitMeshes = []  // flat list for raycasting
        this.currentHoveredArrowId = null

        this.renderPending = false

        // Selection mode state (A-5, FR-034, FR-038)
        this.selectionMode = null          // null | 'fleet' | 'construct'
        this.selectionBannerEl = null
        this.destinationFleetId = null
        this.destinationOriginId = null

        // Re-color stars whenever known state changes
        state.on('systemUpdated', () => this.updateStarColors())
        state.on('stateLoaded',   () => this.updateStarColors())

        // Rebuild arrows whenever the pending-command set changes
        state.on('pendingCommandsChanged',  () => this._rebuildArrows())
        state.on('inTransitFleetsChanged',  () => this._rebuildArrows())
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
            if (!this.sidebarHoverActive) {
                mouse.x =  (e.clientX / window.innerWidth)  * 2 - 1
                mouse.y = -(e.clientY / window.innerHeight) * 2 + 1

                raycaster.setFromCamera(mouse, this.camera)

                // Arrow hit takes priority over star hover
                let arrowHit = null
                if (this.arrowHitMeshes.length > 0) {
                    const aHits = raycaster.intersectObjects(this.arrowHitMeshes)
                    if (aHits.length > 0) arrowHit = aHits[0].object.userData.arrowId
                }

                if (arrowHit !== null) {
                    if (arrowHit !== this.currentHoveredArrowId) {
                        this._clearHoverElements()
                        this.currentHoveredIndex = -1
                        this._showArrowHover(arrowHit)
                        this.currentHoveredArrowId = arrowHit
                    }
                } else {
                    if (this.currentHoveredArrowId !== null) {
                        this._clearArrowHover()
                        this.currentHoveredArrowId = null
                    }
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
                }
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
                const mode     = this.selectionMode
                const fleetId  = this.destinationFleetId
                const originId = this.destinationOriginId
                this.exitSelectionMode()
                if (mode === 'fleet') {
                    this.ui.confirmFleetDestination(star, fleetId, originId)
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

        this._writeMessageLog(star, sys)

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
        for (const line of this.hoverLines) {
            this.scene.remove(line)
            line.geometry.dispose()
            line.material.dispose()
        }
        this.hoverLines = []
        const log = document.getElementById('message-log')
        if (log) log.innerHTML = ''
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

    _writeMessageLog(star, sys) {
        const log = document.getElementById('message-log')
        if (!log) return
        log.innerHTML = ''

        // Line 1: name · status · econ level
        const statusName = sys ? (STATUS_NAMES[sys.knownStatus] ?? sys.knownStatus) : 'Unknown'
        const asOf       = sys ? ` (yr ${sys.knownAsOfYear.toFixed(1)})` : ''
        const econ       = sys && sys.knownEconLevel > 0 ? `Economy: Level ${sys.knownEconLevel}` : ''

        const line1 = document.createElement('div')
        line1.className = 'msg-line'
        line1.innerHTML =
            `<span class="star-name">${star.displayName}</span>` +
            `\u2002<span class="star-status">${statusName}${asOf}</span>` +
            (econ ? `\u2002<span class="star-econ">${econ}</span>` : '')
        log.appendChild(line1)

        if (!sys) return

        // Line 2: local stationary units
        const localParts = []
        if (sys.knownLocalUnits) {
            for (const [type, count] of Object.entries(sys.knownLocalUnits)) {
                if (count > 0) localParts.push(`${count} ${type.replace(/_/g, ' ')}`)
            }
        }
        if (localParts.length > 0) {
            const line2 = document.createElement('div')
            line2.className = 'msg-line'
            line2.innerHTML = `<span class="star-forces">Local units: ${localParts.join(',  ')}</span>`
            log.appendChild(line2)
        }

        // Line 3: fleets present
        const presentFleets = (sys.knownFleets ?? []).filter(f => !f.inTransit)
        if (presentFleets.length > 0) {
            const line3 = document.createElement('div')
            line3.className = 'msg-line'
            line3.innerHTML = `<span class="star-forces">Fleets: ${presentFleets.map(f => f.name).join(',  ')}</span>`
            log.appendChild(line3)
        }

        // Line 4: fleets in transit to this system
        const inboundFleets = (sys.knownFleets ?? []).filter(f => f.inTransit)
        if (inboundFleets.length > 0) {
            const line4 = document.createElement('div')
            line4.className = 'msg-line'
            line4.innerHTML = `<span class="star-forces">In transit: ${inboundFleets.map(f => f.name).join(',  ')}</span>`
            log.appendChild(line4)
        }
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

    showPopupForStar(systemId) {
        const idx = this.stars.findIndex(s => s.id === systemId)
        if (idx < 0) return
        this.sidebarHoverActive = true
        this._clearHoverElements()
        this._showHoverElements(idx)
        this.requestRender()
    }

    hidePopupForStar() {
        this.sidebarHoverActive = false
        this._clearHoverElements()
        this.currentHoveredIndex = -1
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
    // Amber arrows (pending commands; in-transit fleets added later)
    // -------------------------------------------------------------------------

    _rebuildArrows() {
        this._clearAllArrows()

        const pc = this.state.pendingCommands || {}
        for (const id in pc) {
            const cmd = pc[id]
            const from = this._starPos(cmd.originId)
            const to   = this._starPos(cmd.targetId)
            if (!from || !to) continue
            this._addArrow(`cmd-${id}`, from, to, cmd.description, cmd.executeYear)
        }

        const fleets = this.state.inTransitFleets || {}
        for (const id in fleets) {
            const f = fleets[id]
            const from = this._starPos(f.sourceId)
            const to   = this._starPos(f.destinationId)
            if (!from || !to) continue
            this._addArrow(`fleet-${id}`, from, to, this._describeFleetInTransit(f), f.arrivalYear)
        }

        this.requestRender()
    }

    _describeFleetInTransit(f) {
        const dest = this.stars.find(s => s.id === f.destinationId)
        const destName = dest ? dest.displayName : f.destinationId
        const unitParts = []
        for (const [type, count] of Object.entries(f.units || {})) {
            if (count > 0) {
                const label = type.replace(/_/g, ' ') + (count === 1 ? '' : 's')
                unitParts.push(`${count} ${label}`)
            }
        }
        const composition = unitParts.length > 0 ? ` (${unitParts.join(', ')})` : ''
        return `${f.name}${composition} in transit to ${destName} (arrives yr ${f.arrivalYear.toFixed(1)})`
    }

    _clearAllArrows() {
        for (const entry of this.arrows.values()) {
            this.scene.remove(entry.arrowHelper)
            this.scene.remove(entry.hitMesh)
            entry.hitMesh.geometry.dispose()
            entry.hitMesh.material.dispose()
        }
        this.arrows.clear()
        this.arrowHitMeshes = []
        if (this.currentHoveredArrowId !== null) {
            this._clearArrowHover()
            this.currentHoveredArrowId = null
        }
    }

    _starPos(id) {
        const s = this.stars.find(x => x.id === id)
        if (!s) return null
        return new THREE.Vector3(s.x, s.y, s.z)
    }

    _addArrow(id, from, to, description, endYear) {
        const diff = new THREE.Vector3().subVectors(to, from)
        const length = diff.length()
        if (length < 1e-6) return
        const dir = diff.clone().normalize()

        const arrowHelper = new THREE.ArrowHelper(
            dir, from, length, ARROW_COLOR, ARROW_HEAD_LENGTH, ARROW_HEAD_WIDTH,
        )
        this.scene.add(arrowHelper)

        // Invisible narrow cylinder along the arrow for hit-testing
        const geo  = new THREE.CylinderGeometry(ARROW_HIT_RADIUS, ARROW_HIT_RADIUS, length, 8)
        const mat  = new THREE.MeshBasicMaterial({ visible: false })
        const mesh = new THREE.Mesh(geo, mat)
        mesh.position.copy(from).addScaledVector(dir, length * 0.5)
        mesh.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), dir)
        mesh.userData.arrowId = id
        this.scene.add(mesh)

        this.arrows.set(id, { arrowHelper, hitMesh: mesh, description, endYear })
        this.arrowHitMeshes.push(mesh)
    }

    _showArrowHover(id) {
        const entry = this.arrows.get(id)
        if (!entry) return
        const log = document.getElementById('message-log')
        if (!log) return
        log.innerHTML = ''
        const line = document.createElement('div')
        line.className = 'msg-line'
        line.innerHTML = `<span class="star-forces">${entry.description}</span>`
        log.appendChild(line)
    }

    _clearArrowHover() {
        const log = document.getElementById('message-log')
        if (log) log.innerHTML = ''
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
