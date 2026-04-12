package main

import (
	"fmt"
	"os"

	"lune/internal/app"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "up":
		if err := cmdUp(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		cmdVersion()
	case "check":
		if err := cmdCheck(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nUsage: lune [up|version|check]\n", cmd)
		os.Exit(1)
	}
}

func cmdUp() error {
	cfg := app.LoadConfig()
	a, err := app.New(cfg)
	if err != nil {
		return err
	}
	return a.Run()
}

func cmdVersion() {
	fmt.Printf("lune %s (commit %s, built %s)\n", version, commit, date)
}

func cmdCheck() error {
	cfg := app.LoadConfig()
	return app.Check(cfg)
}
