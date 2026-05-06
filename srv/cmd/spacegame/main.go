// Command spacegame is the SpaceGame server entry point.
// Run from the repository root so that nearest.csv and planets.csv are accessible.
//
// Usage:
//
//	cd /path/to/SpaceGame && go run ./srv/cmd/spacegame
package main

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gmofishsauce/SpaceGame/srv/internal/game"
	"github.com/gmofishsauce/SpaceGame/srv/internal/server"
)

func main() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received signal %s, exiting", sig)
		os.Exit(0)
	}()

	ctx := context.Background()

	// Resolve CSV paths relative to the working directory (project root).
	nearestCSV := envOrDefault("SPACEGAME_NEAREST_CSV", "nearest.csv")
	planetsCSV := envOrDefault("SPACEGAME_PLANETS_CSV", "planets.csv")
	alienNearestCSV := envOrDefault("SPACEGAME_ALIEN_NEAREST_CSV", "alien-nearest.csv")
	alienPlanetsCSV := envOrDefault("SPACEGAME_ALIEN_PLANETS_CSV", "alien-planets.csv")

	if info, err := os.Stat(os.Args[0]); err == nil {
		log.Printf("server v.%s started", info.ModTime().Format("20060102-150405"))
	} else {
		log.Printf("server v.unknown started")
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	state, err := game.Initialize(rng, nearestCSV, planetsCSV, alienNearestCSV, alienPlanetsCSV)
	if err != nil {
		log.Fatalf("initializing game state: %v", err)
	}
	log.Printf("loaded %d star systems", len(state.Catalog.Order))

	events := game.NewEventManager()
	engine := game.NewEngine(state, events, rng)

	go engine.Run(ctx)

	srv := server.New(engine, events, state)
	log.Printf("SpaceGame listening on http://127.0.0.1:8080")
	if err := srv.ListenAndServe(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
	log.Printf("SpaceGame server stopped")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
