package server

import (
	"github.com/gmofishsauce/SpaceGame/srv/internal/game"
)

// CommandRequest is the JSON body for POST /api/command.
type CommandRequest struct {
	Type       game.CommandType `json:"type"`
	SystemID   string           `json:"systemId"`
	WeaponType game.WeaponType  `json:"weaponType,omitempty"`
	Quantity   int              `json:"quantity,omitempty"`
	FleetID    string           `json:"fleetId,omitempty"`
	DestID     string           `json:"destinationId,omitempty"`
}

// CommandResponse is the JSON body returned by POST /api/command.
type CommandResponse struct {
	OK                   bool    `json:"ok"`
	CommandID            string  `json:"commandId,omitempty"`
	EstimatedArrivalYear float64 `json:"estimatedArrivalYear,omitempty"`
	Error                string  `json:"error,omitempty"`
}

// PauseRequest is the JSON body for POST /api/pause.
type PauseRequest struct {
	Paused bool `json:"paused"`
}

// StarDTO is the static star data returned by GET /api/stars.
type StarDTO struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Z           float64 `json:"z"`
	DistFromSol float64 `json:"distFromSol"`
	HasPlanets  bool    `json:"hasPlanets"`
	IsSol       bool    `json:"isSol"`
}

// SystemDTO is the player-visible state of one star system.
type SystemDTO struct {
	ID              string            `json:"id"`
	DisplayName     string            `json:"displayName"`
	KnownStatus     game.SystemStatus `json:"knownStatus"`
	KnownAsOfYear   float64           `json:"knownAsOfYear"`
	KnownEconLevel  int               `json:"knownEconLevel"`
	KnownWealth     float64           `json:"knownWealth"`
	KnownLocalUnits map[string]int    `json:"knownLocalUnits"`
	KnownFleets     []FleetDTO        `json:"knownFleets"`
}

// FleetDTO is the player-visible state of one fleet.
type FleetDTO struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Owner       game.Owner     `json:"owner"`
	Units       map[string]int `json:"units"`
	InTransit   bool           `json:"inTransit"`
	DestID      string         `json:"destinationId,omitempty"`
	ArrivalYear float64        `json:"arrivalYear,omitempty"`
}

// EventDTO is a player-visible event entry.
type EventDTO struct {
	ID          string  `json:"id"`
	ArrivalYear float64 `json:"arrivalYear"`
	SystemID    string  `json:"systemId"`
	Type        string  `json:"type"`
	Description string  `json:"description"`
}

// StateResponse is the full snapshot returned by GET /api/state.
type StateResponse struct {
	GameYear  float64     `json:"gameYear"`
	Paused    bool        `json:"paused"`
	GameOver  bool        `json:"gameOver"`
	Winner    string      `json:"winner,omitempty"`
	WinReason string      `json:"winReason,omitempty"`
	Systems   []SystemDTO `json:"systems"`
	Events    []EventDTO  `json:"events"`
}
