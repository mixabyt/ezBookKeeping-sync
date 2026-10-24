# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`ezbk-tools` (Go module `ezbkTools`) receives Monobank webhooks and stores each transaction as a job in SQLite. A background worker maps each job to an ezBookkeeping category and account, then writes it to a self-hosted ezBookkeeping instance. The same binary serves a small PWA (Vue, embedded from `web/public`) for mapping categories and accounts and for reviewing transactions. Privat and other banks are planned as additional adapters.

This is also a learning project. The user writes most of the code themselves: explain approaches in words and give code only when they explicitly ask for it. The user communicates in Ukrainian, and code comments and log messages are in Ukrainian. `ARCHITECTURE.md` (Ukrainian) explains the layering rationale. It predates the mapping, modes and PWA work.

## Commands

```sh
go build ./...
go vet ./...
go test ./...                     # no tests exist yet; CI still runs it
go test ./internal/app -run TestName
go run ./cmd/ezbk-tools           # run from repo root: .env is loaded from the working dir
go run ./cmd/hashpw               # bcrypt hash for AUTH_PASSWORD_HASH

# migrations (golang-migrate CLI)
migrate create -ext sql -dir database/migrations -seq <name>
migrate -path database/migrations -database "sqlite3://database/ezBookKeepingTool" up
```

**Every push to `master` publishes a GitHub release** (`.github/workflows/release.yml`). The workflow runs `go vet ./...` and `go test ./...`, builds a static linux/amd64 binary, and tars it together with `database/migrations`. A vet failure blocks the release.

## Configuration

Settings come from `.env` (template: `.env.example`).
- `.env` holds real tokens, so never print its values.
- Comments must start with `#`.
- `AUTH_PASSWORD_HASH` must be single-quoted, because godotenv expands `$` and bcrypt hashes start with `$2a$`.
- `DB_DATABASE` is a SQLite file path relative to the working dir.
- `LOG_LEVEL=debug` selects the human-readable tint logger; anything else uses zap through `zapslog`.
- `MONO_SPOOL_URL` is optional (see Spool below).

`sqlite.Open` refuses to start when the schema is older than `requiredSchemaVersion` (`internal/adapters/sqlite/open.go`). **Bump that constant with every new migration.** It also enables WAL and `busy_timeout`, because the worker writes while the PWA reads. Never edit an applied migration. SQLite can't alter CHECK constraints, so status changes rebuild `jobs` (see 000006/000007).

## Architecture

The project uses Hexagonal / Ports & Adapters. Dependencies point inward: `adapters/* → app → domain`. `cmd/ezbk-tools/main.go` is the composition root. Shutdown runs in this order: HTTP → worker → spool poller → DB → logger sync.

- `internal/domain`: pure types and rules. No JSON, SQL or gin.
- `internal/app`: use cases.
  - `ports.go` declares every interface the app consumes (the consumer owns the interface).
  - `SyncService` is the hot path: `Enqueue` and `Process`.
  - `MappingService` covers human work done through the UI: rules, account mappings, mode, confirming jobs. After each change it wakes the worker through `JobWaker`.
  - `Worker` loops.
  - Sentinel errors (`ErrRuleExists`, `ErrJobNotReviewable`, ...) are mapped to HTTP codes in `httpapi/response.go` `classifyError`.
- `internal/adapters/httpapi`: gin server.
  - The Monobank webhook.
  - `/api/*` for the PWA. Single-user auth comes from env: bcrypt password plus an HMAC-signed session cookie, with per-IP login throttling.
  - `NoRoute` serves the embedded PWA.
  - Uses a unified JSON envelope.
- `internal/adapters/monobank`: webhook signature check.
  - The signature is ECDSA secp256k1 over SHA-256 of the **raw body**. `X-Sign` is base64 of r‖s (64 bytes, not DER), and `X-Key-Id` selects the key.
  - The public key comes from `/bank/sync` and is cached by key id.
  - `CheckSign(*http.Request)` wraps `Verify(ctx, header, raw)`.
  - Always verify before parsing JSON.
  - **Note:** the `checkMonoSign` middleware on live `POST /mono-webhook` is currently commented out.
- `internal/adapters/ezbookkeeping`: API client, also used as the category and account catalog for the PWA.
  - `response.go`'s `do()` is the single request path. A call succeeds only on **2xx with `success: true`**; anything else becomes `*APIError`.
  - Every request needs `X-Timezone-Name` / `X-Timezone-Offset` (Europe/Kyiv, embedded through `time/tzdata`), otherwise you get error 200008.
  - DTOs live in `dto/`.
- `internal/adapters/sqlite`: repositories.
- `internal/adapters/spool`: drains the VPS webhook queue (see below).
- `internal/platform/logctx`: carries the `*slog.Logger` in `context`. Use `log/slog` with key-value args; zap fields passed to slog produce `!BADKEY`.

### Transaction flow

1. **Intake:** the webhook handler builds a `domain.BankTransaction` (`Source` plus `tx_id` with a `mb_` prefix) and calls `Enqueue`.
   - For banks without MCC, `Enqueue` normalizes bank category → MCC via `bank_category_mcc`. A miss is recorded in `unmapped_keys`.
   - `jobs.Add` inserts with `ON CONFLICT (source, tx_id) DO NOTHING`, so repeated webhooks are no-ops.
   - The handler then wakes the worker and always answers 200, so Monobank never retries or disables the webhook.
2. **Worker:** each loop drains `pending` jobs in batches, then re-checks `waiting_mapping` and `needs_review` jobs. It walks those with an id cursor, because their set doesn't shrink when nothing was mapped. It then sleeps until a wake signal, a 15-min tick or ctx cancel.
3. **`Process`:** reads the mapping mode from `settings` for every job, so a switch in the UI also applies to jobs already waiting.
   - **Account:** comes from `account_mappings`. There is **no default**.
   - **Category:** a human-confirmed `job.CategoryID` always wins. Otherwise it depends on the mode, and there is **no default** category either:
     - `mcc`: an MCC range rule;
     - `merchant`: a merchant rule (MCC + direction + exact trimmed description), then an MCC rule;
     - `manual`: the job goes to `needs_review` for a person to confirm in the PWA.
   - If the category or account is missing, the job goes to `waiting_mapping` with the reason in `last_error`. Only real errors set `failed`.
   - After a successful write to ezBookkeeping, `MarkDone` stores the category that was used. That column feeds the PWA's "same as last time" suggestion.

Job statuses: `pending → done | failed | waiting_mapping | needs_review`. `needs_review` goes back to `pending` on confirm, and the UPDATE is guarded by status so a double tap can't send twice.

### Spool (webhooks received while the home server was down)

When the server is unreachable, nginx on the VPS answers Monobank with 200 and saves each POST as one JSON file. `MONO_SPOOL_URL` points to the queue, which is reachable only through the VPN.
- Each file holds `method`, `uri`, `headers` (ordered `[name, value]` pairs), `body` (the raw signed bytes as a string) and `upstream_status`.
- The queue is served by nginx `autoindex_format json`. `DELETE` returns 204, or 404 if the file is already gone.

How the service drains it:
- `main` drains the queue once **before** starting HTTP, then `spool.Poller` re-checks every 5 min.
- `monoSpoolHandler` in `main` routes each file to `httpapi.ReplayMonoWebhook`. That function runs `monobank.Verify` on `body` as-is, then reuses the same DTO → domain mapping as the live handler.
- A file is deleted on success or `spool.ErrDiscard`: forged or missing signature, broken JSON, wrong route.
- A file is kept for retry on any other error: key fetch failure, unknown key id after rotation, DB error.

### Invariants

- `Source` is a separate field and column, and it is the source of truth for the bank. Never parse the bank out of the ID prefix.
- `Amount` is an int64 in kopecks; negative means expense (`BankTransaction.Direction()`). Direction is not stored in `jobs`; SQL derives it from `amount < 0`.
- Merchant descriptions are compared exactly after trim, because SQLite's `lower()` is ASCII-only.
