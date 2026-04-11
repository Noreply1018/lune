package main

import (
	"log"
	"os"

	"lune/internal/app"
)

func main() {
	logger := log.New(os.Stdout, "[lune] ", log.LstdFlags|log.Lshortfile)

	if err := app.Run(logger); err != nil {
		logger.Fatalf("server stopped: %v", err)
	}
}
