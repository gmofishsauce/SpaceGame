// Package bot implements the alien bot: HTTP/SSE client, local state mirror,
// and v1 strategy. It does NOT import srv/internal/game. (FR-63)
package bot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Wire-shape types mirror the server's JSON; declared locally per FR-63.

type Star struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Z           float64 `json:"z"`
	DistFromSol float64 `json:"distFromSol"`
	HasPlanets  bool    `json:"hasPlanets"`
	IsSol       bool    `json:"isSol"`
}

type SystemView struct {
	ID              string         `json:"id"`
	DisplayName     string         `json:"displayName"`
	KnownStatus     string         `json:"knownStatus"`
	KnownAsOfYear   float64        `json:"knownAsOfYear"`
	KnownEconLevel  int            `json:"knownEconLevel"`
	KnownWealth     float64        `json:"knownWealth"`
	KnownLocalUnits map[string]int `json:"knownLocalUnits"`
	KnownFleets     []FleetView    `json:"knownFleets"`
}

type FleetView struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Owner       string         `json:"owner"`
	Units       map[string]int `json:"units"`
	InTransit   bool           `json:"inTransit"`
	SourceID    string         `json:"sourceId,omitempty"`
	DestID      string         `json:"destinationId,omitempty"`
	DepartYear  float64        `json:"departYear,omitempty"`
	ArrivalYear float64        `json:"arrivalYear,omitempty"`
}

type EventView struct {
	ID          string  `json:"id"`
	ArrivalYear float64 `json:"arrivalYear"`
	SystemID    string  `json:"systemId"`
	Type        string  `json:"type"`
	Description string  `json:"description"`
}

type PendingCommandView struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	OriginID    string  `json:"originId"`
	TargetID    string  `json:"targetId"`
	ExecuteYear float64 `json:"executeYear"`
	Description string  `json:"description"`
}

type StateSnapshot struct {
	GameYear        float64              `json:"gameYear"`
	Paused          bool                 `json:"paused"`
	GameOver        bool                 `json:"gameOver"`
	Winner          string               `json:"winner,omitempty"`
	WinReason       string               `json:"winReason,omitempty"`
	Systems         []SystemView         `json:"systems"`
	Events          []EventView          `json:"events"`
	PendingCommands []PendingCommandView `json:"pendingCommands"`
	FleetsInTransit []FleetView          `json:"fleetsInTransit"`
}

type CommandRequest struct {
	Type       string         `json:"type"`
	SystemID   string         `json:"systemId"`
	WeaponType string         `json:"weaponType,omitempty"`
	Quantity   int            `json:"quantity,omitempty"`
	FleetID    string         `json:"fleetId,omitempty"`
	DestID     string         `json:"destinationId,omitempty"`
	Units      map[string]int `json:"units,omitempty"`
}

type CommandResponse struct {
	OK                   bool    `json:"ok"`
	CommandID            string  `json:"commandId,omitempty"`
	EstimatedArrivalYear float64 `json:"estimatedArrivalYear,omitempty"`
	FleetName            string  `json:"fleetName,omitempty"`
	Error                string  `json:"error,omitempty"`
}

type DebugSnapshot struct {
	GameYear float64 `json:"gameYear"`
	// Raw map to avoid pulling in full type definitions.
	Events []json.RawMessage `json:"events"`
}

// EventFrame is one SSE frame delivered to the StreamEvents handler.
type EventFrame struct {
	Name string
	Data []byte
}

// Client speaks the public HTTP/SSE API as one specific player. (FR-63)
type Client struct {
	BaseURL string
	Player  string
	HTTP    *http.Client
}

func NewClient(baseURL, player string) *Client {
	return &Client{BaseURL: baseURL, Player: player, HTTP: &http.Client{}}
}

func (c *Client) GetStars(ctx context.Context) ([]Star, error) {
	var out []Star
	if err := c.getJSON(ctx, "/api/stars", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetState(ctx context.Context) (*StateSnapshot, error) {
	var out StateSnapshot
	if err := c.getJSON(ctx, "/api/state?player="+c.Player, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PostCommand(ctx context.Context, cmd CommandRequest) (*CommandResponse, error) {
	body, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/api/command?player="+c.Player, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out CommandResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode command response: %w", err)
	}
	return &out, nil
}

func (c *Client) GetDebugState(ctx context.Context) (*DebugSnapshot, error) {
	var out DebugSnapshot
	if err := c.getJSON(ctx, "/api/debug/state", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StreamEvents opens an SSE subscription and calls handler for each frame
// until ctx is cancelled or the connection drops. (FR-65)
func (c *Client) StreamEvents(ctx context.Context, handler func(EventFrame)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+"/api/events?player="+c.Player, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SSE connect %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewReader(resp.Body)
	var eventName string
	var dataBuf []byte
	for {
		line, err := scanner.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if err != nil && err != io.EOF {
			return err
		}
		if line == "" {
			// blank line → dispatch frame
			if eventName != "" && len(dataBuf) > 0 {
				handler(EventFrame{Name: eventName, Data: dataBuf})
			}
			eventName = ""
			dataBuf = nil
		} else if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataBuf = append(dataBuf, []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))...)
		}
		if err == io.EOF {
			return io.EOF
		}
	}
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
