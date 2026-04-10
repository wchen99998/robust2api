package bootstrap

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	bootstraprunner "github.com/Wei-Shaw/sub2api/internal/bootstrap"
)

func Run() error {
	log.Println("[bootstrap] starting sub2api-bootstrap")

	env := bootstraprunner.LoadBootstrapEnv()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[bootstrap] received signal %s, cancelling...", sig)
		cancel()
	}()

	if err := bootstraprunner.Run(ctx, env); err != nil {
		return err
	}

	log.Println("[bootstrap] completed successfully")
	return nil
}
