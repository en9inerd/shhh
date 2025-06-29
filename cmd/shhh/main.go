package main

import (
	"os"

	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/log"
)

const version = "dev"

func main() {
	cleanedArgs, verbose := cleanArgs(os.Args[1:])

	logger := log.NewLogger(verbose)
	logger.Debug("Starting command", "args", cleanedArgs)
	cfg := config.LoadConfig(logger)
}

func cleanArgs(args []string) (cleanArgs []string, verbose bool) {
	for _, arg := range args {
		if arg == "--verbose" {
			verbose = true
		} else {
			cleanArgs = append(cleanArgs, arg)
		}
	}
	return
}
