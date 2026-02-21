# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```sh
go build ./...              # build all packages
go run . fetch-blogs        # discover top 100 HN blogs, save to DB
go run . scrape             # scrape RSS feeds for saved blogs, print latest 20
go run . analyze [24h|3days|7days]  # AI score + summarize articles (default: 24h)
go run . report  [24h|3days|7days]  # generate trend report (default: 24h)
go run . notify  [24h|3days|7days]  # send report via Telegram (default: 24h)
go run . run                # run pipeline immediately, then HTTP + cron (every 6h)
go run . run "0 */2 * * *"  # start with custom cron schedule
go run . run --addr=:9090   # start with custom HTTP listen address
```

Build and deploy with Make / Docker:

```sh
make build                  # compile binary
make run                    # build + run
make docker                 # build Docker image
make up                     # docker compose up -d --build
make down                   # docker compose down
```

Database is `data/newsbot.db` (SQLite, WAL mode). There are currently no tests in the project.

## Configuration

Config file: `newsbot.yaml` in the working directory. Sensitive fields (username/password) should be set via `.env` file or environment variables.

Environment variables (override YAML values):
- `OLLAMA_ADDRESS` — Ollama API endpoint
- `OLLAMA_MODEL` — model name (default: `gemma3:4b`)
- `OLLAMA_USERNAME` — Basic Auth username
- `OLLAMA_PASSWORD` — Basic Auth password
- `TG_BOT_TOKEN` — Telegram Bot API token
- `TG_CHAT_ID` — Telegram chat/channel ID

`.env` file is auto-loaded on startup. Already in `.gitignore`.

## Architecture

CLI dispatcher in `main.go` routes subcommands (`fetch-blogs`, `scrape`, `analyze`, `report`, `notify`, `run`) to handler functions. All state is persisted in SQLite via `internal/store`.

### Data Pipeline

```
fetch-blogs: hnpopular CDN → parse CSVs → upsert blogs table
scrape:      blogs table → concurrent RSS fetch (10 goroutines) → insert articles table
analyze:     articles table → time window filter → AI score/classify → AI summarize → insert article_analysis table
report:      article_analysis table → print top articles → AI trend summary (2-3 macro trends) → auto-send Telegram if configured
notify:      article_analysis table → filter unnotified → format report → send via Telegram → mark notified
run:         run pipeline immediately → HTTP server (:8080) + cron scheduler (every 6h) in one process
```

### HTTP Server & REST API

The `run` command starts an HTTP server alongside the cron scheduler. Default listen address is `:8080`.

**Pages:**

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Redirect to `/articles?window=24h` |
| `GET` | `/health` | Health check — `{"status":"ok"}` |
| `GET` | `/articles?window=24h\|3days\|7days` | Article list HTML page with tab switching |

**REST API (JSON):**

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/articles?window=24h&limit=20` | Article list, sorted by score desc |
| `GET` | `/api/articles/{id}` | Single article detail |

Query parameters for `/api/articles`:
- `window` — `24h` (default), `3days`, `7days`
- `limit` — 1-100, default 20

### Packages

- **`internal/store`** — SQLite persistence (pure Go driver `modernc.org/sqlite`, no CGo). Three tables: `blogs` (domain, score, author, rank), `articles` (title, url, summary, published_at), and `article_analysis` (scores, category, keywords, ai_summary, title_cn, recommend_reason, notified_at). Auto-migrates schema on startup. Timestamps stored in RFC3339 UTC. Notification dedup via `notified_at` column.
- **`internal/hnpopular`** — Downloads `hn-data.csv` and `domains-meta.csv` from `hn-popularity.cdn.refactoringenglish.com`, aggregates scores, returns top N blogs.
- **`internal/scraper`** — Concurrent RSS/Atom feed scraper using `gofeed`. Tries multiple feed paths per domain (`/feed`, `/rss`, `/atom.xml`, etc.). Caps at 10 articles per blog. Strips HTML from summaries.
- **`internal/ai`** — Ollama client (OpenAI-compatible API). Includes scoring (3-dimension 1-10), summarization with retry (up to 3 attempts), and trend analysis. Uses HTTP Basic Auth when credentials are configured.
- **`internal/notify`** — `Notifier` interface (`Send(ctx, title, body) error`) for pluggable notification channels.
- **`internal/notify/telegram`** — Telegram Bot API client. HTML-formatted messages with auto-splitting for messages > 4096 chars. Pure `net/http`, no new dependencies.
- **`internal/config`** — Loads `newsbot.yaml` config with `.env` file support. Environment variables override YAML values.
- **`internal/server`** — HTTP server (`net/http`). Serves HTML article pages and REST API (`/api/articles`, `/api/articles/{id}`). No external dependencies.
- **`internal/scheduler`** — Wraps `robfig/cron` to run the full pipeline periodically. Runs pipeline immediately on startup, then on schedule. Sends Telegram notification after each run. Blocks on context cancellation for graceful shutdown (SIGINT/SIGTERM).

### Key Dependencies

| Package | Purpose |
|---|---|
| `modernc.org/sqlite` | Pure-Go SQLite (no CGo) |
| `github.com/mmcdole/gofeed` | RSS/Atom feed parsing |
| `github.com/robfig/cron/v3` | Cron scheduling |
| `gopkg.in/yaml.v3` | YAML config parsing |
