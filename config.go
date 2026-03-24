package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config is loaded entirely from environment variables.
type Config struct {
	// Auth: either CF_API_TOKEN (Bearer) or CF_API_KEY + CF_AUTH_EMAIL (Global Key)
	APIToken  string // CF_API_TOKEN — scoped API token, used as Bearer
	APIKey    string // CF_API_KEY   — global API key (legacy)
	AuthEmail string // CF_AUTH_EMAIL — email paired with global API key

	AccountID    string
	ZoneIDs      []string
	PollInterval time.Duration
	StateFile    string
	Sources      []string // firewall, http, dns, audit
}

// SetAuth applies the right Cloudflare auth headers to a request.
func (c *Config) SetAuth(req *http.Request) {
	if c.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIToken)
	} else {
		req.Header.Set("X-Auth-Key", c.APIKey)
		req.Header.Set("X-Auth-Email", c.AuthEmail)
	}
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		APIToken:     os.Getenv("CF_API_TOKEN"),
		APIKey:       os.Getenv("CF_API_KEY"),
		AuthEmail:    os.Getenv("CF_AUTH_EMAIL"),
		AccountID:    os.Getenv("CF_ACCOUNT_ID"),
		PollInterval: 60 * time.Second,
		StateFile:    "/var/lib/cf-log-exporter/state.json",
		Sources:      []string{"firewall", "dns", "audit", "zt_access", "zt_gateway_dns", "zt_gateway_http", "zt_gateway_l4"},
	}

	// Accept CF_ACC_ID as fallback for auth email (existing dotfile convention).
	if cfg.AuthEmail == "" {
		cfg.AuthEmail = os.Getenv("CF_ACC_ID")
	}

	if cfg.APIToken == "" && cfg.APIKey == "" {
		return nil, fmt.Errorf("CF_API_TOKEN or CF_API_KEY is required")
	}
	if cfg.APIKey != "" && cfg.AuthEmail == "" {
		return nil, fmt.Errorf("CF_AUTH_EMAIL (or CF_ACC_ID) is required when using CF_API_KEY")
	}
	if cfg.AccountID == "" {
		return nil, fmt.Errorf("CF_ACCOUNT_ID is required")
	}

	if z := os.Getenv("CF_ZONE_IDS"); z != "" {
		for _, id := range strings.Split(z, ",") {
			if id = strings.TrimSpace(id); id != "" {
				cfg.ZoneIDs = append(cfg.ZoneIDs, id)
			}
		}
	}
	if p := os.Getenv("CF_POLL_INTERVAL"); p != "" {
		d, err := time.ParseDuration(p)
		if err != nil {
			return nil, fmt.Errorf("CF_POLL_INTERVAL: %w", err)
		}
		cfg.PollInterval = d
	}
	if s := os.Getenv("CF_STATE_FILE"); s != "" {
		cfg.StateFile = s
	}
	if s := os.Getenv("CF_SOURCES"); s != "" {
		cfg.Sources = strings.Split(s, ",")
	}
	return cfg, nil
}
