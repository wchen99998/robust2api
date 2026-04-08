package main

import (
	"context"
	_ "embed"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/bootstrap"
)

//go:embed VERSION
var embeddedVersion string

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
	log.Println("[bootstrap] starting sub2api-bootstrap")

	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API Bootstrap %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

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
