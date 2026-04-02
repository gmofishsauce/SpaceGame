import { TimeScaleYearsPerSecond, ECON_WEALTH_RATE } from './constants.js'

export class ClientState {
    constructor() {
        this.stars = []         // static, from /api/stars
        this.systems = {}       // key: systemId → SystemDTO
        this.events = []        // EventDTOs, sorted by arrivalYear
        this.gameYear = 0.0
        this.paused = false
        this.gameOver = false
        this.listeners = []     // {event: string, fn: function}

        this.localClockBase = 0.0
        this.localClockBaseTime = Date.now()
    }

    on(event, fn) { this.listeners.push({ event, fn }) }
    emit(event, data) { this.listeners.filter(l => l.event === event).forEach(l => l.fn(data)) }

    initStars(stars) { this.stars = stars }

    initGameState(gs) {
        this.gameYear = gs.gameYear
        this.paused = gs.paused
        this.gameOver = gs.gameOver
        gs.systems.forEach(s => this.systems[s.id] = s)
        this.events = gs.events.slice().sort((a, b) => a.arrivalYear - b.arrivalYear)
        this.localClockBase = gs.gameYear
        this.localClockBaseTime = Date.now()
        this.emit('stateLoaded', this)
    }

    onClockSync(data) {
        this.gameYear = data.gameYear
        this.paused = data.paused
        this.localClockBase = data.gameYear
        this.localClockBaseTime = Date.now()
        this.emit('clockSync', data)
    }

    onGameEvent(evt) {
        this.events.push(evt)
        this.emit('newEvent', evt)
    }

    onSystemUpdate(upd) {
        if (this.systems[upd.systemId]) {
            Object.assign(this.systems[upd.systemId], upd)
        } else {
            this.systems[upd.systemId] = upd
        }
        this.emit('systemUpdated', upd.systemId)
    }

    onGameOver(data) {
        this.gameOver = true
        this.emit('gameOver', data)
    }

    // Local time interpolation between clock_sync events (FR-014, NFR-U-1)
    getLocalYear() {
        if (this.paused) return this.gameYear
        const elapsed = (Date.now() - this.localClockBaseTime) / 1000.0
        return this.localClockBase + elapsed * TimeScaleYearsPerSecond
    }

    getKnownStatus(systemId) { return this.systems[systemId]?.knownStatus ?? 'unknown' }
    getKnownState(systemId)  { return this.systems[systemId] }

    getProjectedWealth(systemId, deltaYears) {
        const sys = this.systems[systemId]
        if (!sys) return 0
        const rate = ECON_WEALTH_RATE[sys.knownEconLevel] ?? 0
        return (sys.knownWealth ?? 0) + rate * deltaYears
    }
}
