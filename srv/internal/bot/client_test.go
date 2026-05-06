package bot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClient_PlayerParamAppended verifies that every API call appends
// ?player=alien to the URL. (FR-66)
func TestClient_PlayerParamAppended(t *testing.T) {
	var seenPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.URL.String())
		// Return minimal valid JSON for each endpoint.
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/stars"):
			json.NewEncoder(w).Encode([]Star{})
		case strings.HasPrefix(r.URL.Path, "/api/state"):
			json.NewEncoder(w).Encode(StateSnapshot{})
		case strings.HasPrefix(r.URL.Path, "/api/command"):
			json.NewEncoder(w).Encode(CommandResponse{OK: true})
		case strings.HasPrefix(r.URL.Path, "/api/debug/state"):
			json.NewEncoder(w).Encode(DebugSnapshot{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewClient(srv.URL, "alien")

	c.GetStars(ctx)
	c.GetState(ctx)
	c.PostCommand(ctx, CommandRequest{Type: "construct", SystemID: "sol"})
	c.GetDebugState(ctx)

	for _, path := range seenPaths {
		// These endpoints do not use ?player — exempt from the check.
		if strings.HasPrefix(path, "/api/stars") || strings.HasPrefix(path, "/api/debug") {
			continue
		}
		if !strings.Contains(path, "player=alien") {
			t.Errorf("URL %q missing ?player=alien", path)
		}
	}
}

// TestClient_GetStars_NoPlayer verifies that GetStars does not append
// ?player (it is exempt from the middleware). (FR-39)
func TestClient_GetStars_NoPlayer(t *testing.T) {
	var starsURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		starsURL = r.URL.String()
		json.NewEncoder(w).Encode([]Star{})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "alien")
	c.GetStars(context.Background())
	if strings.Contains(starsURL, "player=") {
		t.Errorf("/api/stars should not include ?player, got %q", starsURL)
	}
}
