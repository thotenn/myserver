# General Workflow — MyServer

> Centralized self-hosted dashboard. Rewrite of Homepage (Next.js/React) to Go
with Templ + HTMX + Tailwind CSS. 100% compatible with the original Homepage
YAML files. Target: ~50-80 MiB RAM at runtime.

Complete user documentation: `README.md` (1200+ lines).

---

## 1. Build & Development

### Main Commands (`Makefile`)

| Command | Description |
|---------|-------------|
| `make build` | `templ generate` + `tailwindcss --minify` + `go build -ldflags="-s -w"` |
| `make test` | `go test ./...` |
| `make test-race` | `go test -race ./...` — **MANDATORY before merging changes in `internal/scripts/`** |
| `make test-cover` | Coverage per package |
| `make lint` | `gofmt -l` + `go vet` |
| `make tidy` | `go mod tidy` |
| `make dev` | Hot reload with `air` (reads `.air.toml`) |
| `make templ` | Regenerates `*_templ.go` |
| `make tailwind` | Compiles CSS from `web/tailwind/input.css` |
| `make docker-build` | Docker image build |
| `make up` | `docker compose up -d --build` |
| `make down` | `docker compose down` |
| `make logs` | `docker compose logs -f` |
| `make clean` | Removes binary, generated CSS, and `*_templ.go` files |

### Prerequisites

- **Go 1.25+**
- **Tailwind CSS v3 standalone CLI** (NOT v4; `input.css` uses `@layer base` with `@apply` which v4 broke)
- **templ** (`go install github.com/a-h/templ/cmd/templ@latest`)
- **air** (optional, for `make dev`)
- **docker-cli** + **bash** (runtime, for scripts feature)

### Tailwind CSS v3

The build uses the standalone version downloaded from TailwindLabs releases. In the `Dockerfile` it is pinned to `v3.4.17`. The input is `web/tailwind/input.css` and the output is `web/static/css/main.css`.

### Templ

`.templ` files are compiled into `*_templ.go`. These generated files **are committed** to the repo. Any change to `.templ` requires running `make templ`.

**Critical Templ Rules:**
- **DO NOT interpolate `{ ... }` inside `<script>` or `<style>` blocks** — Templ passes them literally. For dynamic JS/CSS, use `data-*` attributes and read them from `web/static/js/app.js`.
- Dynamic attributes: `href={ "/x?v=" + data.Hash }`, NOT `href="/x?v={ data.Hash }"`.
- For `style` attribute in grid layouts: the helper must return the complete declaration: `grid-template-columns: repeat(4, 1fr);`, not just the value.

---

## 2. Request Flow (HTTP)

```
Client (browser)
    ↓
[Reverse proxy / auth layer]
    ↓
Go HTTP server (:3000 by default)
    ↓
chi.Router
    ↓
Global middleware: Recovery → Logging
    ↓
Static files (/static/*)  OR  Dashboard (/)  OR  API (/api/*)
    ↓
API routes apply: RateLimit → SecurityHeaders → CORS → HostValidation
    ↓
Specific handler
```

### Content Negotiation

Handlers perform **content negotiation** via the `HX-Request` header:
- `HX-Request: true` → returns HTML (server-side rendered with Templ templates)
- No header → returns JSON (for API clients / JS widgets)

This is key to understanding how HTMX works in the dashboard: widgets update via `hx-get` and the server responds with partial HTML.

---

## 3. Configuration & Hot-Reload

### Configuration YAML Files

Located in `$HOMEPAGE_CONFIG_DIR` (default: `/app/config`):

| File | Loader | Used by |
|------|--------|---------|
| `settings.yaml` | `d.Settings()` | Dashboard, i18n, theme, scripts config |
| `services.yaml` | `d.Services()` | Dashboard, `/api/services`, Proxy, Ping, SiteMonitor, Docker stats |
| `bookmarks.yaml` | `d.Bookmarks()` | Dashboard, `/api/bookmarks` |
| `widgets.yaml` | `d.Widgets()` | Dashboard, `/api/widgets` |
| `docker.yaml` | `d.Docker()` | DockerStats, DockerStatus handlers |
| `kubernetes.yaml` | `d.Kubernetes()` | KubernetesStats, KubernetesStatus (stub) |
| `proxmox.yaml` | `d.Proxmox()` | ProxmoxStats handler |
| `scripts.yaml` | `d.ScriptsFile()` | ScriptManager registration (no hot-reload yet) |
| `custom.css` | — | Injected into the `<head>` of the dashboard |
| `custom.js` | — | Injected before the closing `</body>` |

### Env Var Substitution

`config.SubstituteEnvVars()`, applied to every file as it is read, replaces:
- `{{HOMEPAGE_VAR_XXX}}` → value of env var `HOMEPAGE_VAR_XXX`
- `{{HOMEPAGE_FILE_XXX}}` → contents of the file pointed to by `HOMEPAGE_FILE_XXX`

If a reference cannot be resolved, the placeholder is kept literally (fail-visible).

### Hot-Reload

1. `main.go` builds the dashboard registry at startup and calls `Reload()` on each dashboard, which computes that dashboard's config hash and stores it in the snapshot it swaps in atomically.
2. An `fsnotify.Watcher` is started over the config directory.
3. Every change to `.yaml`, `.yml`, `.css`, `.js` triggers `onChange()` which recalculates the hash.
4. The frontend (in `app.js`) polls `/api/hash` every 10s; if it changes, it performs `window.location.reload()`.

**IMPORTANT:** handlers MUST read the hash per request, from the dashboard on the request context (`config.DashboardFrom(ctx).Hash()`), NOT capture it in a closure at startup. There is no process-wide config hash: each dashboard has its own, so a change to one does not tell every other dashboard's browser to reload.

---

## 4. Scripts Feature (opt-in)

Enabled with `HOMEPAGE_SCRIPTS_ENABLED=true`.

### Lifecycle

1. `main.go` loads `settings.yaml` to read `ScriptSettings` (timeouts, dirs, concurrency).
2. Loads `scripts.yaml` and registers each script in `scripts.Manager`.
3. The `Manager` validates each script:
   - Command must be **relative** (no absolute path)
   - Must resolve to a `.sh` file within the whitelisted `scriptDirs`
   - Anti path-traversal validation with `EvalSymlinks` + `HasPrefix`
   - Env vars in denylist (`LD_PRELOAD`, `PATH`, `BASH_FUNC_*`, etc.) are rejected
4. `handlers.ScriptManager` is exposed to the HTTP handlers.

### Execution

- `RunScript` (POST `/api/scripts/{name}`): synchronous execution, returns full result
- `StreamScript` (POST `/api/scripts/{name}/stream`): execution with real-time SSE output streaming
- `requireConfirm` is enforced **server-side** with the header `X-Homepage-Confirm: yes` (not just `hx-confirm`)

### Script Security

- Does NOT inherit parent process env. Only: `PATH`, `HOME=/tmp`, `USER=myserver`, `SHELL=/bin/bash`, `TZ`.
- Executor uses `Setpgid` to kill the entire process tree on cancellation.
- Output capped at 1 MiB per execution.
- Global semaphore for concurrency (default 5).
- Default timeout 60s, capped by `maxTimeout`.

### Important Note

`scripts.yaml` **has hot-reload** via the config watcher. Changes trigger `Manager.ReplaceAll()` automatically. No restart needed.

---

## 5. Docker / Deployment

### Image

- Multi-stage: `golang:1.25-alpine` → `alpine:3.21`
- Runtime with: `su-exec`, `bash`, `docker-cli`, `wget`, `tini`, `tzdata`
- User `myserver:1000` (non-root)
- Exposed port: `3000`

### Docker Compose

- **Host bind mount** `/srv/myserver/config:/app/config` — user YAMLs, scripts (`config/scripts/`), and local data (`config/data/`)
- Mount `/var/run/docker.sock` — for Docker stats and script wrappers
- Critical environment variables (set via your deployment UI, NOT in compose):
  - `HOMEPAGE_ALLOWED_HOSTS=your.domain.com,localhost:3000`
  - `HOMEPAGE_SCRIPTS_ENABLED=true`
  - `TZ=UTC` (or your timezone)

---

## 6. Testing

### Critical Adversarial Tests

| Package | Key Tests |
|---------|-----------|
| `internal/scripts` | Path traversal, `.sh` extension, env denylist, race conditions, timeout, output limit, hot-reload |
| `internal/middleware` | HostValidation: defaults, wildcard, port-aware, case-insensitive |
| `internal/handlers` | Basic-auth strip, recursive sanitization, scripts disabled → 404 |

### Golden Rule

> **Before any change in `internal/scripts/`, always run:**
> ```bash
> go test -race -count=1 ./internal/scripts/
> ```

---

## 7. Project Conventions

- Commits in English: `myserver: phase N description`
- Tests in `*_test.go` next to the source code
- Templates use `config.Service`, `config.ServiceGroup`, etc. directly
- i18n via `T(lang, "key")` with hardcoded map in `internal/templates/i18n.go`
- Structured logs with zap; never `fmt.Printf`
- Handlers return generic messages to the client, log details internally (don't leak paths, stack traces, etc.)
- Public by default: guard the dashboard with either an external auth layer (reverse proxy, Cloudflare Access, Authelia…) or the built-in email allowlist (`config/auth.yaml`, see [`authentication.md`](./authentication.md)). There are no local usernames or passwords.

---

## 8. Quick Environment Glossary

| Variable | Description |
|----------|-------------|
| `HOMEPAGE_CONFIG_DIR` | Config directory (default: `/app/config`) |
| `HOMEPAGE_ALLOWED_HOSTS` | Allowed hosts for HostValidation (default: localhost only) |
| `HOMEPAGE_SCRIPTS_ENABLED` | `true` to enable scripts feature |
| `HOMEPAGE_ALLOW_PRIVATE_HOSTS` | `true` (default) allows proxy to private IPs and loopback |
| `HOMEPAGE_PROXY_DISABLE_IPV6` | `true` forces IPv4 in proxy |
| `HOMEPAGE_HSTS` | `true` adds `Strict-Transport-Security` header |
| `TRUSTED_PROXIES` | Comma-separated list of trusted proxy IPs/CIDRs |
| `DEBUG` | `true` uses zap Development mode (verbose logs) |
| `HOMEPAGE_AUTH_REQUIRED` | `true` refuses to start without an email allowlist, and answers 503 whenever the auth policy is unavailable |
| `HOMEPAGE_VAR_*` / `HOMEPAGE_FILE_*` | Substituted into any YAML. Used for the Google OAuth client id/secret and the session key — see [`authentication.md`](./authentication.md) |
