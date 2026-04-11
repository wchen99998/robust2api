package main

import (
	_ "embed"
	"flag"
	"log"
	"strings"

	_ "github.com/wchen99998/robust2api/ent/runtime"
	gatewayapp "github.com/wchen99998/robust2api/internal/app/gateway"
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
		log.Printf("Robust2API Gateway %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	if err := gatewayapp.Run(); err != nil {
		log.Fatalf("Failed to run gateway: %v", err)
	}
}
