package bootstrap

import (
	"time"

	"github.com/otyang/demo/pkg/store"
)

// EnvPrefix prefixes every configuration variable, such as DEMO_HOST.
const EnvPrefix = "DEMO"

// AppConfig is the application configuration, read from DEMO_* environment
// variables and the env file by config.Load.
type AppConfig struct {
	AppName  string `envconfig:"APP_NAME" default:"demo-api"`
	Debug    bool   `envconfig:"DEBUG" default:"false"`
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
	// Host is the address the API listens on.
	Host string `envconfig:"HOST" default:"localhost:3000"`
	// CORSAllowedOrigins lists the browser origins allowed to call the API
	// (comma-separated). With none, browsers cannot call it cross-origin.
	CORSAllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS"`

	Database struct {
		// Type is sqlite or postgres.
		Type     store.DatabaseType `envconfig:"TYPE" default:"sqlite"`
		URL      string             `envconfig:"URL" required:"true"`
		Timeout  time.Duration      `envconfig:"TIMEOUT" default:"5s"`
		PoolSize int                `envconfig:"POOL_SIZE" default:"10"`
	} `envconfig:"DB"`

	Auth struct {
		// JWKSURL, Issuer and Audience verify Bearer tokens. All three are
		// required unless TestClaims is enabled.
		JWKSURL  string `envconfig:"JWKS_URL"`
		Issuer   string `envconfig:"ISSUER"`
		Audience string `envconfig:"AUDIENCE"`
		// TestClaims replaces JWT verification with fixed claims, so the API
		// can be tried locally without an identity provider. It is refused
		// unless Debug is also enabled.
		TestClaims      bool     `envconfig:"TEST_CLAIMS" default:"false"`
		TestOrgID       string   `envconfig:"TEST_ORG_ID" default:"org_demo"`
		TestPermissions []string `envconfig:"TEST_PERMISSIONS"`
	} `envconfig:"AUTH"`
}
