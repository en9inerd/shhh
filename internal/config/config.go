package config

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port string
}

func LoadConfig(logger *slog.Logger) *Config {
	cfg := &Config{}

	// Try loading from config paths
	for _, path := range configPaths() {
		if err := parseConfig(path, cfg); err == nil {
			logger.Debug("Loaded config file", "file", path)
			break
		}
	}

	// Override from environment variables
	if v := os.Getenv("SHHH_PORT"); v != "" {
		cfg.Port = v
	}

	return cfg
}

func configPaths() []string {
	var paths []string

	// User config dir
	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			xdgConfigHome = filepath.Join(home, ".config")
		}
	}
	if xdgConfigHome != "" {
		paths = append(paths, filepath.Join(xdgConfigHome, "shhh", "config"))
	}

	// Optional local fallback
	paths = append(paths, "config")

	return paths
}

// Parse ini-like config file
func parseConfig(path string, cfg *Config) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip full-line comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip inline comments only if preceded by whitespace
		if idx := strings.Index(line, "#"); idx != -1 {
			if idx == 0 || (idx > 0 && line[idx-1] == ' ') {
				line = strings.TrimSpace(line[:idx])
			}
		}

		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "port":
			cfg.Port = value
		}
	}

	return scanner.Err()
}

// SaveConfig writes the current configuration to the first available config path.
func SaveConfig(cfg *Config, logger *slog.Logger) error {
	paths := configPaths()
	if len(paths) == 0 {
		return os.ErrInvalid
	}

	configPath := paths[0] // Save to the highest-priority path

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	lines := []string{
		"# shhh configuration file",
		"",
		"port = " + cfg.Port,
	}

	for _, line := range lines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return err
		}
	}

	if err := writer.Flush(); err != nil {
		return err
	}

	logger.Debug("Saved config file", "file", configPath)
	return nil
}
