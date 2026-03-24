package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Audit logs — account-level config changes. 18-month retention, all plans.
type auditSource struct {
	cfg  *Config
	http *http.Client
}

// newAuditSource polls account audit logs via the REST API.
func newAuditSource(cfg *Config) Source {
	return &auditSource{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *auditSource) Name() string { return "audit" }

func (s *auditSource) Poll(ctx context.Context, since time.Time) ([]Event, time.Time, error) {
	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/audit_logs",
		url.PathEscape(s.cfg.AccountID),
	)

	params := url.Values{}
	params.Set("since", since.UTC().Format(time.RFC3339))
	params.Set("per_page", "100")
	params.Set("direction", "asc")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("new request: %w", err)
	}
	s.cfg.SetAuth(req)

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("status %d", resp.StatusCode)
	}

	var payload struct {
		Result []json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, time.Time{}, fmt.Errorf("decode: %w", err)
	}

	events := make([]Event, 0, len(payload.Result))
	var last time.Time

	for _, raw := range payload.Result {
		var meta struct {
			When time.Time `json:"when"`
		}
		if err := json.Unmarshal(raw, &meta); err != nil || meta.When.IsZero() {
			continue
		}
		if meta.When.After(last) {
			last = meta.When
		}
		events = append(events, Event{
			TS:     meta.When,
			Source: "audit",
			Data:   raw,
		})
	}

	next := time.Now()
	if !last.IsZero() {
		// Advance by 1s — audit log timestamps are second-granularity.
		next = last.Add(time.Second)
	}
	return events, next, nil
}
