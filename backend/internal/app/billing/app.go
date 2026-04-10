package billing

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	platformconfig "github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformlogging "github.com/Wei-Shaw/sub2api/internal/platform/logging"
)

func Run() error {
	platformlogging.InitBootstrap()
	defer platformlogging.Sync()

	cfg, err := platformconfig.LoadBilling()
	if err != nil {
		return err
	}
	if err := platformlogging.Init(cfg.Log); err != nil {
		return err
	}

	app, err := initialize()
	if err != nil {
		return err
	}

	app.Health.SetReady()

	healthPort := cfg.Billing.HealthPort
	if healthPort == "" {
		healthPort = "8082"
	}

	mux := http.NewServeMux()
	app.Health.RegisterOnMux(mux)
	healthServer := &http.Server{
		Addr:    fmt.Sprintf(":%s", healthPort),
		Handler: mux,
	}

	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Health server error: %v", err)
		}
	}()

	log.Printf("Billing service started (health server on :%s)", healthPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down billing service...")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	app.Cleanup()

	if err := healthServer.Shutdown(ctx); err != nil {
		log.Printf("Health server forced to shutdown: %v", err)
	}

	log.Println("Billing service exited")
	return nil
}
