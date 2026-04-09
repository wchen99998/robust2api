package main

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

//go:embed VERSION
var embeddedVersion string

// Build-time variables (can be set by ldflags)
var (
	Version   = ""
	Commit    = "unknown"
	Date      = "unknown"
	BuildType = "source"
)

func init() {
	if strings.TrimSpace(Version) != "" {
		return
	}
	Version = strings.TrimSpace(embeddedVersion)
	if Version == "" {
		Version = "0.0.0-dev"
	}
}

func main() {
	logger.InitBootstrap()
	defer logger.Sync()

	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API Billing %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	runBilling()
}

func runBilling() {
	cfg, err := config.LoadBilling()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := logger.Init(logger.OptionsFromConfig(cfg.Log)); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	app, err := initializeBillingApplication()
	if err != nil {
		log.Fatalf("Failed to initialize billing application: %v", err)
	}

	// Mark as ready after successful initialization.
	app.Health.SetReady()

	// Start internal health HTTP server.
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

	// Wait for interrupt signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down billing service...")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Stop all background services.
	app.Cleanup()

	// Shut down the health server last.
	if err := healthServer.Shutdown(ctx); err != nil {
		log.Printf("Health server forced to shutdown: %v", err)
	}

	log.Println("Billing service exited")
}
