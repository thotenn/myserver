# Handler Glossary — Controllers

> All HTTP handlers organized by route, with description, source file, and key behavior.

---

## Middleware (applied in `internal/handlers/api.go`)

| Middleware | File | Description |
|-----------|------|-------------|
| **Recovery** | `internal/middleware/recovery.go` | Recovers from panics, logs stack trace, returns generic 500. Adapts format (JSON/HTML/text) based on Content-Type. |
| **Logging** | `internal/middleware/logging.go` | Logs each request with zap: method, path, duration, remote addr. Debug level. |
| **RateLimit** | `internal/middleware/rate_limit.go` | Token bucket rate limiter per IP. Default: 60 req/min for most routes, 10 req/min for script execution. Returns 429 with `Retry-After`. |
| **SecurityHeaders** | `internal/middleware/security_headers.go` | Adds CSP, X-Content-Type-Options, X-Frame-Options, Referrer-Policy. Optional HSTS via `HOMEPAGE_HSTS`. |
| **CORS** | `internal/middleware/cors.go` | Strict same-origin CORS. Only applies to `/api/*`. Reflects `Origin` only if it matches `Host`. Responds OPTIONS 204. |
| **HostValidation** | `internal/middleware/host_validation.go` | Validates `Host` header against `HOMEPAGE_ALLOWED_HOSTS`. Default: localhost only. Port-aware. `*` = allow all. |
| **Auth** | `internal/middleware/auth.go` | The email-allowlist gate, global. A no-op (single bool check) while `config/auth.yaml` lists nobody. With an allowlist: everything requires a session except `/static/*`, `/auth/*`, `/api/healthcheck` and `publicPaths`. Answers `302` / `401`+`HX-Redirect` / `401` JSON by request type, `403` for an authenticated address that is not listed, and `503` when the policy cannot be read. Reads the policy **per request** so edits to the YAML take effect immediately. |

---

## Auth Routes (`/auth/*`)

Registered unconditionally, but every handler answers `404` while the allowlist
is empty — byte-for-byte what chi returns for an unknown route. Registering them
conditionally would lock the operator out when they enable auth by editing the
YAML, since the gate arms live but the login page would not exist until a
restart. All live in `internal/handlers/auth.go`.

| Route | Function | Behaviour |
|---|---|---|
| `GET /auth/login` | `AuthLogin` | Minimal Templ page with a "Sign in with Google" link. Redirects to `/` if a valid session already exists. Does not load `custom.css` (operator content sits behind the gate). |
| `GET /auth/denied` | `AuthDenied` | `403` page for an address that authenticated but is not on the allowlist. |
| `GET /auth/google/start` | `AuthGoogleStart` | Generates `state` + `nonce` (32 bytes each, `crypto/rand`), stores them in a 10-minute `__Host-` cookie, redirects to Google. Rate limit 1/s. |
| `GET /auth/google/callback` | `AuthGoogleCallback` | Constant-time `state` comparison, server-to-server code exchange, claim validation, allowlist check, session cookie. Rate limit 1/s. |
| `POST /auth/logout` | `AuthLogout` | Clears the session cookie. POST on purpose: a GET logout can be triggered by any image tag. |

---

## Main Routes

### `GET /` — Dashboard

- **File:** `internal/handlers/pages.go`
- **Function:** `Dashboard() http.HandlerFunc`
- **Description:** Renders the full dashboard server-side. Loads all YAMLs (settings, services, bookmarks, widgets), determines theme/color/language, and passes `PageData` to `templates.Index()`.
- **Content negotiation:** NO — always returns HTML.
- **Hash:** Uses `config.CurrentHash()` for cache-busting in asset URLs.

---

## API Routes (`/api/*`)

### Health & Meta

| Route | Method | Handler | File | Description |
|-------|--------|---------|------|-------------|
| `/api/healthcheck` | GET | `HealthCheck` | `internal/handlers/health.go` | Returns `{"status": "OK"}`. Used by Docker healthcheck. |
| `/api/hash` | GET | `Hash` | `internal/handlers/hash.go` | Returns current config hash. Reads `atomic.Value` via `CurrentHash()`. Fallback: recompute on-the-fly. |
| `/api/reload` | POST | `Reload` | `internal/handlers/reload.go` | No-op currently. Future: will invalidate caches. |
| `/api/validate` | GET | `Validate` | `internal/handlers/validate.go` | Loads and parses all YAMLs. Returns `{"valid": true}` or list of parse errors. |
| `/api/config/{path}` | GET | `ConfigFile` | `internal/handlers/config.go` | Serves individual config files from `HOMEPAGE_CONFIG_DIR`. Path-traversal safe. |

### Config Data

| Route | Method | Handler | File | Description |
|-------|--------|---------|------|-------------|
| `/api/services` | GET | `Services` | `internal/handlers/services.go` | Loads `services.yaml`, sanitizes credentials (`SanitizeService`), returns JSON. Rate limit: 60/min. |
| `/api/bookmarks` | GET | `Bookmarks` | `internal/handlers/bookmarks.go` | Loads `bookmarks.yaml`, returns JSON without sanitization. Rate limit: 60/min. |
| `/api/widgets` | GET | `Widgets` | `internal/handlers/widgets.go` | Loads `widgets.yaml`, sanitizes credentials recursively (`SanitizeWidgets`), returns JSON. Rate limit: 60/min. |

### Widget Proxy

| Route | Method | Handler | File | Description |
|-------|--------|---------|------|-------------|
| `/api/services/proxy` | GET/POST | `Proxy` | `internal/handlers/proxy.go` | Generic proxy to widget APIs. Queries widget registry for `APITemplate`/`Mappings`. Supports `file://` scheme for local JSON. Requires `?group=X&service=Y&endpoint=Z`. Body capped to 1 MiB. Rate limit: 60/min. Content negotiation: HTMX + `display: dynamic-list` → HTML; otherwise JSON. |

### Docker

| Route | Method | Handler | File | Description |
|-------|--------|---------|------|-------------|
| `/api/docker/stats/{container}/{server}` | GET | `DockerStats` | `internal/handlers/docker.go` | Container stats (CPU%, memory, network) via Docker Engine API. HTMX → partial HTML. If not found, returns 200 with placeholder so HTMX can swap. |
| `/api/docker/status/{container}/{server}` | GET | `DockerStatus` | `internal/handlers/docker.go` | Container status (running/exited/health). Same "notfound → 200" logic for HTMX swap. |

### Proxmox

| Route | Method | Handler | File | Description |
|-------|--------|---------|------|-------------|
| `/api/proxmox/stats/{vmid}/{server}` | GET | `ProxmoxStats` | `internal/handlers/proxmox.go` | Real Proxmox API integration. Uses token auth, returns VM/container stats. |

### Monitoring

| Route | Method | Handler | File | Description |
|-------|--------|---------|------|-------------|
| `/api/ping` | GET | `Ping` | `internal/handlers/ping.go` | ICMP ping via `go-ping` (unprivileged UDP mode). 3 attempts, 5s timeout. Rate limit: 60/min. HTMX → HTML. JSON uses key `"errors"` (not `"error"`) for customapi widget compatibility. |
| `/api/siteMonitor` | GET | `SiteMonitor` | `internal/handlers/monitor.go` | HTTP HEAD fallback to GET. Measures status + latency (ms). Rate limit: 60/min. HTMX → partial HTML with badge. |

### Info Widgets

| Route | Method | Handler | File | Description |
|-------|--------|---------|------|-------------|
| `/api/widgets/resources` | GET | `ResourcesWidget` | `internal/handlers/widgets_resources.go` | Reads `/proc/stat`, `/proc/meminfo`, `/proc/uptime`, `statfs`. Returns resource bars (CPU, RAM, disk, temp, uptime). Rate limit: 60/min. HTMX → partial HTML. |
| `/api/widgets/openmeteo` | GET | `OpenMeteoWidget` | `internal/handlers/widgets_weather.go` | Weather from Open-Meteo API (free, no key). Rate limit: 60/min. HTMX → partial HTML. Also mapped as `/api/widgets/weather`. |

### Scripts (only if `HOMEPAGE_SCRIPTS_ENABLED=true`)

| Route | Method | Handler | File | Description |
|-------|--------|---------|------|-------------|
| `/api/scripts` | GET | `ListScripts` | `internal/handlers/scripts.go` | Lists all registered scripts with status (running, lastStatus, lastExitCode). Rate limit: 60/min. |
| `/api/scripts/{name}/status` | GET | `GetScriptStatus` | `internal/handlers/scripts.go` | Status of a specific script. Rate limit: 60/min. |
| `/api/scripts/{name}` | POST | `RunScript` | `internal/handlers/scripts.go` | Executes script. Requires `Origin` validation. If `requireConfirm`, demands header `X-Homepage-Confirm: yes`. Rate limit: 10/min. HTMX → partial HTML (`ScriptResult`). |
| `/api/scripts/{name}/stream` | POST | `StreamScript` | `internal/handlers/scripts.go` | Executes script with SSE streaming. Same CSRF + confirm validation. Rate limit: 10/min. |

---

## Content Negotiation Helpers

| Function | File | Description |
|----------|------|-------------|
| `isHTMXRequest(r)` | `internal/handlers/docker.go` | `r.Header.Get("HX-Request") == "true"` |
| `writeUpstreamError(w, msg)` | `internal/handlers/docker.go` | Returns 502 with generic message. |
| `writeJSONError(w, msg, status)` | `internal/handlers/ping.go` | Returns JSON with key `"errors"`. |
| `writePingResult(...)` | `internal/handlers/ping.go` | Renders `PingHTML` for HTMX or JSON for API. |
| `writeWeatherResult(...)` | `internal/handlers/widgets_weather.go` | Renders `WeatherHTML` for HTMX or JSON for API. |

---

## Lookup / Resolution Functions

| Function | File | Description |
|----------|------|-------------|
| `findServiceWidget(group, service)` | `internal/handlers/proxy.go` | Looks up `WidgetConfig` for a service in `services.yaml`. |
| `findServiceByScript(name)` | `internal/handlers/scripts.go` | Looks up `Service` that references a script by name. |
| `resolveSiteMonitorURL(group, service)` | `internal/handlers/monitor.go` | Looks up `SiteMonitor` URL for a service. |
| `resolvePingHost(group, service)` | `internal/handlers/ping.go` | Looks up `Ping` URL for a service. |
| `preferredLang(r)` | `internal/handlers/scripts.go` | Reads language from `settings.yaml`, fallback "en". |
| `clientIPFromRequest(r)` | `internal/handlers/scripts.go` | Extracts real client IP. Trusts `X-Forwarded-For` / `X-Real-IP` only if peer is loopback. |
| `isOriginAllowed(r)` | `internal/handlers/scripts.go` | Same-origin check for script endpoints. |
