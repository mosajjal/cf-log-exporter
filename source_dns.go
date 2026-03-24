package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// dnsAnalyticsAdaptiveGroups — daily DNS query stats grouped by name/type/rcode.
// Date-granularity only; poll interval effectively once per day.
const dnsQuery = `
query DNSAnalytics($zoneTag: string, $since: Date, $until: Date) {
  viewer {
    zones(filter: { zoneTag: $zoneTag }) {
      dnsAnalyticsAdaptiveGroups(
        filter: { date_geq: $since, date_leq: $until }
        limit: 1000
        orderBy: [count_DESC]
      ) {
        count
        dimensions {
          queryName
          queryType
          responseCode
          responseCached
        }
      }
    }
  }
}`

type dnsSource struct {
	zoneID string
	gql    *graphqlClient
}

// newDNSSource polls daily DNS analytics for the given zone.
func newDNSSource(zoneID string, gql *graphqlClient) Source {
	return &dnsSource{zoneID: zoneID, gql: gql}
}

func (s *dnsSource) Name() string { return "dns:" + s.zoneID }

type dnsGroup struct {
	Count      int64 `json:"count"`
	Dimensions struct {
		QueryName      string `json:"queryName"`
		QueryType      string `json:"queryType"`
		ResponseCode   string `json:"responseCode"`
		ResponseCached int    `json:"responseCached"`
	} `json:"dimensions"`
}

func (s *dnsSource) Poll(ctx context.Context, since time.Time) ([]Event, time.Time, error) {
	now := time.Now().UTC()
	sinceDate := since.UTC().Format("2006-01-02")
	untilDate := now.Format("2006-01-02")

	data, err := s.gql.Query(ctx, dnsQuery, map[string]any{
		"zoneTag": s.zoneID,
		"since":   sinceDate,
		"until":   untilDate,
	})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("graphql: %w", err)
	}

	var result struct {
		Viewer struct {
			Zones []struct {
				DNSAnalyticsAdaptiveGroups []dnsGroup `json:"dnsAnalyticsAdaptiveGroups"`
			} `json:"zones"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal: %w", err)
	}
	if len(result.Viewer.Zones) == 0 {
		return nil, nextMidnightUTC(now), nil
	}

	groups := result.Viewer.Zones[0].DNSAnalyticsAdaptiveGroups
	events := make([]Event, 0, len(groups))

	for _, g := range groups {
		b, err := json.Marshal(g)
		if err != nil {
			continue
		}
		events = append(events, Event{
			TS:     now,
			Source: "dns",
			Zone:   s.zoneID,
			Data:   b,
		})
	}

	// DNS data is date-granular; next poll at start of next UTC day.
	return events, nextMidnightUTC(now), nil
}

func nextMidnightUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC)
}
