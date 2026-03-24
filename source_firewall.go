package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// firewallEventsAdaptive — individual WAF/firewall events, 24h retention on free.
const firewallQuery = `
query FirewallEvents($zoneTag: string, $since: Time, $limit: int) {
  viewer {
    zones(filter: { zoneTag: $zoneTag }) {
      firewallEventsAdaptive(
        filter: { datetime_geq: $since }
        limit: $limit
        orderBy: [datetime_ASC]
      ) {
        action
        clientASNDescription
        clientCountryName
        clientIP
        clientRequestHTTPHost
        clientRequestHTTPMethodName
        clientRequestPath
        clientRequestQuery
        datetime
        edgeColoName
        edgeResponseStatus
        kind
        ruleId
        rayName
        source
        userAgent
      }
    }
  }
}`

type firewallSource struct {
	zoneID string
	gql    *graphqlClient
}

// newFirewallSource polls firewallEventsAdaptive for the given zone.
func newFirewallSource(zoneID string, gql *graphqlClient) Source {
	return &firewallSource{zoneID: zoneID, gql: gql}
}

func (s *firewallSource) Name() string { return "firewall:" + s.zoneID }

type firewallEvent struct {
	Datetime                time.Time `json:"datetime"`
	Action                  string    `json:"action"`
	ClientASNDescription    string    `json:"clientASNDescription"`
	ClientCountryName       string    `json:"clientCountryName"`
	ClientIP                string    `json:"clientIP"`
	ClientRequestHTTPHost   string    `json:"clientRequestHTTPHost"`
	ClientRequestHTTPMethod string    `json:"clientRequestHTTPMethodName"`
	ClientRequestPath       string    `json:"clientRequestPath"`
	ClientRequestQuery      string    `json:"clientRequestQuery"`
	EdgeColoName            string    `json:"edgeColoName"`
	EdgeResponseStatus      int       `json:"edgeResponseStatus"`
	Kind                    string    `json:"kind"`
	RuleID                  string    `json:"ruleId"`
	RayName                 string    `json:"rayName"`
	Source                  string    `json:"source"`
	UserAgent               string    `json:"userAgent"`
}

func (s *firewallSource) Poll(ctx context.Context, since time.Time) ([]Event, time.Time, error) {
	data, err := s.gql.Query(ctx, firewallQuery, map[string]any{
		"zoneTag": s.zoneID,
		"since":   since.UTC().Format(time.RFC3339),
		"limit":   10000,
	})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("graphql: %w", err)
	}

	var result struct {
		Viewer struct {
			Zones []struct {
				FirewallEventsAdaptive []firewallEvent `json:"firewallEventsAdaptive"`
			} `json:"zones"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal: %w", err)
	}
	if len(result.Viewer.Zones) == 0 {
		return nil, time.Now(), nil
	}

	raw := result.Viewer.Zones[0].FirewallEventsAdaptive
	events := make([]Event, 0, len(raw))
	var last time.Time

	for _, fe := range raw {
		b, err := json.Marshal(fe)
		if err != nil {
			continue
		}
		events = append(events, Event{
			TS:     fe.Datetime,
			Source: "firewall",
			Zone:   s.zoneID,
			Data:   b,
		})
		if fe.Datetime.After(last) {
			last = fe.Datetime
		}
	}

	// Advance cursor by 1ns to exclude the last seen event on next poll.
	next := time.Now()
	if !last.IsZero() {
		next = last.Add(time.Nanosecond)
	}
	return events, next, nil
}
