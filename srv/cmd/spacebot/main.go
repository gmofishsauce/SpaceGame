// spacebot is the alien bot player for SpaceGame. It connects to a running
// spacegame server over the public HTTP/SSE API. (FR-61, FR-63, FR-68)
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gmofishsauce/SpaceGame/srv/internal/bot"
)

func main() {
	serverURL := flag.String("server", "http://127.0.0.1:8080", "base URL of the spacegame server")
	debugTruth := flag.Bool("debug-truth", false, "periodically log /api/debug/state (does not affect decisions)")
	logPath := flag.String("log", "", "log file path (default: stderr)")
	flag.Parse()

	logger := log.New(os.Stderr, "spacebot: ", log.LstdFlags)
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logger.Fatalf("open log file: %v", err)
		}
		defer f.Close()
		logger = log.New(f, "spacebot: ", log.LstdFlags)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := bot.NewClient(*serverURL, "alien")
	local := bot.NewLocal(client)
	agent := bot.NewAgent(client, local, *debugTruth, logger)

	// SSE keeps Local fresh; Agent decides commands on a cadence.
	errCh := make(chan error, 1)
	go func() {
		if err := local.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Printf("SSE stream dropped: %v", err)
			errCh <- err
		}
	}()

	go func() {
		select {
		case <-ctx.Done():
		case err := <-errCh:
			logger.Printf("exiting due to stream error: %v", err)
			cancel()
		}
	}()

	if err := agent.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Printf("agent exited: %v", err)
		os.Exit(1)
	}
}
