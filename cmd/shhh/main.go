package main

import (
	"context"
	"fmt"
	"os"

	"github.com/en9inerd/shhh/internal/config"
	"github.com/en9inerd/shhh/internal/log"
)

const version = "dev"

func run(ctx context.Context, args []string, getenv func(string) string) error {
	return nil
}

func main() {
	cleanedArgs, verbose := cleanArgs(os.Args[1:])

	logger := log.NewLogger(verbose)
	cfg, err := config.ParseConfig(os.Args, os.Getenv)
	if err != nil {
		fmt.Errorf("")
	}

	fmt.Println(cfg)

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
