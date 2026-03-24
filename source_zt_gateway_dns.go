package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Zero Trust Gateway DNS — queries resolved through Cloudflare Gateway.
// Requires Gateway DNS to be configured (WARP client or custom resolver).
const ztGatewayDNSQuery = `
query GatewayDNS($accountTag: string, $since: Time, $limit: int) {
  viewer {
    accounts(filter: { accountTag: $accountTag }) {
      gatewayResolverQueriesAdaptiveGroups(
        filter: { datetime_geq: $since }
        limit: $limit
        orderBy: [datetime_ASC]
      ) {
        count
        dimensions {
          datetime
          queryName
          resolverDecision
          policyName
          locationName
          categoryNames
          srcIpCountry
        }
      }
    }
  }
}`

type ztGatewayDNSSource struct {
	cfg *Config
	gql *graphqlClient
}

func newZTGatewayDNSSource(cfg *Config, gql *graphqlClient) Source {
	return &ztGatewayDNSSource{cfg: cfg, gql: gql}
}

func (s *ztGatewayDNSSource) Name() string { return "zt_gateway_dns" }

type ztGatewayDNSGroup struct {
	Count      int64 `json:"count"`
	Dimensions struct {
		Datetime         time.Time `json:"datetime"`
		QueryName        string    `json:"queryName"`
		ResolverDecision uint16    `json:"resolverDecision"`
		PolicyName       string    `json:"policyName"`
		LocationName     string    `json:"locationName"`
		CategoryNames    []string  `json:"categoryNames"`
		SrcIpCountry     string    `json:"srcIpCountry"`
	} `json:"dimensions"`
}

func (s *ztGatewayDNSSource) Poll(ctx context.Context, since time.Time) ([]Event, time.Time, error) {
	data, err := s.gql.Query(ctx, ztGatewayDNSQuery, map[string]any{
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
				GatewayResolverQueriesAdaptiveGroups []ztGatewayDNSGroup `json:"gatewayResolverQueriesAdaptiveGroups"`
			} `json:"accounts"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal: %w", err)
	}
	if len(result.Viewer.Accounts) == 0 {
		return nil, time.Now(), nil
	}

	groups := result.Viewer.Accounts[0].GatewayResolverQueriesAdaptiveGroups
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
			Source: "zt_gateway_dns",
			Data:   b,
		})
	}

	next := time.Now()
	if !last.IsZero() {
		next = last.Add(time.Nanosecond)
	}
	return events, next, nil
}
