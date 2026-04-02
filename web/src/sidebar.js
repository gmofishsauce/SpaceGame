export class Sidebar {
    constructor(state, starMap) {
        this.state   = state
        this.starMap = starMap

        this.sidebarEl  = null
        this.eventLogEl = null

        state.on('stateLoaded', () => this._onStateLoaded())
        state.on('newEvent',    (evt) => this._appendEvent(evt))
    }

    init() {
        this.sidebarEl  = document.getElementById('sidebar')
        this.eventLogEl = document.getElementById('event-log')
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
    // Append a single event entry (FR-025, FR-026)
    // -------------------------------------------------------------------------

    _appendEvent(evt) {
        if (!this.eventLogEl) return

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

        this.eventLogEl.appendChild(entry)

        // Auto-scroll if we were already at the bottom (FR-028)
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
