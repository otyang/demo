package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndEnvironmentValues(t *testing.T) {
	type testConfig struct {
		Name    string `envconfig:"NAME" default:"default-name"`
		Enabled bool   `envconfig:"ENABLED" default:"false"`
	}

	envPath := writeEmptyEnvFile(t)
	unsetEnvironment(t, "NAME")
	unsetEnvironment(t, "ENABLED")

	cfg, err := Load[testConfig]("", envPath)
	if err != nil {
		t.Fatalf("Load[testConfig]() error = %v", err)
	}
	if cfg.Name != "default-name" || cfg.Enabled {
		t.Errorf("config = %+v, want tag defaults", cfg)
	}

	t.Setenv("NAME", "mailer")
	t.Setenv("ENABLED", "true")
	cfg, err = Load[testConfig]("", envPath)
	if err != nil {
		t.Fatalf("Load[testConfig]() error = %v", err)
	}
	if cfg.Name != "mailer" || !cfg.Enabled {
		t.Errorf("config = %+v, want environment values", cfg)
	}
}

func TestLoadDotEnvWithEnvironmentPrecedence(t *testing.T) {
	type testConfig struct {
		Name     string `envconfig:"NAME"`
		Endpoint string `envconfig:"ENDPOINT"`
	}

	envPath := filepath.Join(t.TempDir(), "config.env")
	content := []byte("NAME=from-file\nENDPOINT=https://file.example\n")
	if err := os.WriteFile(envPath, content, 0o600); err != nil {
		t.Fatalf("write dotenv file: %v", err)
	}
	unsetEnvironment(t, "NAME")
	unsetEnvironment(t, "ENDPOINT")
	t.Setenv("NAME", "from-environment")

	cfg, err := Load[testConfig]("", envPath)
	if err != nil {
		t.Fatalf("Load[testConfig]() error = %v", err)
	}
	if cfg.Name != "from-environment" {
		t.Errorf("Name = %q, want environment value", cfg.Name)
	}
	if cfg.Endpoint != "https://file.example" {
		t.Errorf("Endpoint = %q, want dotenv value", cfg.Endpoint)
	}
}

func TestLoadRequiresAtLeastOneFile(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "existing.env"))

	if _, err := Load[struct{}](""); err == nil {
		t.Fatal("Load[struct{}]() error = nil, want file path required error")
	}
}

func TestLoadReturnsZeroValueOnParseError(t *testing.T) {
	type testConfig struct {
		Name    string `envconfig:"NAME"`
		Enabled bool   `envconfig:"ENABLED"`
	}

	envPath := writeEmptyEnvFile(t)
	t.Setenv("NAME", "mailer")
	t.Setenv("ENABLED", "not-a-bool")

	cfg, err := Load[testConfig]("", envPath)
	if err == nil {
		t.Fatal("Load[testConfig]() error = nil, want parse error")
	}
	if cfg != (testConfig{}) {
		t.Errorf("config = %+v, want zero value on error", cfg)
	}
}

func TestLoadWithPrefix(t *testing.T) {
	type testConfig struct {
		Name string `envconfig:"NAME"`
	}

	envPath := writeEmptyEnvFile(t)
	unsetEnvironment(t, "NAME")
	unsetEnvironment(t, "MAILER_NAME")
	t.Setenv("MAILER_NAME", "prefixed-mailer")

	cfg, err := Load[testConfig]("MAILER", envPath)
	if err != nil {
		t.Fatalf("Load[testConfig]() error = %v", err)
	}
	if cfg.Name != "prefixed-mailer" {
		t.Errorf("Name = %q, want prefixed environment value", cfg.Name)
	}
}

func TestLoadMultipleFilesAndPrecedence(t *testing.T) {
	type testConfig struct {
		Name     string `envconfig:"NAME"`
		Endpoint string `envconfig:"ENDPOINT"`
	}

	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.env")
	overridePath := filepath.Join(dir, "override.env")
	if err := os.WriteFile(basePath, []byte("NAME=from-base\nENDPOINT=https://base.example\n"), 0o600); err != nil {
		t.Fatalf("write base dotenv file: %v", err)
	}
	if err := os.WriteFile(overridePath, []byte("NAME=from-override\n"), 0o600); err != nil {
		t.Fatalf("write override dotenv file: %v", err)
	}
	unsetEnvironment(t, "NAME")
	unsetEnvironment(t, "ENDPOINT")

	cfg, err := Load[testConfig]("", basePath, overridePath)
	if err != nil {
		t.Fatalf("Load[testConfig]() error = %v", err)
	}
	if cfg.Name != "from-base" {
		t.Errorf("Name = %q, want first dotenv file value", cfg.Name)
	}
	if cfg.Endpoint != "https://base.example" {
		t.Errorf("Endpoint = %q, want base dotenv file value", cfg.Endpoint)
	}
}

func TestLoadMultipleFilesEnvironmentTakesPrecedence(t *testing.T) {
	type testConfig struct {
		Name string `envconfig:"NAME"`
	}

	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.env")
	secondPath := filepath.Join(dir, "second.env")
	for path, value := range map[string]string{
		firstPath:  "NAME=from-first\n",
		secondPath: "NAME=from-second\n",
	} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatalf("write dotenv file %q: %v", path, err)
		}
	}
	unsetEnvironment(t, "NAME")
	t.Setenv("NAME", "from-process")

	cfg, err := Load[testConfig]("", firstPath, secondPath)
	if err != nil {
		t.Fatalf("Load[testConfig]() error = %v", err)
	}
	if cfg.Name != "from-process" {
		t.Errorf("Name = %q, want process environment value", cfg.Name)
	}
}

func TestLoadRejectsMissingConfiguredFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.env")
	if _, err := Load[struct{}]("", path); err == nil {
		t.Fatal("Load[struct{}]() error = nil, want missing configured file error")
	}
}

func unsetEnvironment(t *testing.T, name string) {
	t.Helper()
	value, wasSet := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}

	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func writeEmptyEnvFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write empty env file: %v", err)
	}
	return path
}
