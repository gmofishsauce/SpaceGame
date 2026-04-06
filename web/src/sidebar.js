export class Sidebar {
    constructor(state, starMap, api) {
        this.state   = state
        this.starMap = starMap
        this.api     = api

        this.sidebarEl  = null
        this.eventLogEl = null
        this.debugLogEl = null
        this.debugBtn   = null

        this.debugMode    = false
        this._pollTimer   = null

        state.on('stateLoaded', () => this._onStateLoaded())
        state.on('newEvent',    (evt) => this._appendEvent(evt))
    }

    init() {
        this.sidebarEl  = document.getElementById('sidebar')
        this.eventLogEl = document.getElementById('event-log')
        this.debugLogEl = document.getElementById('debug-log')
        this.debugBtn   = document.getElementById('debug-btn')

        this.debugBtn.addEventListener('click', () => this._toggleDebug())
    }

    // -------------------------------------------------------------------------
    // State load: populate log with all existing events
    // -------------------------------------------------------------------------

    _onStateLoaded() {
        if (!this.eventLogEl) return
        this.eventLogEl.innerHTML = ''
        for (const evt of this.state.events) {
            this._appendEvent(evt)
        }
    }

    // -------------------------------------------------------------------------
    // Append a single event entry to the normal log (FR-025, FR-026)
    // -------------------------------------------------------------------------

    _appendEvent(evt) {
        if (!this.eventLogEl) return
        if (this.debugMode) return   // don't accumulate while debug view is shown

        const atBottom = this._isAtBottom()

        const entry = document.createElement('div')
        entry.className = 'event-entry'
        entry.dataset.systemId = evt.systemId

        const yearSpan = document.createElement('span')
        yearSpan.className = 'evt-year'
        yearSpan.textContent = `Year ${evt.arrivalYear.toFixed(1)}`

        const systemName = this._systemName(evt.systemId)
        const sysSpan = document.createElement('span')
        sysSpan.className = 'evt-system'
        sysSpan.textContent = systemName

        const descSpan = document.createElement('span')
        descSpan.className = 'evt-desc'
        descSpan.textContent = evt.description

        entry.appendChild(yearSpan)
        entry.appendChild(sysSpan)
        entry.appendChild(descSpan)

        // Sidebar hover highlights the corresponding star (FR-027)
        entry.addEventListener('mouseenter', () => {
            this.starMap.highlightStar(evt.systemId)
        })
        entry.addEventListener('mouseleave', () => {
            this.starMap.unhighlightStar()
        })

        // Hovering the bold system name shows the standard popup on the star map
        sysSpan.addEventListener('mouseenter', () => {
            this.starMap.showPopupForStar(evt.systemId)
        })
        sysSpan.addEventListener('mouseleave', () => {
            this.starMap.hidePopupForStar()
        })

        this.eventLogEl.appendChild(entry)

        // Auto-scroll if we were already at the bottom (FR-028)
        if (atBottom) {
            this.sidebarEl.scrollTop = this.sidebarEl.scrollHeight
        }
    }

    // -------------------------------------------------------------------------
    // Debug mode toggle
    // -------------------------------------------------------------------------

    _toggleDebug() {
        this.debugMode = !this.debugMode

        if (this.debugMode) {
            this.debugBtn.textContent = 'normal'
            this.eventLogEl.style.display = 'none'
            this.debugLogEl.style.display = ''
            this._startDebugPoll()
        } else {
            this.debugBtn.textContent = 'debug'
            this._stopDebugPoll()
            this.debugLogEl.style.display = 'none'
            this.debugLogEl.innerHTML = ''
            this.eventLogEl.style.display = ''
        }
    }

    _startDebugPoll() {
        this._fetchAndRenderDebug()
        this._pollTimer = setInterval(() => this._fetchAndRenderDebug(), 2000)
    }

    _stopDebugPoll() {
        if (this._pollTimer !== null) {
            clearInterval(this._pollTimer)
            this._pollTimer = null
        }
    }

    async _fetchAndRenderDebug() {
        let data
        try {
            data = await this.api.fetchDebugState()
        } catch (e) {
            return
        }
        if (!this.debugMode) return   // toggled off while fetch was in flight

        const atBottom = this._isAtBottom()
        this.debugLogEl.innerHTML = ''

        for (const evt of data.events) {
            const entry = document.createElement('div')
            entry.className = 'event-entry'
            entry.dataset.systemId = evt.systemId

            const yearSpan = document.createElement('span')
            yearSpan.className = 'evt-year'
            yearSpan.textContent = `Year ${evt.eventYear.toFixed(1)}`

            const sysSpan = document.createElement('span')
            sysSpan.className = 'evt-system'
            sysSpan.textContent = this._systemName(evt.systemId)

            const descSpan = document.createElement('span')
            descSpan.className = 'evt-desc'
            descSpan.textContent = `[${evt.type}] ${evt.description}`

            entry.appendChild(yearSpan)
            entry.appendChild(sysSpan)
            entry.appendChild(descSpan)

            entry.addEventListener('mouseenter', () => {
                this.starMap.highlightStar(evt.systemId)
            })
            entry.addEventListener('mouseleave', () => {
                this.starMap.unhighlightStar()
            })

            // Hovering the bold system name shows the standard popup on the star map
            sysSpan.addEventListener('mouseenter', () => {
                this.starMap.showPopupForStar(evt.systemId)
            })
            sysSpan.addEventListener('mouseleave', () => {
                this.starMap.hidePopupForStar()
            })

            this.debugLogEl.appendChild(entry)
        }

        if (atBottom) {
            this.sidebarEl.scrollTop = this.sidebarEl.scrollHeight
        }
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    _isAtBottom() {
        if (!this.sidebarEl) return true
        return this.sidebarEl.scrollTop + this.sidebarEl.clientHeight >=
               this.sidebarEl.scrollHeight - 10
    }

    _systemName(systemId) {
        const sys = this.state.getKnownState(systemId)
        if (sys && sys.displayName) return sys.displayName
        // Fall back to the star catalogue if the system DTO isn't loaded yet
        const star = this.state.stars.find(s => s.id === systemId)
        return star ? star.displayName : systemId
    }
}
