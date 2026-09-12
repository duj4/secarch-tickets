package main

import (
	"flag"
	"os"

	"secarch-tickets/internal/appconfig"
	"secarch-tickets/internal/logger"
	"secarch-tickets/web"
)

func main() {
	configPath := flag.String("config", appconfig.DefaultPath, "path to the application JSON configuration")
	flag.Parse()
	// Initialize logging before starting the web service.
	logger.Init()

	if err := web.Run(*configPath); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}
