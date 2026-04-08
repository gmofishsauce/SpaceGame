export class APIClient {
    constructor(state) {
        this.state = state
        this.es = null
    }

    async fetchStars() {
        const r = await fetch('/api/stars')
        if (!r.ok) throw new Error(`/api/stars: ${r.status}`)
        return r.json()
    }

    async fetchState() {
        const r = await fetch('/api/state')
        if (!r.ok) throw new Error(`/api/state: ${r.status}`)
        return r.json()
    }

    async sendCommand(cmd) {
        const r = await fetch('/api/command', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(cmd),
        })
        if (!r.ok) throw new Error(`/api/command: ${r.status}`)
        return r.json()
    }

    async fetchDebugState() {
        const r = await fetch('/api/debug/state')
        if (!r.ok) throw new Error(`/api/debug/state: ${r.status}`)
        return r.json()
    }

    async setPaused(paused) {
        const r = await fetch('/api/pause', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ paused }),
        })
        if (!r.ok) throw new Error(`/api/pause: ${r.status}`)
        return r.json()
    }

    connectSSE() {
        this.es = new EventSource('/api/events')

        this.es.addEventListener('connected', e => {
            // Full state snapshot on (re)connect; re-sync in case we missed events.
            const gs = JSON.parse(e.data)
            this.state.initGameState(gs)
        })

        this.es.addEventListener('clock_sync', e => {
            this.state.onClockSync(JSON.parse(e.data))
        })

        this.es.addEventListener('game_event', e => {
            this.state.onGameEvent(JSON.parse(e.data))
        })

        this.es.addEventListener('system_update', e => {
            this.state.onSystemUpdate(JSON.parse(e.data))
        })

        this.es.addEventListener('game_over', e => {
            this.state.onGameOver(JSON.parse(e.data))
        })

        // EventSource reconnects automatically on error; on reconnect the
        // 'connected' handler above re-syncs any missed state.
    }
}
