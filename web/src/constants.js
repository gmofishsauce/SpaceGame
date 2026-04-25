export const TimeScaleYearsPerSecond = 10.0 / 180.0
export const CommandSpeedC = 1.0 // commands travel at c (Sol has a comm laser)
export const FleetSpeedC = 0.8

export const STATUS_COLORS = {
    human:       0x4488ff,
    alien:       0xff3333,
    contested:   0xff8800,
    unknown:     0x888888,
    uninhabited: 0xffffff,
}
export const SOL_COLOR = 0xffff88

// EconWealthRate[level] = wealth units per in-game year = 2^level (indices 0..4)
export const ECON_WEALTH_RATE = [1.0, 2.0, 4.0, 8.0, 16.0]

// Mirrored weapon defs for UI display (costs and min levels)
export const WEAPON_DEFS = {
    orbital_defense: { displayName: 'Orbital Defense', cost: 1,  minLevel: 1 },
    interceptor:     { displayName: 'Interceptor',     cost: 2,  minLevel: 1 },
    reporter:        { displayName: 'Reporter',         cost: 4,  minLevel: 1 },
    escort:          { displayName: 'Escort',           cost: 8,  minLevel: 2 },
    battleship:      { displayName: 'Battleship',       cost: 32, minLevel: 3 },
    comm_laser:      { displayName: 'Comm Laser',       cost: 64, minLevel: 4 },
}
