package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Zero Trust Gateway L4 — TCP/UDP network sessions through Cloudflare Gateway.
const ztGatewayL4Query = `
query GatewayL4($accountTag: string, $since: Time, $limit: int) {
  viewer {
    accounts(filter: { accountTag: $accountTag }) {
      gatewayL4SessionsAdaptiveGroups(
        filter: { datetime_geq: $since }
        limit: $limit
        orderBy: [datetime_ASC]
      ) {
        count
        dimensions {
          datetime
          destinationIp
          destinationPort
          action
          email
          sourceIp
          transport
          dstIpCountry
        }
      }
    }
  }
}`

type ztGatewayL4Source struct {
	cfg *Config
	gql *graphqlClient
}

func newZTGatewayL4Source(cfg *Config, gql *graphqlClient) Source {
	return &ztGatewayL4Source{cfg: cfg, gql: gql}
}

func (s *ztGatewayL4Source) Name() string { return "zt_gateway_l4" }

type ztGatewayL4Group struct {
	Count      int64 `json:"count"`
	Dimensions struct {
		Datetime        time.Time `json:"datetime"`
		DestinationIp   string    `json:"destinationIp"`
		DestinationPort int       `json:"destinationPort"`
		Action          string    `json:"action"`
		Email           string    `json:"email"`
		SourceIp        string    `json:"sourceIp"`
		Transport       string    `json:"transport"`
		DstIpCountry    string    `json:"dstIpCountry"`
	} `json:"dimensions"`
}

func (s *ztGatewayL4Source) Poll(ctx context.Context, since time.Time) ([]Event, time.Time, error) {
	data, err := s.gql.Query(ctx, ztGatewayL4Query, map[string]any{
		"accountTag": s.cfg.AccountID,
		"since":      since.UTC().Format(time.RFC3339),
		"limit":      10000,
	})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("graphql: %w", err)
	}

	var result struct {
		Viewer struct {
			Accounts []struct {
				GatewayL4SessionsAdaptiveGroups []ztGatewayL4Group `json:"gatewayL4SessionsAdaptiveGroups"`
			} `json:"accounts"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal: %w", err)
	}
	if len(result.Viewer.Accounts) == 0 {
		return nil, time.Now(), nil
	}

	groups := result.Viewer.Accounts[0].GatewayL4SessionsAdaptiveGroups
	events := make([]Event, 0, len(groups))
	var last time.Time

	for _, g := range groups {
		b, err := json.Marshal(g)
		if err != nil {
			continue
		}
		if g.Dimensions.Datetime.After(last) {
			last = g.Dimensions.Datetime
		}
		events = append(events, Event{
			TS:     g.Dimensions.Datetime,
			Source: "zt_gateway_l4",
			Data:   b,
		})
	}

	next := time.Now()
	if !last.IsZero() {
		next = last.Add(time.Nanosecond)
	}
	return events, next, nil
}
