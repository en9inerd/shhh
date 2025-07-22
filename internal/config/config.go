package config

import (
	"flag"
)

type Config struct {
	Port string
}

func ParseConfig(args []string, getenv func(string) string) (*Config, error) {
	fs := flag.NewFlagSet("shhh", flag.ContinueOnError)

	port := fs.String("port", "8000", "Port number (0-65535)")

	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}

	portValue := *port
	if envPort := getenv("SHHH_PORT"); envPort != "" {
		portValue = envPort
	}

	return &Config{
		Port: portValue,
	}, nil
}
