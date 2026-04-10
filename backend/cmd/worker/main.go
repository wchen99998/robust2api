package main

import (
	_ "embed"
	"flag"
	"log"
	"strings"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	workerapp "github.com/Wei-Shaw/sub2api/internal/app/worker"
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
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API Worker %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	if err := workerapp.Run(); err != nil {
		log.Fatalf("Failed to run worker: %v", err)
	}
}
