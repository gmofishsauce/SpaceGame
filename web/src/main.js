import { ClientState }   from './state.js'
import { APIClient }     from './api.js'
import { StarMap }       from './starmap.js'
import { Sidebar }       from './sidebar.js'
import { UIController }  from './ui.js'

const state   = new ClientState()
const api     = new APIClient(state)
const starMap = new StarMap(state)
const sidebar = new Sidebar(state, starMap, api)
const ui      = new UIController(state, api, starMap)
starMap.setUIController(ui)

async function init() {
    const [stars, gameState] = await Promise.all([api.fetchStars(), api.fetchState()])

    // Stars must be stored before starMap.init so the first updateStarColors
    // can resolve system names; game state is applied last so all DOM elements
    // are ready before stateLoaded fires its listeners.
    state.initStars(stars)
    starMap.init(stars)
    sidebar.init()
    ui.init()
    state.initGameState(gameState)  // emits 'stateLoaded' → colors, sidebar, pause all update
    api.connectSSE()
}

document.addEventListener('DOMContentLoaded', init)
