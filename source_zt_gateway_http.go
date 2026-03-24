package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Zero Trust Gateway HTTP — web requests inspected by Cloudflare Gateway.
const ztGatewayHTTPQuery = `
query GatewayHTTP($accountTag: string, $since: Time, $limit: int) {
  viewer {
    accounts(filter: { accountTag: $accountTag }) {
      gatewayL7RequestsAdaptiveGroups(
        filter: { datetime_geq: $since }
        limit: $limit
        orderBy: [datetime_ASC]
      ) {
        count
        dimensions {
          datetime
          httpHost
          httpStatusCode
          action
          email
          url
          categoryNames
          dstIpCountry
        }
      }
    }
  }
}`

type ztGatewayHTTPSource struct {
	cfg *Config
	gql *graphqlClient
}

func newZTGatewayHTTPSource(cfg *Config, gql *graphqlClient) Source {
	return &ztGatewayHTTPSource{cfg: cfg, gql: gql}
}

func (s *ztGatewayHTTPSource) Name() string { return "zt_gateway_http" }

type ztGatewayHTTPGroup struct {
	Count      int64 `json:"count"`
	Dimensions struct {
		Datetime       time.Time `json:"datetime"`
		HTTPHost       string    `json:"httpHost"`
		HTTPStatusCode int       `json:"httpStatusCode"`
		Action         string    `json:"action"`
		Email          string    `json:"email"`
		URL            string    `json:"url"`
		CategoryNames  []string  `json:"categoryNames"`
		DstIpCountry   string    `json:"dstIpCountry"`
	} `json:"dimensions"`
}

func (s *ztGatewayHTTPSource) Poll(ctx context.Context, since time.Time) ([]Event, time.Time, error) {
	data, err := s.gql.Query(ctx, ztGatewayHTTPQuery, map[string]any{
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
				GatewayL7RequestsAdaptiveGroups []ztGatewayHTTPGroup `json:"gatewayL7RequestsAdaptiveGroups"`
			} `json:"accounts"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal: %w", err)
	}
	if len(result.Viewer.Accounts) == 0 {
		return nil, time.Now(), nil
	}

	groups := result.Viewer.Accounts[0].GatewayL7RequestsAdaptiveGroups
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
			Source: "zt_gateway_http",
			Data:   b,
		})
	}

	next := time.Now()
	if !last.IsZero() {
		next = last.Add(time.Nanosecond)
	}
	return events, next, nil
}
