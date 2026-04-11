package control

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	platformconfig "github.com/wchen99998/robust2api/internal/platform/config"
	platformlogging "github.com/wchen99998/robust2api/internal/platform/logging"
)

type BuildInfo struct {
	Version   string
	BuildType string
}

func Run(buildInfo BuildInfo) error {
	platformlogging.InitBootstrap()
	defer platformlogging.Sync()

	cfg, err := platformconfig.LoadControl()
	if err != nil {
		return err
	}
	if err := platformlogging.Init(cfg.Log); err != nil {
		return err
	}
	if cfg.RunMode == "simple" {
		log.Println("WARNING: Running in SIMPLE mode - billing and quota checks are DISABLED")
	}

	app, err := initialize(buildInfo)
	if err != nil {
		return err
	}
	defer app.Cleanup()

	app.Health.SetReady()

	go func() {
		if err := app.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start control server: %v", err)
		}
	}()

	if app.MetricsServer != nil {
		go func() {
			if err := app.MetricsServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("Metrics server error: %v", err)
			}
		}()
	}

	log.Printf("Control server started on %s", app.Server.Addr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down control server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Server.Shutdown(ctx); err != nil {
		return err
	}

	if app.MetricsServer != nil {
		if err := app.MetricsServer.Shutdown(ctx); err != nil {
			log.Printf("Metrics server forced to shutdown: %v", err)
		}
	}

	log.Println("Control server exited")
	return nil
}
