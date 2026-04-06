package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Wei-Shaw/sub2api/internal/bootstrap"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
)

func main() {
	log.Println("[bootstrap] starting sub2api-bootstrap")

	env := bootstrap.LoadBootstrapEnv()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[bootstrap] received signal %s, cancelling...", sig)
		cancel()
	}()

	if err := bootstrap.Run(ctx, env); err != nil {
		log.Fatalf("[bootstrap] FAILED: %v", err)
	}

	log.Println("[bootstrap] completed successfully")
}
