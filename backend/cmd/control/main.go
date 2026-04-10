package main

import (
	_ "embed"
	"flag"
	"log"
	"strings"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	controlapp "github.com/Wei-Shaw/sub2api/internal/app/control"
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
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API Control %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	if err := controlapp.Run(controlapp.BuildInfo{
		Version:   Version,
		BuildType: BuildType,
	}); err != nil {
		log.Fatalf("Failed to run control: %v", err)
	}
}
