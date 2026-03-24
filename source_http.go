package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// httpRequests1mGroups — per-minute aggregated traffic. Aggregated, not per-request.
// NOTE: availability on free plan may vary; disable via CF_SOURCES if not accessible.
const httpQuery = `
query HTTPRequests($zoneTag: string, $since: Time, $until: Time) {
  viewer {
    zones(filter: { zoneTag: $zoneTag }) {
      httpRequests1mGroups(
        filter: { datetime_geq: $since, datetime_leq: $until }
        limit: 1440
        orderBy: [datetimeMinute_ASC]
      ) {
        dimensions {
          datetimeMinute
        }
        sum {
          requests
          bytes
          cachedBytes
          cachedRequests
          threats
          pageViews
        }
        uniq {
          uniques
        }
      }
    }
  }
}`

type httpSource struct {
	zoneID string
	gql    *graphqlClient
}

// newHTTPSource polls per-minute HTTP request aggregates for the given zone.
func newHTTPSource(zoneID string, gql *graphqlClient) Source {
	return &httpSource{zoneID: zoneID, gql: gql}
}

func (s *httpSource) Name() string { return "http:" + s.zoneID }

type httpMinuteGroup struct {
	Dimensions struct {
		DatetimeMinute time.Time `json:"datetimeMinute"`
	} `json:"dimensions"`
	Sum struct {
		Requests       int64 `json:"requests"`
		Bytes          int64 `json:"bytes"`
		CachedBytes    int64 `json:"cachedBytes"`
		CachedRequests int64 `json:"cachedRequests"`
		Threats        int64 `json:"threats"`
		PageViews      int64 `json:"pageViews"`
	} `json:"sum"`
	Uniq struct {
		Uniques int64 `json:"uniques"`
	} `json:"uniq"`
}

func (s *httpSource) Poll(ctx context.Context, since time.Time) ([]Event, time.Time, error) {
	// Don't query the current minute — it may be incomplete.
	until := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)

	data, err := s.gql.Query(ctx, httpQuery, map[string]any{
		"zoneTag": s.zoneID,
		"since":   since.UTC().Format(time.RFC3339),
		"until":   until.Format(time.RFC3339),
	})
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("graphql: %w", err)
	}

	var result struct {
		Viewer struct {
			Zones []struct {
				HTTPRequests1mGroups []httpMinuteGroup `json:"httpRequests1mGroups"`
			} `json:"zones"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal: %w", err)
	}
	if len(result.Viewer.Zones) == 0 {
		return nil, until.Add(time.Minute), nil
	}

	groups := result.Viewer.Zones[0].HTTPRequests1mGroups
	events := make([]Event, 0, len(groups))

	for _, g := range groups {
		b, err := json.Marshal(g)
		if err != nil {
			continue
		}
		events = append(events, Event{
			TS:     g.Dimensions.DatetimeMinute,
			Source: "http",
			Zone:   s.zoneID,
			Data:   b,
		})
	}

	// Next poll starts from the minute after our until.
	return events, until.Add(time.Minute), nil
}
