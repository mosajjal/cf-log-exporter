# Agent Guide — cf-log-exporter

## What this is

A Go binary that polls the Cloudflare API on a ticker and emits JSON lines to stdout. Designed to run as a systemd service. No external dependencies — stdlib only.

## Architecture

```
main.go           — wires sources, starts goroutines, output loop
config.go         — env var config, SetAuth() helper
state.go          — thread-safe last-polled timestamp map, atomic file save
event.go          — common Event struct (ts, source, zone, data)
source.go         — Source interface
poller.go         — generic poll loop: calls Source.Poll(), forwards to channel
graphql.go        — Cloudflare GraphQL Analytics API client
source_*.go       — one file per data source
```

## Adding a new source

1. Create `source_<name>.go` in `package main`
2. Implement the `Source` interface:
   ```go
   type Source interface {
       Name() string  // unique key for state, e.g. "mysource:zoneid"
       Poll(ctx context.Context, since time.Time) ([]Event, time.Time, error)
   }
   ```
3. Return `(events, nextSince, nil)` on success. `nextSince` becomes the `since` on the next poll.
4. Wire it up in `buildSources()` in `main.go` with an `enabled["myname"]` guard.
5. Add `"myname"` to the default `Sources` slice in `config.go` if it should run by default.

### GraphQL source pattern (account-level)

```go
const myQuery = `
query MyQuery($accountTag: string, $since: Time, $limit: int) {
  viewer {
    accounts(filter: { accountTag: $accountTag }) {
      someDatasetAdaptiveGroups(
        filter: { datetime_geq: $since }
        limit: $limit
        orderBy: [datetime_ASC]
      ) {
        count
        dimensions { datetime ... }
      }
    }
  }
}`

data, err := s.gql.Query(ctx, myQuery, map[string]any{
    "accountTag": s.cfg.AccountID,
    "since":      since.UTC().Format(time.RFC3339),
    "limit":      10000,
})
```

For zone-level datasets, use `zones(filter: { zoneTag: $zoneTag })` instead.

### REST source pattern

See `source_audit.go` or `source_zt_access.go`. Use `s.cfg.SetAuth(req)` to apply auth headers — this handles both Bearer token and Global API Key automatically.

## Auth

`config.go` supports two modes set via env vars:
- `CF_API_TOKEN` → `Authorization: Bearer <token>`
- `CF_API_KEY` + `CF_AUTH_EMAIL` → `X-Auth-Key` + `X-Auth-Email`

`cfg.SetAuth(req)` picks the right one. All sources use this — never hardcode auth headers.

## Introspecting the Cloudflare GraphQL schema

When adding a new GraphQL source, verify field names before coding:

```bash
# List all types matching a keyword
curl -s -X POST https://api.cloudflare.com/client/v4/graphql \
  -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"{ __schema { types { name } } }"}' \
  | jq '[.data.__schema.types[].name | select(test("keyword"; "i"))]'

# Get fields for a specific type
curl -s -X POST https://api.cloudflare.com/client/v4/graphql \
  -H "Authorization: Bearer $CF_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"{ __type(name: \"TypeName\") { fields { name type { name kind ofType { name } } } } }"}' \
  | jq '.data.__type.fields[] | "\(.name) -> \(.type.name // .type.ofType.name)"'
```

Field names in the API do not always match the docs. Always introspect first.

## State

`state.go` stores a `map[string]time.Time` keyed by `Source.Name()`. It's loaded at startup, saved every 30s and on clean shutdown. On first run (zero state) the poller defaults to `time.Now() - 5 minutes`.

To backfill, write a state file before starting:
```bash
echo '{"audit":"2024-01-01T00:00:00Z"}' > state.json
```

## Known free-plan limitations

- `http` source (`httpRequests1mGroups`) is excluded from defaults — not available on free
- All ZT Gateway sources return empty if Gateway/WARP is not configured
- Firewall events have 24h retention — service must run continuously to avoid gaps
- DNS zone analytics are date-granular (one poll per day)
