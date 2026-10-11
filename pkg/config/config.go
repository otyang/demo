// Package config loads configuration values from environment variables and
// optional dotenv files.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Load reads environment variables and dotenv files into a value of T.
// Defaults and required fields are declared with envconfig struct tags.
// At least one file path must be provided. Existing process environment
// variables take precedence over values in dotenv files.
func Load[T any](prefix string, files ...string) (T, error) {
	var zero T

	if len(files) == 0 {
		return zero, fmt.Errorf("at least one environment file path is required")
	}

	existingFiles := make([]string, 0, len(files))
	for _, path := range files {
		if _, err := os.Stat(path); err != nil {
			return zero, fmt.Errorf("stat environment file %q: %w", path, err)
		}
		existingFiles = append(existingFiles, path)
	}
	if len(existingFiles) > 0 {
		if err := godotenv.Load(existingFiles...); err != nil {
			return zero, fmt.Errorf("load environment files %q: %w", strings.Join(existingFiles, ", "), err)
		}
	}

	var config T
	if err := envconfig.Process(prefix, &config); err != nil {
		return zero, fmt.Errorf("process environment config: %w", err)
	}
	return config, nil
}
