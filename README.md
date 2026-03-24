# cf-log-exporter

A zero-dependency Go binary that polls the Cloudflare API and streams your account's logs as JSON lines to stdout.

Works on the **free Cloudflare plan** — no Logpush required. Designed to run as a systemd service and feed logs into journald, a SIEM, or any log aggregator that reads stdin.

## What it exports

| Source | Description | Retention (free) |
|--------|-------------|-----------------|
| `firewall` | WAF/security events per zone | 24h |
| `dns` | DNS query analytics per zone (daily aggregates) | varies |
| `audit` | Account-level config changes | 18 months |
| `zt_access` | Zero Trust Access login/logout events | 24h |
| `zt_gateway_dns` | Gateway DNS queries (requires WARP or Gateway resolver) | 24h |
| `zt_gateway_http` | Gateway HTTP requests (requires WARP) | 24h |
| `zt_gateway_l4` | Gateway TCP/UDP sessions (requires WARP) | 24h |

Output is one JSON object per line:

```json
{"ts":"2026-03-24T03:10:05Z","source":"zt_gateway_dns","zone":"...","data":{...}}
```

## Authentication

Two auth methods are supported:

### API Token (recommended)

Create a scoped token at **Cloudflare Dashboard → My Profile → API Tokens → Create Token**.

Use the "Read all resources" template, or manually grant:
- `Account / Account Settings: Read`
- `Account / Zero Trust: Read`
- `Zone / Zone: Read`
- `Zone / Analytics: Read`
- `Zone / Firewall Services: Read`

Set `CF_API_TOKEN` in your environment.

### Global API Key (legacy)

Found at **Cloudflare Dashboard → My Profile → API Tokens → Global API Key**.

Set both `CF_API_KEY` and `CF_AUTH_EMAIL`.

## Configuration

All configuration is via environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CF_API_TOKEN` | one of | — | Scoped API token (Bearer auth) |
| `CF_API_KEY` | one of | — | Global API key (legacy) |
| `CF_AUTH_EMAIL` | if using key | — | Account email (also accepts `CF_ACC_ID`) |
| `CF_ACCOUNT_ID` | yes | — | Account ID (hex string from dashboard URL) |
| `CF_ZONE_IDS` | no | — | Comma-separated zone IDs (for firewall/dns sources) |
| `CF_POLL_INTERVAL` | no | `60s` | Poll interval (Go duration string) |
| `CF_STATE_FILE` | no | `/var/lib/cf-log-exporter/state.json` | Path for cursor state |
| `CF_SOURCES` | no | all sources | Comma-separated list of sources to enable |

**Finding your Account ID:** Log in to the Cloudflare dashboard. Your account ID is in the URL:
`https://dash.cloudflare.com/<account-id>/...`

**Finding your Zone IDs:** Dashboard → select a domain → Overview → right sidebar shows the Zone ID.

## Running

```bash
# Build
go build -o cf-log-exporter .

# Run (API token auth)
CF_API_TOKEN=your_token \
CF_ACCOUNT_ID=your_account_id \
CF_ZONE_IDS=zone1,zone2 \
./cf-log-exporter

# Run (Global API Key auth)
CF_API_KEY=your_global_key \
CF_AUTH_EMAIL=you@example.com \
CF_ACCOUNT_ID=your_account_id \
CF_ZONE_IDS=zone1,zone2 \
./cf-log-exporter

# Only specific sources
CF_SOURCES=firewall,audit ./cf-log-exporter
```

JSON lines go to **stdout**. Structured logs (slog) go to **stderr**.

## Systemd deployment

Create `/etc/cf-log-exporter.env`:

```ini
CF_API_TOKEN=your_token
CF_ACCOUNT_ID=your_account_id
CF_ZONE_IDS=zone1,zone2
CF_STATE_FILE=/var/lib/cf-log-exporter/state.json
```

Install the service:

```bash
make build
sudo make install
sudo systemctl enable --now cf-log-exporter
```

Or manually:

```bash
sudo cp cf-log-exporter /usr/local/bin/
sudo cp cf-log-exporter.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cf-log-exporter
```

Logs flow to journald:

```bash
journalctl -fu cf-log-exporter
```

## Piping to a log aggregator

Since output is JSON lines on stdout, you can pipe it anywhere:

```bash
# To a file
./cf-log-exporter >> /var/log/cloudflare.jsonl

# To Vector
./cf-log-exporter | vector --config vector.toml

# Via systemd to a local VictoriaLogs / Loki instance
# (configure your log shipper to read from journald unit syslog_identifier=cf-log-exporter)
```

## State

The exporter writes a JSON file tracking the last-polled timestamp per source. On restart it resumes from where it left off. On first run it looks back 5 minutes.

To backfill, seed the state file:

```bash
echo '{"audit":"2024-01-01T00:00:00Z"}' > /var/lib/cf-log-exporter/state.json
```

## Notes

- `httpRequests1mGroups` (per-minute HTTP traffic) is not available on the free plan and is excluded from defaults. Add `http` to `CF_SOURCES` if you're on a paid plan.
- Gateway sources (`zt_gateway_*`) return empty results if you are not using Cloudflare Gateway / WARP.
- The GraphQL Analytics API allows 300 requests per 5-minute window. With 7 sources at 60s poll interval you use ~7 requests/minute, well within limits.

## License

MIT
