# Features — MyServer

> Complete list of features and capabilities, with references to where they are implemented.

---

## 1. Main Dashboard

- **Server-side rendering** of the full dashboard using Templ + Go.
- **Automatic hot-reload**: polling `/api/hash` every 10s; reloads when YAML/CSS/JS changes are detected.
- **Themes**: dark/light with configurable accent color (default: slate).
- **i18n**: English and Spanish hardcoded in `internal/templates/i18n.go`.
- **Bookmarks**: top section with bookmark groups.
- **Service Groups**: services organized in groups, with tab support via `layout`.
- **Quick Launch**: instant search for services/bookmarks with keyboard shortcut.
- **Custom CSS/JS**: `custom.css` and `custom.js` injected into the dashboard.
- **Zero-code customization**: everything lives in `config/` — YAMLs, scripts, and local JSON data sources. No container access needed.
- **`file://` scheme**: widgets can read local JSON files directly from `config/` without HTTP round-trips.

**Implementation:**
- `internal/handlers/pages.go` — `Dashboard()` handler
- `internal/templates/index.templ` — main layout
- `internal/templates/layout.templ`, `header.templ`, `footer.templ` — HTML structure
- `web/static/js/app.js` — hot-reload, datetime, greeting, search, quicklaunch, theme toggle, CSP-safe event listeners

---

## 2. Services & Widgets (46 types)

### Widget Registry

Widgets are registered in a global `Registry` (`widgets.DefaultRegistry`) at application startup (`widgets.RegisterBuiltinWidgets()` in `cmd/myserver/main.go`).

### Registered Widget Categories

| Category | Widgets |
|----------|---------|
| **Priority** | customapi, docker, glances, resources, speedtest, photoprism, vikunja |
| **Media management** | sonarr, radarr, lidarr, prowlarr, bazarr, overseerr |
| **Media servers** | plex, jellyfin, emby, tautulli |
| **Download clients** | qbittorrent, transmission, deluge, sabnzbd |
| **Networking** | pihole, adguard, traefik, caddy, npm (nginx-proxy-manager), cloudflared, tailscale |
| **Monitoring** | portainer, uptimekuma, netdata, prometheus, grafana |
| **Productivity** | nextcloud, trilium, paperlessngx |
| **Infrastructure** | proxmox, argocd |
| **Info widgets** | datetime, greeting, search, weather, openmeteo, stocks, kubernetes-info, longhorn |
| **Aliases** | jellyseerr → overseerr, seerr → overseerr, openweathermap → weather, hoarder → karakeep |

**Implementation:**
- `internal/widgets/registry.go` — global registry
- `internal/widgets/*.go` — per-widget definitions
- `internal/widgets/types.go` — `Widget`, `ProxyHandler`, `Mapper`, `Validator` interfaces

### `customapi` Widget

The most flexible widget. Supports:
- Any API URL
- Field mapping via `mappings`
- `display: dynamic-list` mode → server-side rendered as HTML list
- Extensible `display` modes

**Implementation:** Fully implemented with `GetValue` field-path traversal, `FormatValue` with number/date/bytes formats, and display-mode dispatch (`text`, `dynamic-list`, `graph`, `list`, `tile`). `GenericProxyHandler` queries the widget registry for `APITemplate()` and `Mappings()` dynamically.

**Implementation:** `internal/widgets/customapi.go`

---

## 3. Service Proxy

### Generic Proxy Handler

`GET/POST /api/services/proxy?group=X&service=Y&endpoint=Z`

- Resolves the service's widget from `services.yaml`
- Validates endpoint against regex `^[a-zA-Z0-9_./-]*$` (anti path traversal)
- Substitutes placeholders `{url}`, `{endpoint}`, `{key}`, `{apiKey}`, `{token}`, `{username}`, `{password}`
- Adds authentication: Basic auth > Bearer token
- Makes upstream request via `proxy.Proxy()`
- Content negotiation: if HTMX + `display: dynamic-list`, returns HTML; otherwise JSON

**Implementation:**
- `internal/handlers/proxy.go` — HTTP handler
- `internal/proxy/handlers/generic.go` — `GenericProxyHandler()`
- `internal/proxy/proxy.go` — `Proxy()` with SSRF guard, decompression, error scrubbing

### Specialized Proxy Handlers

| Handler | Use Case | File |
|---------|----------|------|
| **CredentialedProxyHandler** | Login via form POST + cookie jar | `internal/proxy/handlers/credentialed.go` |
| **JSONRPCProxyHandler** | Deluge, Transmission (JSON-RPC 2.0) | `internal/proxy/handlers/jsonrpc.go` |
| **UniFiProxyHandler** | UniFi Controller (cookie auth, TLS skip) | `internal/proxy/handlers/unifi.go` |
| **SynologyProxyHandler** | Synology DiskStation (SID auth) | `internal/proxy/handlers/synology.go` |

---

## 4. Docker Integration

### Stats & Status

- `GET /api/docker/stats/{container}/{server}` — CPU%, memory, network (via Docker Engine API)
- `GET /api/docker/status/{container}/{server}` — status (running, exited, etc.) + health check

### Content Negotiation

- HTMX → returns partial HTML (`templates.DockerStatsHTML`, `templates.DockerStatusHTML`)
- JSON → returns JSON object with metrics

### Docker Discovery

- Discovers services from containers with `homepage.*` labels
- Supports Docker Swarm when `swarm: true` in `docker.yaml`
- Supported labels: `homepage.name`, `homepage.href`, `homepage.icon`, `homepage.description`, `homepage.ping`, `homepage.siteMonitor`, `homepage.group`, `homepage.weight`, `homepage.widget` (JSON)
- Merges with config services: config takes priority over Docker

**Implementation:**
- `internal/handlers/docker.go` — HTTP handlers
- `internal/discovery/docker.go` — `DockerDiscoverer`
- `internal/discovery/merger.go` — `MergeServices()`

---

## 5. Kubernetes Integration (stub)

- `GET /api/kubernetes/stats/{service}/{server}` — stub, returns `{}`
- `GET /api/kubernetes/status/{service}/{server}` — stub
- `KubernetesDiscoverer` exists but does not implement real discovery

**Implementation:**
- `internal/handlers/kubernetes.go`
- `internal/discovery/kubernetes.go`

---

## 6. Proxmox Integration

- `GET /api/proxmox/stats/{vmid}/{server}` — real Proxmox API integration using token auth
- Requires `proxmox.yaml` with `url`, `token`, `secret`

**Implementation:** `internal/handlers/proxmox.go` — fetches live VM/container stats via Proxmox API.

---

## 7. Site Monitoring

- `GET /api/siteMonitor?url=...` or `?groupName=X&serviceName=Y`
- Tries HEAD first; if it fails or returns >=400, falls back to GET
- Timeout per attempt: 5s
- Returns: HTTP status + latency in ms
- HTMX → partial HTML with status badge

**Implementation:** `internal/handlers/monitor.go`

---

## 8. Ping (ICMP)

- `GET /api/ping?host=...` or `?groupName=X&serviceName=Y`
- Uses `go-ping` in unprivileged UDP mode (no `CAP_NET_RAW` needed)
- 3 attempts, 5s timeout
- Returns: alive (bool) + latency (ms)
- HTMX → partial HTML with status icon

**Implementation:** `internal/handlers/ping.go`

---

## 9. Info Widgets with Data Endpoints

### Resources Widget

- `GET /api/widgets/resources?cpu=true&memory=true&disk=/&uptime=true&cputemp=true&network=true`
- Reads `/proc/stat`, `/proc/meminfo`, `/proc/uptime`, `statfs`, and thermal sensors
- Returns progress bars with percentages and colors (green/yellow/red)
- HTMX → partial HTML; JSON → object with `bars[]`

**Implementation:** `internal/handlers/widgets_resources.go`

### Weather Widget (Open-Meteo)

- `GET /api/widgets/openmeteo?latitude=X&longitude=Y&label=...`
- Free, no API key. Uses `api.open-meteo.com`
- Returns temperature + weather emoji per WMO code
- HTMX → partial HTML

**Implementation:** `internal/handlers/widgets_weather.go`

---

## 10. Scripts Feature (opt-in)

See `workflow.md` for details. Summary:

- `GET /api/scripts` — list scripts and their status
- `GET /api/scripts/{name}/status` — status of a script
- `POST /api/scripts/{name}` — execute script
- `POST /api/scripts/{name}/stream` — execute with SSE streaming
- Server-side security validation (path traversal, `.sh` only, env denylist)
- `requireConfirm` enforced with header `X-Homepage-Confirm: yes`
- CSRF defense with `Origin` header validation
- Auditing to stderr (future: append-only file)

**Implementation:** `internal/handlers/scripts.go`, `internal/scripts/*.go`

---

## 11. Search & QuickLaunch

### Search Widget

- Search input with i18n placeholder
- `handleSearch()` in `app.js`: if query looks like a URL, opens directly; otherwise Google search

### QuickLaunch

- Top search bar (configurable in `settings.yaml`)
- Filters services and bookmarks in real time (DOM-based, no innerHTML of user input)
- Escape key closes results
- Max 8 results
- Fallback to "Search web for ..." if no matches

**Implementation:** `web/static/js/app.js`

---

## 12. Security

| Layer | Implementation |
|-------|---------------|
| **Auth** | Public by default; an external layer in front is the usual setup. Optional built-in email allowlist with Google sign-in — see §16. |
| **Host Validation** | `HOMEPAGE_ALLOWED_HOSTS`. Default: localhost only. `*` = allow all. Port-aware. |
| **CORS** | Same-origin only. Applied only to `/api/*`. Reflects `Origin` only if it matches `Host`. |
| **SSRF Guard** | `proxy.Proxy` blocks cloud-metadata IPs and RFC1918 by default. `HOMEPAGE_ALLOW_PRIVATE_HOSTS=true` to opt out (default true for self-hosted). |
| **Credential Sanitization** | `/api/widgets` and `/api/services` do deep-sanitize with `IsSensitiveKey` recursively. Also strips basic-auth from URLs. |
| **Script Security** | Path traversal safe, `.sh` only, env denylist, no absolute paths, timeout, output limit, concurrency semaphore. |
| **Recovery** | Panic recovery middleware that logs stack trace and returns generic 500. |

---

## 13. Config Validation

- `GET /api/validate` — loads and parses `services`, `bookmarks`, `widgets` and `settings`, returning parse errors if any.
- Useful for verifying config before deployment.
- Errors keep the YAML detail (`line 12: did not find expected key`) but have filesystem paths scrubbed: handlers must not describe the container's layout. The full error goes to the log.
- **`auth.yaml` is not included.** Its validation errors name environment variables, which would turn a config check into an environment probe. The auth policy's state is reported in the startup and hot-reload logs instead.
- **Not a strict parser.** Go's `yaml.v3` accepts ambiguous input like `key:{flow}` (no space after the colon) and silently produces the wrong shape, so a config can pass this check and still break downstream. Run a strict YAML linter as well.

**Implementation:** `internal/handlers/validate.go`

---

## 14. Hash & Reload

- `GET /api/hash` — returns current hash of all config files (SHA256 truncated to 16 chars)
- `POST /api/reload` — placeholder; currently no-op because config is re-read per request

**Implementation:** `internal/handlers/hash.go`, `internal/handlers/reload.go`

---

## 15. Health Check

- `GET /api/healthcheck` — returns `{"status": "OK"}`
- Used by Docker healthcheck (`wget -qO- http://localhost:3000/api/healthcheck`)

**Implementation:** `internal/handlers/health.go`

---

## 16. Email Allowlist (opt-in login)

Optional. A `config/auth.yaml` listing at least one address makes Google sign-in
mandatory for the whole dashboard; no file (or an empty list) leaves everything
public and byte-for-byte unchanged. There is no `enabled` flag — **the allowlist
is the switch**.

| Aspect | Behaviour |
|---|---|
| **Providers** | `google` (OAuth 2.0 Authorization Code) or `trustedHeader` (identity asserted by a proxy already doing SSO, honoured only when the peer is in `TRUSTED_PROXIES`) |
| **Dependencies** | None. The ID Token arrives over direct TLS from the token endpoint — the case OIDC Core §3.1.3.7(6) covers — so `iss`/`aud`/`exp`/`nonce`/`email_verified` are validated without JWKS or a JWT library |
| **Session** | Stateless: `email \| expiry \| nonce` signed with HMAC-SHA256. `HttpOnly` (mandatory — `custom.js` is operator JS on the same page), `SameSite=Lax`, `Secure`, sliding renewal past half-life |
| **Coverage** | Allowlist of public paths, not a denylist: `/static/*`, `/auth/*`, `/api/healthcheck` and `publicPaths` are open; everything else needs a session, including the widget proxy and the scripts endpoints |
| **Responses** | `302` for navigation · `401` + `HX-Redirect` for HTMX (so a polling widget never paints a login form inside its card) · `401` JSON for API clients · `403` for an authenticated address that is not listed |
| **Hot-reload** | The policy is read per request. Adding an address grants access on the next request; removing one evicts that person immediately, not when their cookie expires |
| **Fail-closed** | A broken `auth.yaml` keeps the last known good allowlist; one that vanishes while sign-in is active locks down with 503. Only a well-formed, empty allowlist opens the dashboard. Bad config at startup is fatal; on hot-reload it never is |
| **Guard rails** | Public mail providers under `domains:` are rejected at startup unless `allowPublicDomains: true`; `redirectURL` is explicit and never derived from the `Host` header |

**Implementation:** `internal/config/auth.go` (policy), `internal/auth/` (allowlist,
session, providers), `internal/middleware/auth.go` (the gate),
`internal/handlers/auth.go` (endpoints), `internal/templates/login.templ`.

**Docs:** [`authentication.md`](./authentication.md).

---

## 17. Known Issues / Real Blockers

1. ~~Real `customapi` widget~~ — **DONE** (v1.2.0)
2. ~~Wire `MergeServices` + `DockerDiscoverer`~~ — **DONE** (v1.2.0)
3. ~~`GenericProxyHandler` queries the registry~~ — **DONE** (v1.2.0)
4. ~~Hot-reload of `scripts.yaml`~~ — **DONE** (v1.2.0)
5. **Audit log to append-only file** instead of stderr.
