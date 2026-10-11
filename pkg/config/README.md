# Configuration

The `config` package loads a struct from environment variables with
`github.com/kelseyhightower/envconfig` and loads explicitly provided dotenv files with
`github.com/joho/godotenv`.

```go
import "github.com/otyang/demo/pkg/config"

type MailerConfig struct {
	Host string `envconfig:"HOST" default:"localhost"`
	Port int    `envconfig:"PORT" default:"1025"`
}

cfg, err := config.Load[MailerConfig](
	"MAILER",
	"config/base.env",
	"config/local.env",
)
if err != nil {
	return err
}
```

The first argument is the optional prefix passed to `envconfig.Process`; in
the example, the fields read `MAILER_HOST` and `MAILER_PORT`. Following
arguments are dotenv paths. At least one path is required. Files are loaded in
the listed order, and the first file that defines a variable wins over later
files. Variables already set in the process environment take precedence over
all dotenv values.

The loader does not implicitly read `.env` or `ENV_FILE`; callers should
explicitly pass the desired file paths. Every provided file must exist.

The loader returns `T` by value and returns its zero value with an error.
