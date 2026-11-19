package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Env           string        `env:"APP_ENV" envDefault:"development"`
	LogLevel      string        `env:"LOG_LEVEL" envDefault:"info"`
	Addr          string        `env:"HTTP_ADDR" envDefault:":8090"`
	PublicOrigin  string        `env:"PUBLIC_ORIGIN" envDefault:"http://localhost:8090"`
	DatabaseURL   string        `env:"DATABASE_URL,required,notEmpty"`
	EncryptionKey string        `env:"ENCRYPTION_KEY,required,notEmpty"`
	SessionTTL    time.Duration `env:"STAFF_SESSION_TTL" envDefault:"12h"`
	ConsoleDir    string        `env:"CONSOLE_DIR"`
	TrustProxy    bool          `env:"TRUST_PROXY" envDefault:"false"`
	Messenger     Messenger
}

type Messenger struct {
	InternalURL     string `env:"MESSENGER_INTERNAL_URL"`
	InternalToken   string `env:"MESSENGER_INTERNAL_TOKEN"`
	ReadDatabaseURL string `env:"MESSENGER_READ_DATABASE_URL"`
}

func Load() (Config, error) {
	return LoadFrom(environ())
}

func LoadFrom(vars map[string]string) (Config, error) {
	var c Config
	if err := env.ParseWithOptions(&c, env.Options{Environment: vars}); err != nil {
		return c, err
	}
	return c, c.validate()
}

func (c Config) Production() bool { return c.Env == "production" }

func (c Config) Key() []byte {
	key, _ := base64.StdEncoding.DecodeString(c.EncryptionKey)
	return key
}

func (c Config) validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	switch c.Env {
	case "development", "test", "production":
	default:
		add("APP_ENV must be development, test or production")
	}
	if key, err := base64.StdEncoding.DecodeString(c.EncryptionKey); err != nil || len(key) != 32 {
		add("ENCRYPTION_KEY must be 32 random bytes, base64 encoded (openssl rand -base64 32)")
	}
	if !strings.HasPrefix(c.DatabaseURL, "postgres://") && !strings.HasPrefix(c.DatabaseURL, "postgresql://") {
		add("DATABASE_URL must be a postgres:// URL")
	}
	origin, err := url.Parse(c.PublicOrigin)
	if err != nil || origin.Host == "" || origin.Path != "" || (origin.Scheme != "http" && origin.Scheme != "https") {
		add("PUBLIC_ORIGIN must be an origin such as https://moderation.example.com")
	} else if c.Production() && origin.Scheme != "https" {
		add("PUBLIC_ORIGIN must use https in production")
	}
	if c.SessionTTL < time.Minute || c.SessionTTL > 24*time.Hour {
		add("STAFF_SESSION_TTL must be between 1m and 24h")
	}
	if c.Messenger.InternalURL != "" && len(c.Messenger.InternalToken) < 32 {
		add("MESSENGER_INTERNAL_TOKEN must be at least 32 characters when MESSENGER_INTERNAL_URL is set")
	}
	if len(problems) > 0 {
		return errors.New("invalid configuration: " + strings.Join(problems, "; "))
	}
	return nil
}

func environ() map[string]string {
	vars := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			vars[k] = v
		}
	}
	return vars
}
