package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Zero Trust Access request logs — login/logout events per protected app.
// 24h retention on free tier.
type ztAccessSource struct {
	cfg  *Config
	http *http.Client
}

func newZTAccessSource(cfg *Config) Source {
	return &ztAccessSource{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *ztAccessSource) Name() string { return "zt_access" }

type ztAccessEvent struct {
	Action                      string    `json:"action"`
	Allowed                     bool      `json:"allowed"`
	AppDomain                   string    `json:"appDomain"`
	AppUUID                     string    `json:"appUUID"`
	Connection                  string    `json:"connection"`
	Country                     string    `json:"country"`
	CreatedAt                   time.Time `json:"createdAt"`
	Email                       string    `json:"email"`
	IPAddress                   string    `json:"ipAddress"`
	PurposeJustificationPrompt  string    `json:"purposeJustificationPrompt"`
	PurposeJustificationResponse string   `json:"purposeJustificationResponse"`
	RayID                       string    `json:"rayID"`
	UserUID                     string    `json:"userUID"`
}

func (s *ztAccessSource) Poll(ctx context.Context, since time.Time) ([]Event, time.Time, error) {
	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/access/logs/access_requests",
		url.PathEscape(s.cfg.AccountID),
	)

	params := url.Values{}
	params.Set("since", since.UTC().Format(time.RFC3339))
	params.Set("limit", "100")
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
		Result []ztAccessEvent `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, time.Time{}, fmt.Errorf("decode: %w", err)
	}

	events := make([]Event, 0, len(payload.Result))
	var last time.Time

	for _, e := range payload.Result {
		b, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if e.CreatedAt.After(last) {
			last = e.CreatedAt
		}
		events = append(events, Event{
			TS:     e.CreatedAt,
			Source: "zt_access",
			Data:   b,
		})
	}

	next := time.Now()
	if !last.IsZero() {
		next = last.Add(time.Second)
	}
	return events, next, nil
}
