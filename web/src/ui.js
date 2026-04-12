import { CommandSpeedC, FleetSpeedC, WEAPON_DEFS } from './constants.js'

export class UIController {
    constructor(state, api, starMap) {
        this.state   = state
        this.api     = api
        this.starMap = starMap

        this.yearDisplayEl  = null
        this.pauseOverlayEl = null
        this.contextMenuEl  = null
        this.messageLogEl   = null

        state.on('stateLoaded', ()     => this._syncPauseOverlay())
        state.on('clockSync',   ()     => this._syncPauseOverlay())
        state.on('gameOver',    (data) => this.showGameOverScreen(data))
    }

    init() {
        this.yearDisplayEl  = document.getElementById('year-display')
        this.pauseOverlayEl = document.getElementById('pause-overlay')
        this.messageLogEl   = document.getElementById('message-log')

        document.getElementById('build-btn').addEventListener('click', () => {
            this.showBuildAllDialog()
        })

        // Local year interpolation at 100 ms (FR-014, NFR-U-1)
        setInterval(() => this._updateYearDisplay(), 100)

        // Escape: exit destination mode or toggle pause (FR-013)
        window.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') this._onEscape()
        })

        // Click outside any open context menu closes it
        document.addEventListener('click', () => this._closeContextMenu())
    }

    // -------------------------------------------------------------------------
    // Year display (FR-014)
    // -------------------------------------------------------------------------

    _updateYearDisplay() {
        if (!this.yearDisplayEl) return
        this.yearDisplayEl.textContent = `Year: ${this.state.getLocalYear().toFixed(1)}`
    }

    // -------------------------------------------------------------------------
    // Pause overlay (FR-013)
    // -------------------------------------------------------------------------

    _syncPauseOverlay() {
        if (!this.pauseOverlayEl) return
        this.pauseOverlayEl.style.display = this.state.paused ? 'flex' : 'none'
    }

    _onEscape() {
        if (this.starMap.selectionMode) {
            this.starMap.exitSelectionMode()
            return
        }
        this.api.setPaused(!this.state.paused)
    }

    // -------------------------------------------------------------------------
    // Context menu (FR-029, FR-030)
    // -------------------------------------------------------------------------

    showContextMenu(x, y, star) {
        this._closeContextMenu()

        const sys    = this.state.getKnownState(star.id)
        const status = sys?.knownStatus ?? 'unknown'

        const menu = document.createElement('div')
        menu.className  = 'context-menu'
        menu.style.left = `${x}px`
        menu.style.top  = `${y}px`

        const title = document.createElement('div')
        title.className   = 'context-menu-title'
        title.textContent = star.displayName
        menu.appendChild(title)

        if (star.isSol) {
            // Sol: "Construct…" unconditionally (FR-030, FR-034); fleet command if fleets present
            menu.appendChild(this._menuItem('Construct\u2026', () => {
                this._closeContextMenu()
                this.starMap.enterSelectionMode('construct')
            }))

            const stationedFleets = (sys?.knownFleets ?? []).filter(f => !f.inTransit)
            if (stationedFleets.length > 0) {
                menu.appendChild(this._menuItem('Command Fleet\u2026', () => {
                    this._closeContextMenu()
                    this.showFleetCommandDialog(star, stationedFleets)
                }))
            }
        } else if (status === 'human') {
            // Non-Sol human system: fleet command only, never construction (FR-034)
            const stationedFleets = (sys?.knownFleets ?? []).filter(f => !f.inTransit)
            if (stationedFleets.length > 0) {
                menu.appendChild(this._menuItem('Command Fleet\u2026', () => {
                    this._closeContextMenu()
                    this.showFleetCommandDialog(star, stationedFleets)
                }))
            } else {
                menu.appendChild(this._disabledItem('No actions available'))
            }
        } else {
            // Alien-held, unknown, or uninhabited (FR-029)
            menu.appendChild(this._disabledItem('No actions available'))
        }

        document.body.appendChild(menu)
        this.contextMenuEl = menu

        // Prevent the document-level click listener from immediately closing this menu
        menu.addEventListener('click', e => e.stopPropagation())
    }

    _menuItem(label, onClick) {
        const item = document.createElement('div')
        item.className   = 'context-menu-item'
        item.textContent = label
        item.addEventListener('click', onClick)
        return item
    }

    _disabledItem(label) {
        const item = document.createElement('div')
        item.className   = 'context-menu-item disabled'
        item.textContent = label
        return item
    }

    _closeContextMenu() {
        if (this.contextMenuEl) {
            this.contextMenuEl.remove()
            this.contextMenuEl = null
        }
    }

    // -------------------------------------------------------------------------
    // Build-all dialog (Build... button)
    // -------------------------------------------------------------------------

    showBuildAllDialog() {
        // Snapshot human-held systems at dialog-open time
        const humanSystems = this.state.stars
            .filter(s => this.state.getKnownStatus(s.id) === 'human')
            .map(s => ({ star: s, sys: this.state.getKnownState(s.id) }))
            .filter(({ sys }) => sys != null)

        const modal = this._makeModal()

        const title = document.createElement('h2')
        title.textContent = 'Build Orders'
        modal.content.appendChild(title)

        if (humanSystems.length === 0) {
            const p = document.createElement('p')
            p.textContent = 'No human-held systems available.'
            modal.content.appendChild(p)
            modal.content.appendChild(this._cancelButton(() => modal.overlay.remove()))
            document.body.appendChild(modal.overlay)
            return
        }

        // "Set all" control
        const weaponsByDescCost = Object.entries(WEAPON_DEFS)
            .sort(([, a], [, b]) => b.cost - a.cost)

        const setAllRow = document.createElement('div')
        setAllRow.className = 'build-all-setall'

        const setAllLabel = document.createElement('span')
        setAllLabel.textContent = 'Set all: '
        setAllRow.appendChild(setAllLabel)

        const setAllSel = document.createElement('select')
        setAllSel.className = 'build-all-select'

        ;[['', '— choose —'], ['best', 'Best affordable'],
          ...weaponsByDescCost.map(([id, def]) => [id, `${def.displayName} (${def.cost})`])
        ].forEach(([val, label]) => {
            const opt = document.createElement('option')
            opt.value = val
            opt.textContent = label
            setAllSel.appendChild(opt)
        })

        setAllRow.appendChild(setAllSel)
        modal.content.appendChild(setAllRow)

        const table = document.createElement('table')
        table.className = 'build-all-table'
        table.innerHTML =
            '<thead><tr>' +
            '<th></th><th>System</th><th>Level</th><th>Wealth</th><th>Weapon</th>' +
            '</tr></thead>'

        const checkAll = document.createElement('input')
        checkAll.type = 'checkbox'
        table.querySelector('thead th').appendChild(checkAll)

        const tbody = document.createElement('tbody')

        const rows = []
        for (const { star, sys } of humanSystems) {
            const econLevel  = sys.knownEconLevel ?? 0
            const deltaYears = Math.max(0, this.state.getLocalYear() - (sys.knownAsOfYear ?? 0))
            const wealth     = this.state.getProjectedWealth(star.id, deltaYears)

            const buildable = Object.entries(WEAPON_DEFS)
                .filter(([, def]) => def.minLevel <= econLevel)

            const row = document.createElement('tr')

            const cbTd = document.createElement('td')
            const cb   = document.createElement('input')
            cb.type = 'checkbox'
            cbTd.appendChild(cb)
            row.appendChild(cbTd)

            const nameTd = document.createElement('td')
            nameTd.textContent = star.displayName
            row.appendChild(nameTd)

            const levelTd = document.createElement('td')
            levelTd.textContent = econLevel
            row.appendChild(levelTd)

            const wealthTd = document.createElement('td')
            wealthTd.textContent = wealth.toFixed(1)
            row.appendChild(wealthTd)

            const selTd = document.createElement('td')
            const sel   = document.createElement('select')
            for (const [typeId, def] of buildable) {
                const opt       = document.createElement('option')
                opt.value       = typeId
                opt.textContent = `${def.displayName} (${def.cost})`
                sel.appendChild(opt)
            }
            selTd.appendChild(sel)
            row.appendChild(selTd)

            tbody.appendChild(row)
            rows.push({ star, cb, sel, wealth })
        }

        checkAll.addEventListener('change', () => {
            for (const { cb } of rows) cb.checked = checkAll.checked
        })

        setAllSel.addEventListener('change', () => {
            const val = setAllSel.value
            setAllSel.value = ''   // reset to prompt immediately
            if (!val) return

            for (const { cb, sel, wealth } of rows) {
                if (val === 'best') {
                    for (const [typeId, def] of weaponsByDescCost) {
                        if (typeId === 'comm_laser' || typeId === 'reporter') continue
                        if ([...sel.options].some(o => o.value === typeId) && def.cost <= wealth) {
                            sel.value = typeId
                            cb.checked = true
                            break
                        }
                    }
                } else {
                    if ([...sel.options].some(o => o.value === val)) {
                        sel.value = val
                        cb.checked = true
                    }
                }
            }
        })

        table.appendChild(tbody)
        modal.content.appendChild(table)

        const okBtn = document.createElement('button')
        okBtn.textContent = 'OK'
        okBtn.addEventListener('click', () => {
            modal.overlay.remove()
            for (const { star, cb, sel } of rows) {
                if (cb.checked) this._sendConstruct(star.id, sel.value)
            }
        })
        modal.content.appendChild(okBtn)
        modal.content.appendChild(this._cancelButton(() => modal.overlay.remove()))

        document.body.appendChild(modal.overlay)
    }

    // -------------------------------------------------------------------------
    // Construction dialog (FR-034, FR-035, FR-036)
    // -------------------------------------------------------------------------

    showConstructDialog(star) {
        const sys            = this.state.getKnownState(star.id)
        const econLevel      = sys?.knownEconLevel ?? (star.isSol ? 4 : 0)
        const travelYears    = star.isSol ? 0 : (star.distFromSol / CommandSpeedC)
        const projWealth     = this.state.getProjectedWealth(star.id, travelYears)

        const modal = this._makeModal()

        const title = document.createElement('h2')
        title.textContent = `Construct at ${star.displayName}`
        modal.content.appendChild(title)

        if (!star.isSol) {
            const note = document.createElement('p')
            note.className   = 'dialog-note'
            note.textContent =
                `Command arrives in ${travelYears.toFixed(1)} years. ` +
                `Projected wealth at arrival: ${projWealth.toFixed(1)}.`
            modal.content.appendChild(note)
        }

        const table = document.createElement('table')
        table.className   = 'weapon-table'
        table.innerHTML =
            '<thead><tr><th>Weapon</th><th>Cost</th><th>Min Level</th><th></th></tr></thead>'
        const tbody = document.createElement('tbody')

        for (const [typeId, def] of Object.entries(WEAPON_DEFS)) {
            const canLevel  = econLevel  >= def.minLevel
            const canAfford = projWealth >= def.cost
            const canBuild  = canLevel && canAfford

            const row = document.createElement('tr')
            row.innerHTML =
                `<td>${def.displayName}</td>` +
                `<td>${def.cost}</td>` +
                `<td>${def.minLevel}</td>`

            const td  = document.createElement('td')
            const btn = document.createElement('button')
            btn.textContent = 'Build'
            btn.disabled    = !canBuild
            if (!canLevel) {
                btn.title = `Requires economy level ${def.minLevel}`
            } else if (!canAfford) {
                btn.title = `Need ${def.cost} wealth (projected: ${projWealth.toFixed(1)})`
            }
            btn.addEventListener('click', () => {
                modal.overlay.remove()
                this._sendConstruct(star.id, typeId)
            })
            td.appendChild(btn)
            row.appendChild(td)
            tbody.appendChild(row)
        }

        table.appendChild(tbody)
        modal.content.appendChild(table)
        modal.content.appendChild(this._cancelButton(() => modal.overlay.remove()))
        document.body.appendChild(modal.overlay)
    }

    async _sendConstruct(systemId, weaponType) {
        const resp = await this.api.sendCommand({
            type: 'construct',
            systemId,
            weaponType,
            quantity: 1,
        })
        if (!resp.ok) this.showCommandError(resp.error)
    }

    // -------------------------------------------------------------------------
    // Fleet command dialog (FR-037, FR-038)
    // -------------------------------------------------------------------------

    showFleetCommandDialog(star, fleets) {
        const savedHTML = this.messageLogEl.innerHTML
        const restore   = () => { this.messageLogEl.innerHTML = savedHTML }

        this.messageLogEl.innerHTML = ''

        const header = document.createElement('div')
        header.className = 'msg-line'
        const nameSpan = document.createElement('span')
        nameSpan.className = 'evt-system'
        nameSpan.textContent = `Command Fleet from ${star.displayName}`
        header.appendChild(nameSpan)
        this.messageLogEl.appendChild(header)

        const note = document.createElement('div')
        note.className = 'msg-line'
        const noteSpan = document.createElement('span')
        noteSpan.className = 'evt-desc'
        noteSpan.textContent = 'Select a fleet, then click a destination star.'
        note.appendChild(noteSpan)
        this.messageLogEl.appendChild(note)

        for (const fleet of fleets) {
            const item = document.createElement('div')
            item.className = 'msg-line'
            item.style.cursor = 'pointer'
            const span = document.createElement('span')
            span.className = 'evt-system'
            span.textContent = `\u25b6 ${fleet.name} \u2014 ${this._formatUnits(fleet.units)}`
            item.appendChild(span)
            item.addEventListener('click', () => {
                restore()
                this.starMap.enterSelectionMode('fleet', fleet.id, star.id)
            })
            this.messageLogEl.appendChild(item)
        }

        const cancelLine = document.createElement('div')
        cancelLine.className = 'msg-line'
        cancelLine.appendChild(this._cancelButton(restore))
        this.messageLogEl.appendChild(cancelLine)
    }

    // Called by StarMap when the user clicks a destination star (FR-038, A-5)
    confirmFleetDestination(destStar) {
        const fleetId  = this.starMap.destinationFleetId
        const originId = this.starMap.destinationOriginId

        const originStar       = this.state.stars.find(s => s.id === originId)
        const commandYears     = (originStar && !originStar.isSol)
            ? originStar.distFromSol / CommandSpeedC : 0
        const fleetTravelYears = originStar
            ? this._starDist(originStar, destStar) / FleetSpeedC : 0
        const totalYears       = commandYears + fleetTravelYears

        const sys       = this.state.getKnownState(originId)
        const fleet     = sys?.knownFleets?.find(f => f.id === fleetId)
        const fleetName = fleet?.name ?? fleetId

        const modal = this._makeModal()

        const title = document.createElement('h2')
        title.textContent = 'Confirm Fleet Movement'
        modal.content.appendChild(title)

        const msg = document.createElement('p')
        msg.textContent =
            `Send ${fleetName} from ${originStar?.displayName ?? originId} ` +
            `to ${destStar.displayName}?`
        modal.content.appendChild(msg)

        const timing = document.createElement('p')
        timing.className   = 'dialog-note'
        timing.textContent =
            `Command arrives at origin in ${commandYears.toFixed(1)} years. ` +
            `Fleet reaches destination ~${totalYears.toFixed(1)} years from now.`
        modal.content.appendChild(timing)

        const confirmBtn = document.createElement('button')
        confirmBtn.textContent = 'Confirm'
        confirmBtn.addEventListener('click', () => {
            modal.overlay.remove()
            this._sendMove(originId, fleetId, destStar.id)
        })
        modal.content.appendChild(confirmBtn)

        // Cancel puts the player back into destination-selection mode
        modal.content.appendChild(this._cancelButton(() => {
            modal.overlay.remove()
            this.starMap.enterSelectionMode('fleet', fleetId, originId)
        }))

        document.body.appendChild(modal.overlay)
    }

    async _sendMove(systemId, fleetId, destinationId) {
        const resp = await this.api.sendCommand({
            type: 'move',
            systemId,
            fleetId,
            destinationId,
        })
        if (!resp.ok) this.showCommandError(resp.error)
    }

    // -------------------------------------------------------------------------
    // Game-over screen (FR-058)
    // -------------------------------------------------------------------------

    showGameOverScreen(data) {
        const overlay = document.createElement('div')
        overlay.id = 'game-over-overlay'

        const box = document.createElement('div')
        box.id = 'game-over-box'

        const heading = document.createElement('h1')
        heading.textContent = data.winner === 'human' ? 'VICTORY' : 'DEFEAT'
        heading.className   = data.winner === 'human' ? 'victory-text' : 'defeat-text'
        box.appendChild(heading)

        const reason = document.createElement('p')
        reason.textContent = data.reason ?? ''
        box.appendChild(reason)

        const closeBtn = document.createElement('button')
        closeBtn.textContent = 'Close'
        closeBtn.addEventListener('click', () => overlay.remove())
        box.appendChild(closeBtn)

        overlay.appendChild(box)
        document.body.appendChild(overlay)
    }

    // -------------------------------------------------------------------------
    // Error toast
    // -------------------------------------------------------------------------

    showCommandError(message) {
        const toast = document.createElement('div')
        toast.className   = 'error-toast'
        toast.textContent = message
        document.body.appendChild(toast)
        setTimeout(() => toast.remove(), 4000)
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    _makeModal() {
        const overlay = document.createElement('div')
        overlay.className = 'modal-overlay'

        const content = document.createElement('div')
        content.className = 'modal-content'
        overlay.appendChild(content)

        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) overlay.remove()
        })

        return { overlay, content }
    }

    _cancelButton(onClick) {
        const btn = document.createElement('button')
        btn.className   = 'cancel-btn'
        btn.textContent = 'Cancel'
        btn.addEventListener('click', onClick)
        return btn
    }

    _formatUnits(units) {
        if (!units) return ''
        return Object.entries(units)
            .filter(([, count]) => count > 0)
            .map(([type, count]) => `${count} ${type.replace(/_/g, ' ')}`)
            .join(', ')
    }

    _starDist(a, b) {
        const dx = a.x - b.x, dy = a.y - b.y, dz = a.z - b.z
        return Math.sqrt(dx*dx + dy*dy + dz*dz)
    }
}
