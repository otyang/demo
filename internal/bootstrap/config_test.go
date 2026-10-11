package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otyang/demo/pkg/config"
	"github.com/otyang/demo/pkg/store"
)

// loadConfig loads AppConfig from env alone, with an empty env file and
// every other DEMO_ variable cleared.
func loadConfig(t *testing.T, env map[string]string) (AppConfig, error) {
	t.Helper()
	for _, name := range []string{
		"DEMO_APP_NAME", "DEMO_DEBUG", "DEMO_LOG_LEVEL", "DEMO_HOST", "DEMO_CORS_ALLOWED_ORIGINS",
		"DEMO_DB_TYPE", "DEMO_DB_URL", "DEMO_DB_TIMEOUT", "DEMO_DB_POOL_SIZE",
		"DEMO_AUTH_JWKS_URL", "DEMO_AUTH_ISSUER", "DEMO_AUTH_AUDIENCE",
		"DEMO_AUTH_TEST_CLAIMS", "DEMO_AUTH_TEST_ORG_ID", "DEMO_AUTH_TEST_PERMISSIONS",
	} {
		t.Setenv(name, "") // restored after the test
		require.NoError(t, os.Unsetenv(name))
	}
	for name, value := range env {
		t.Setenv(name, value)
	}
	envFile := filepath.Join(t.TempDir(), "empty.env")
	require.NoError(t, os.WriteFile(envFile, nil, 0o600))
	return config.Load[AppConfig](EnvPrefix, envFile)
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := loadConfig(t, map[string]string{"DEMO_DB_URL": "file:test.db"})
	require.NoError(t, err)

	assert.Equal(t, "demo-api", cfg.AppName)
	assert.False(t, cfg.Debug)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "localhost:3000", cfg.Host)
	assert.Equal(t, store.SQLite, cfg.Database.Type)
	assert.Equal(t, 5*time.Second, cfg.Database.Timeout)
	assert.Equal(t, 10, cfg.Database.PoolSize)
	assert.False(t, cfg.Auth.TestClaims)
	assert.Equal(t, "org_demo", cfg.Auth.TestOrgID)
}

func TestLoadRequiresTheDatabaseURL(t *testing.T) {
	_, err := loadConfig(t, nil)
	assert.ErrorContains(t, err, "required key URL")
}

func TestLoadSplitsLists(t *testing.T) {
	cfg, err := loadConfig(t, map[string]string{
		"DEMO_DB_URL":                "file:test.db",
		"DEMO_CORS_ALLOWED_ORIGINS":  "http://a.test,http://b.test",
		"DEMO_AUTH_TEST_PERMISSIONS": "tasks:read,tasks:write",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"http://a.test", "http://b.test"}, cfg.CORSAllowedOrigins)
	assert.Equal(t, []string{"tasks:read", "tasks:write"}, cfg.Auth.TestPermissions)
}

func TestValidateConfig(t *testing.T) {
	jwt := map[string]string{
		"DEMO_DB_URL":        "file:test.db",
		"DEMO_AUTH_JWKS_URL": "https://issuer.test/jwks",
		"DEMO_AUTH_ISSUER":   "https://issuer.test",
		"DEMO_AUTH_AUDIENCE": "demo",
	}
	with := func(changes map[string]string) map[string]string {
		env := map[string]string{}
		for k, v := range jwt {
			env[k] = v
		}
		for k, v := range changes {
			env[k] = v
		}
		return env
	}

	for name, test := range map[string]struct {
		env  map[string]string
		want string // "" means valid
	}{
		"jwt settings":              {jwt, ""},
		"missing issuer":            {with(map[string]string{"DEMO_AUTH_ISSUER": ""}), "DEMO_AUTH_ISSUER"},
		"missing audience":          {with(map[string]string{"DEMO_AUTH_AUDIENCE": ""}), "DEMO_AUTH_AUDIENCE"},
		"test claims in debug":      {map[string]string{"DEMO_DB_URL": "x", "DEMO_DEBUG": "true", "DEMO_AUTH_TEST_CLAIMS": "true"}, ""},
		"test claims without debug": {map[string]string{"DEMO_DB_URL": "x", "DEMO_AUTH_TEST_CLAIMS": "true"}, "requires DEMO_DEBUG"},
		"unknown database":          {with(map[string]string{"DEMO_DB_TYPE": "mysql"}), "DEMO_DB_TYPE"},
		"postgres pool of one":      {with(map[string]string{"DEMO_DB_TYPE": "postgres", "DEMO_DB_POOL_SIZE": "1"}), "DEMO_DB_POOL_SIZE"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := loadConfig(t, test.env)
			require.NoError(t, err)
			err = validateConfig(&cfg)
			if test.want == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, test.want)
			}
		})
	}
}
