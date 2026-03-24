package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	state, err := loadState(cfg.StateFile)
	if err != nil {
		slog.Warn("could not load state, starting fresh", "err", err)
		state = newState()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	gql := newGraphQLClient(cfg)
	sources := buildSources(cfg, gql)

	if len(sources) == 0 {
		slog.Error("no sources configured; set CF_ZONE_IDS and/or CF_SOURCES")
		os.Exit(1)
	}

	events := make(chan Event, 512)

	var wg sync.WaitGroup
	for _, src := range sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			runPoller(ctx, s, state, cfg.PollInterval, events)
		}(src)
	}

	// Close events channel once all pollers exit.
	go func() {
		wg.Wait()
		close(events)
	}()

	// Persist state every 30s.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := state.Save(cfg.StateFile); err != nil {
					slog.Warn("save state", "err", err)
				}
			}
		}
	}()

	// Output loop: JSON lines to stdout, logs to stderr.
	enc := json.NewEncoder(os.Stdout)
	for e := range events {
		if err := enc.Encode(e); err != nil {
			slog.Error("encode", "err", err)
		}
	}

	if err := state.Save(cfg.StateFile); err != nil {
		slog.Warn("final save state", "err", err)
	}
}

func buildSources(cfg *Config, gql *graphqlClient) []Source {
	enabled := make(map[string]bool, len(cfg.Sources))
	for _, s := range cfg.Sources {
		enabled[strings.TrimSpace(s)] = true
	}

	var sources []Source
	for _, zoneID := range cfg.ZoneIDs {
		zoneID = strings.TrimSpace(zoneID)
		if enabled["firewall"] {
			sources = append(sources, newFirewallSource(zoneID, gql))
		}
		if enabled["http"] {
			sources = append(sources, newHTTPSource(zoneID, gql))
		}
		if enabled["dns"] {
			sources = append(sources, newDNSSource(zoneID, gql))
		}
	}
	if enabled["audit"] {
		sources = append(sources, newAuditSource(cfg))
	}
	if enabled["zt_access"] {
		sources = append(sources, newZTAccessSource(cfg))
	}
	if enabled["zt_gateway_dns"] {
		sources = append(sources, newZTGatewayDNSSource(cfg, gql))
	}
	if enabled["zt_gateway_http"] {
		sources = append(sources, newZTGatewayHTTPSource(cfg, gql))
	}
	if enabled["zt_gateway_l4"] {
		sources = append(sources, newZTGatewayL4Source(cfg, gql))
	}
	return sources
}
