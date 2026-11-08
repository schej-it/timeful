# Timeful Deployment Guide

Production deployment using Docker Compose behind a Caddy reverse proxy.

## Prerequisites

- Docker and Docker Compose
- Caddy on the host (for reverse proxy + automatic HTTPS, although you can use any reverse proxy)
- Domain with DNS pointing to your server

## Quick Start

```bash
# 1. Clone the repository
git clone https://github.com/schej-it/timeful.app
cd timeful.app

# 2. Create server environment file
cp server/.env.template server/.env
# Edit server/.env with your values (see Configuration below)

# 3. Build and start services
docker compose up -d --build

# 4. Configure Caddy
sudo cp Caddyfile.example /etc/caddy/Caddyfile
# Edit /etc/caddy/Caddyfile with your domain
sudo systemctl reload caddy
```

## Services

| Service    | Description                             | Port           |
| ---------- | --------------------------------------- | -------------- |
| `mongo`    | MongoDB 7 database                      | Internal only  |
| `frontend` | Vue.js build (outputs to shared volume) | N/A            |
| `server`   | Go backend                              | 127.0.0.1:3002 |

## Caddy

The example Caddyfile proxies all traffic to the Go backend on port 3002. Caddy handles:

- Automatic HTTPS certificates
- HTTP → HTTPS redirect
- www → non-www redirect
- Compression (gzip/zstd)
- Security headers

Edit `/etc/caddy/Caddyfile` with your domain before reloading.

## Health check

`GET /api/health` (and `HEAD`) pings MongoDB with a 2s deadline (`MONGODB_HEALTH_TIMEOUT`):

- `200 {"status":"ok"}` when MongoDB responds.
- `503 {"status":"unavailable","error":"service-unavailable"}` when it does not. The driver error is written to the server log, not to the response.

The compose healthcheck probes this route, so `docker compose ps` shows the server as `unhealthy` while MongoDB is unreachable. Plain Docker Compose does not restart a container that is only unhealthy; `restart: unless-stopped` applies when the process exits.

When MongoDB is unreachable, API requests that need it return `503 {"error":"service-unavailable"}` with `Retry-After: 10` within about 7s when MongoDB is down (5s server selection timeout, plus up to 2s for a health ping), instead of an empty 500 after 30s. When MongoDB accepts connections but stops replying (e.g. a hung mongod), a request can take about 20 to 30s: the 15s socket timeout, then one driver retry that waits for server selection and can wait up to another 15s if the driver has not yet marked the server as down. A request that runs several queries can take longer.

If a MongoDB ping fails when the server starts, the server retries all ping errors except authentication failures for 30s (`MONGODB_STARTUP_TIMEOUT`), then logs one `[ERROR]` line and keeps running in degraded mode: the frontend is served, `/api/health` returns 503, and API calls return 503. The driver reconnects on its own, so once MongoDB is back `/api/health` returns 200 without a restart. An invalid URI or option, an invalid `MONGODB_*_TIMEOUT` value, or wrong credentials in `MONGODB_URI` stop the server at startup. The startup check uses `ping`, which MongoDB allows without logging in, so a URI with no credentials at all passes the check even when MongoDB requires authentication; queries then fail with an empty 500.

On the non-Docker deployment, `deploy_scripts/reboot_server_if_down.sh` (run from cron) checks this route: 200 is healthy; 503 is logged and posted to `HEALTH_ALERT_WEBHOOK_URL` if set, without a restart; no response for 2 consecutive runs restarts the server.

## Commands

```bash
docker compose up -d              # Start services
docker compose logs -f            # View logs
docker compose logs -f server     # View specific service logs
docker compose up -d --build      # Rebuild after code changes
docker compose down               # Stop services
docker compose down -v            # Stop and remove volumes (deletes data!)
```

## Data & Backup

Data is persisted in Docker volumes: `mongo_data`, `frontend_dist`, `server_logs`.

```bash
# Backup MongoDB
docker compose exec mongo mongodump --db=schej-it --archive=/data/db/backup.archive
docker compose cp mongo:/data/db/backup.archive ./backup.archive

# Restore MongoDB
docker compose cp ./backup.archive mongo:/data/db/backup.archive
docker compose exec mongo mongorestore --drop --db=schej-it --archive=/data/db/backup.archive
```

## Troubleshooting

```bash
# Container won't start
docker compose logs server
ls -la server/.env

# MongoDB connection issues
docker compose ps
docker compose exec mongo mongosh --eval "db.adminCommand('ping')"

# Frontend not loading
docker compose logs frontend
docker compose exec server ls -la /app/frontend/dist
```

---

## Configuration

### Required Environment Variables

Create `server/.env` from the template (`server/.env.template`).

#### Required

| Variable         | Description                                                                 |
| ---------------- | --------------------------------------------------------------------------- |
| `CLIENT_ID`      | Google OAuth client ID                                                      |
| `CLIENT_SECRET`  | Google OAuth client secret                                                  |
| `ENCRYPTION_KEY` | Key for encrypting sensitive data (generate with `openssl rand -base64 32`) |
| `SESSION_SECRET` | Session cookie encryption key (generate with `openssl rand -base64 32`)     |

#### Optional — Additional Calendars

| Variable                  | Description                             |
| ------------------------- | --------------------------------------- |
| `MICROSOFT_CLIENT_ID`     | Microsoft OAuth client ID (for Outlook) |
| `MICROSOFT_CLIENT_SECRET` | Microsoft OAuth client secret           |

#### Optional — MongoDB

| Variable                           | Default               | Description                                                                                                                                                                                  |
| ---------------------------------- | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MONGODB_URI`                      | `mongodb://localhost` | Connection string. Timeouts given here (e.g. `serverSelectionTimeoutMS`) take precedence over the variables below                                                                          |
| `MONGODB_SERVER_SELECTION_TIMEOUT` | `5s`                  | How long a request waits for a reachable MongoDB before failing (driver default 30s). MongoDB runs on the same host or Docker network, where selection takes milliseconds |
| `MONGODB_CONNECT_TIMEOUT`          | `5s`                  | TCP dial and heartbeat connection timeout (driver default 30s). A mongod that stops replying is marked down after about 15s (this value plus the 10s heartbeat interval)                                                                                          |
| `MONGODB_SOCKET_TIMEOUT`           | `15s`                 | Limit for a single read or write when MongoDB accepts the connection but does not reply. `0` disables it. Raise it if large analytics aggregations take longer                             |
| `MONGODB_HEALTH_TIMEOUT`           | `2s`                  | Deadline for `/api/health` and for the ping that classifies failed requests                                                                                                                   |
| `MONGODB_STARTUP_TIMEOUT`          | `30s`                 | How long the server retries MongoDB at startup before it starts in degraded mode                                                                                                             |

Values are Go durations (`5s`, `1500ms`). An invalid value stops the server at startup.

#### Optional — CORS

| Variable       | Description                                                                                                          |
| -------------- | -------------------------------------------------------------------------------------------------------------------- |
| `CORS_ORIGINS` | Comma-separated allowed origins (default: production domains). For local development, set to `http://localhost:8080` |

#### Optional — Other Services

| Variable                                     | Description                                  |
| -------------------------------------------- | -------------------------------------------- |
| `ANALYTICS_USERNAME` / `ANALYTICS_PASSWORD`  | Basic auth for /api/analytics routes         |
| `SERVICE_ACCOUNT_KEY_PATH`                   | Google Cloud service account for Cloud Tasks |
| `SLACK_*_WEBHOOK_URL`                        | Slack webhooks for notifications             |
| `SLACK_SIGNING_SECRET`                       | Verifies /api/slackbot slash commands        |
| `GMAIL_APP_PASSWORD` / `SCHEJ_EMAIL_ADDRESS` | Gmail SMTP for sending emails                |
| `LISTMONK_*`                                 | Listmonk email service configuration         |
| `DISCORD_BOT_TOKEN` / `GUILD_ID`             | Discord bot integration                      |
| `TRUSTED_PROXIES`                            | Proxy IPs allowed to set X-Forwarded-For     |

See `server/.env.template` for the complete list.

### Google OAuth Setup

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select existing
3. Enable the following APIs:
   - Google Calendar API
   - People API (Contacts)
   - Admin SDK API (Directory)
4. Create OAuth 2.0 credentials (Web application type)
5. Add authorized redirect URIs:
   - `https://yourdomain.com/api/auth/callback`
   - `http://localhost:3002/api/auth/callback` (for development)
6. Copy the Client ID and Client Secret to your `.env`
