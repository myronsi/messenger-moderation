package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func valid() map[string]string {
	return map[string]string{
		"DATABASE_URL":   "postgres://moderation:pw@localhost:5432/moderation",
		"ENCRYPTION_KEY": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))),
	}
}

func TestDefaults(t *testing.T) {
	cfg, err := LoadFrom(valid())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8090" || cfg.SessionTTL != 12*time.Hour || cfg.Production() || len(cfg.Key()) != 32 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]map[string]string{
		"ENCRYPTION_KEY":           {"ENCRYPTION_KEY": "short"},
		"DATABASE_URL":             {"DATABASE_URL": "mysql://x"},
		"PUBLIC_ORIGIN":            {"PUBLIC_ORIGIN": "https://example.com/path"},
		"must use https":           {"APP_ENV": "production", "PUBLIC_ORIGIN": "http://moderation.example.com"},
		"STAFF_SESSION_TTL":        {"STAFF_SESSION_TTL": "48h"},
		"MESSENGER_INTERNAL_TOKEN": {"MESSENGER_INTERNAL_URL": "http://api:8082", "MESSENGER_INTERNAL_TOKEN": "short"},
	}
	for want, override := range cases {
		vars := valid()
		for k, v := range override {
			vars[k] = v
		}
		if _, err := LoadFrom(vars); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", want, err)
		}
	}
}
