package main

import (
	_ "embed"
	"flag"
	"log"
	"strings"

	_ "github.com/wchen99998/robust2api/ent/runtime"
	bootstrapapp "github.com/wchen99998/robust2api/internal/app/bootstrap"
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
		log.Printf("Robust2API Bootstrap %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	if err := bootstrapapp.Run(); err != nil {
		log.Fatalf("[bootstrap] FAILED: %v", err)
	}
}
