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
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

	if info, err := os.Stat(os.Args[0]); err == nil {
		log.Printf("server v.%s started", info.ModTime().Format("20060102-150405"))
	} else {
		log.Printf("server v.unknown started")
	}

	state, err := game.Initialize(nearestCSV, planetsCSV)
	if err != nil {
		log.Fatalf("initializing game state: %v", err)
	}
	log.Printf("loaded %d star systems", len(state.Systems))

	events := game.NewEventManager()
	bot := game.NewDefaultBot()
	engine := game.NewEngine(state, bot, events)

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
