# System Architecture — MyServer

> High-level view of the architecture, data flows, package dependencies, and key design decisions.

---

## 1. Technology Stack

| Layer | Technology |
|-------|-----------|
| **Language** | Go 1.25 |
| **HTTP Router** | chi/v5 |
| **Templating** | Templ (compiles to Go functions) |
| **Frontend Interactivity** | HTMX (server-driven updates) |
| **CSS** | Tailwind CSS v3 (standalone CLI) |
| **Logger** | zap (uber-go) |
| **Docker client** | github.com/docker/docker/client |
| **Ping** | github.com/go-ping/ping |
| **YAML** | gopkg.in/yaml.v3 |
| **Cache** | github.com/jellydator/ttlcache/v3 |
| **File watcher** | github.com/fsnotify/fsnotify |

---

## 2. Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         CLIENT                               │
│  (Browser with HTMX + Tailwind CSS + app.js)              │
└─────────────────────────┬───────────────────────────────────┘
                          │
          ┌───────────────┴───────────────┐
          │  Reverse Proxy / external auth │
          │  (optional — see the gate below)│
          └───────────────┬───────────────┘
                          │
┌─────────────────────────┴───────────────────────────────────┐
│                    MyServer (Go)                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  chi.Router                                         │   │
│  │  ├── Global MW: Recovery → Logging → SecurityHdr    │   │
│  │  │               → Auth gate → Compress             │   │
│  │  ├── API Routes (/api/*): CORS → HostValidation     │   │
│  │  │                       → RateLimit                │   │
│  │  ├── Static Files (/static/*)        [public]       │   │
│  │  ├── Auth (/auth/*)                  [public]       │   │
│  │  ├── Dashboard (GET /)                              │   │
│  │  └── Config Files (GET /api/config/*)               │   │
│  └─────────────────────────────────────────────────────┘   │
│   The Auth gate is a no-op while config/auth.yaml lists     │
│   nobody; with an allowlist it guards everything except     │
│   /static/*, /auth/* and /api/healthcheck.                  │
│                          │                                 │
│  ┌───────────────────────┼───────────────────────────────┐  │
│  │                       │                               │  │
│  │  ┌──────────────┐   │   ┌──────────────────────┐   │  │
│  │  │  Handlers    │◄──┘   │  Config (YAML + Env)  │   │  │
│  │  │  · Dashboard │       │  · LoadSettings()      │   │  │
│  │  │  · Proxy     │◄──────│  · LoadServices()      │   │  │
│  │  │  · Docker    │       │  · LoadBookmarks()     │   │  │
│  │  │  · Scripts   │       │  · LoadWidgets()       │   │  │
│  │  │  · Monitor   │       │  · SubstituteEnvVars() │   │  │
│  │  │  · Ping      │       └──────────────────────┘   │  │
│  │  │  · Widgets   │                                    │  │
│  │  └──────┬───────┘                                    │  │
│  │         │                                             │  │
│  │  ┌──────┴───────┐    ┌─────────────┐    ┌────────┐  │  │
│  │  │   Proxy      │◄──►│   Cache     │    │Widget  │  │  │
│  │  │  · Generic   │    │  (TTL)      │    │Registry│  │  │
│  │  │  · UniFi     │    └─────────────┘    └────┬───┘  │  │
│  │  │  · Synology  │                            │      │  │
│  │  │  · JSON-RPC  │    ┌────────────────────────┘      │  │
│  │  │  · Credentialed│   │                              │  │
│  │  └──────────────┘   │   ┌────────────────────┐      │  │
│  │                      └──►│ 46 Widget types    │      │  │
│  │                          │  · customapi        │      │  │
│  │  ┌──────────────┐        │  · plex, jellyfin   │      │  │
│  │  │  Discovery   │        │  · sonarr, radarr   │      │  │
│  │  │  · Docker    │        │  · docker, glances  │      │  │
│  │  │  · K8s (stub)│        └────────────────────┘      │  │
│  │  └──────────────┘                                     │  │
│  │                                                       │  │
│  │  ┌──────────────┐        ┌────────────────────┐     │  │
│  │  │  Scripts     │        │  Templates (Templ) │     │  │
│  │  │  · Manager   │        │  · Index, Layout     │     │  │
│  │  │  · Executor  │        │  · ServiceGroup      │     │  │
│  │  │  · Audit     │        │  · ServiceCard       │     │  │
│  │  └──────────────┘        │  · Bookmark          │     │  │
│  │                            │  · InfoWidgets       │     │  │
│  │                            │  · Scripts           │     │  │
│  │                            │  · i18n, helpers     │     │  │
│  │                            └────────────────────┘     │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                          │
          ┌───────────────┴───────────────┐
          │    Docker Daemon (optional)   │
          │  · Container stats & status     │
          │  · Service discovery via labels │
          └───────────────────────────────┘
```

---

## 3. Data Flow: Dashboard Render

```
1. Browser → GET /
2. handlers.Dashboard()
3.   ├─ config.LoadSettings()      → settings.yaml
4.   ├─ config.LoadServices()      → services.yaml
5.   ├─ config.LoadBookmarks()     → bookmarks.yaml
6.   ├─ config.LoadWidgets()       → widgets.yaml
7.   └─ config.CurrentHash()      → atomic hash
8. templates.Index(data PageData)
9.   ├─ templates.Layout(data)    → head, header, footer
10.  ├─ BookmarkGroup()           → bookmarks section
11.  └─ ServiceGroup()            → for each service group
12.      └─ ServiceCard()         → for each service
13.          ├─ PingHTML (hx-get)  → /api/ping
14.          ├─ SiteMonitorHTML     → /api/siteMonitor
15.          ├─ DockerStatsHTML    → /api/docker/stats
16.          ├─ DockerStatusHTML   → /api/docker/status
17.          └─ Widget partial     → /api/services/proxy
18. Browser receives full HTML
19. HTMX initializes hx-get / hx-post on the cards
20. Every X seconds, HTMX makes requests and receives partial HTML
```

**Critical note:** API handlers do content negotiation. If the request comes from HTMX (`HX-Request: true`), they return partial HTML (Templ). Otherwise, they return JSON.

---

## 4. Data Flow: Widget Proxy

```
1. Browser hx-get → /api/services/proxy?group=X&service=Y&endpoint=Z
2. handlers.Proxy()
3.   ├─ findServiceWidget(group, service) → finds WidgetConfig in services.yaml
4.   ├─ Validates endpoint against regex ^[a-zA-Z0-9_./-]*$
5.   └─ proxyhandlers.GenericProxyHandler()
6.       ├─ Queries widget registry for APITemplate + Mappings
7.       ├─ Resolves placeholders in Widget.URL: {url}, {endpoint}, {key}, etc.
8.       ├─ If URL is `file://` → reads local JSON directly from config/
9.       ├─ Otherwise adds authentication (Basic > Bearer), headers, body
10.      └─ proxy.Proxy(ctx, targetURL, params)
11.          ├─ Validates URL scheme (http/https/file)
12.          ├─ SSRF check: resolves DNS, blocks private IPs (loopback allowed if private hosts enabled)
13.          ├─ Decompresses gzip/zlib
14.          ├─ Caps body to 10 MiB
15.          ├─ scrubError (sanitizes credentials)
16.          └─ Returns Result{Status, Body, ContentType}
17.      Parses JSON from Body
18. handlers.Proxy()
19.   ├─ If HTMX + display: dynamic-list → templates.DynamicListHTML(items)
20.   └─ Otherwise → JSON encoder
```

---

## 5. Data Flow: Script Execution

```
1. Browser POST → /api/scripts/{name} (or /stream)
2. handlers.RunScript() / StreamScript()
3.   ├─ scriptsEnabled() check
4.   ├─ isOriginAllowed() → CSRF defense
5.   ├─ ScriptManager.Get(name) → ScriptConfig
6.   ├─ If requireConfirm → validates X-Homepage-Confirm: yes
7.   └─ ScriptManager.Run(ctx, name, clientIP)
8.       ├─ Concurrency check (per-name + global semaphore)
9.       ├─ Executor.Execute() / StreamOutput()
10.          ├─ buildCommand() → /bin/bash {resolvedPath}
11.          │   ├─ Minimal env (PATH, HOME=/tmp, USER, SHELL, TZ)
12.          │   ├─ Setpgid to kill process tree
13.          │   └─ SIGTERM → grace 5s → SIGKILL
14.          ├─ Captures stdout+stderr (cap 1 MiB)
15.          └─ Returns Execution{Status, Output, ExitCode, Duration}
16.      ├─ Stores in history (max 10 entries)
17.      ├─ AuditLogger.Log(ex)
18.      └─ Returns to handler
19. handlers.RunScript()
20.   ├─ If HTMX → templates.ScriptResult(svc, status, exitCode, output, duration, lang)
21.   └─ Otherwise → JSON
```

---

## 5b. Data Flow: Sign-In (when an allowlist is configured)

```
1.  Browser GET / (no session cookie)
2.  middleware.Auth() reads config.Auth() — per request, never a closure
3.    ├─ Lockdown?      → 503 everywhere except /api/healthcheck
4.    ├─ Not required?  → next handler, untouched (the public default)
5.    ├─ Public path?   → next handler (/static/*, /auth/*, healthcheck)
6.    └─ Otherwise → authenticate()
7.         ├─ provider google        → auth.ReadSession() (HMAC cookie)
8.         └─ provider trustedHeader → auth.EmailFromTrustedHeader()
9.                                     (only if PeerIsTrusted)
10.  No identity → challenge()
11.    ├─ HX-Request: true → 401 + HX-Redirect: /auth/login
12.    ├─ JSON client      → 401 {"error":"unauthorized"}
13.    └─ Navigation       → 302 /auth/login?next=…
14.  Identity → auth.IsAllowed() re-checked on EVERY request
15.    ├─ No  → 403 (a valid cookie is not a standing permission)
16.    └─ Yes → context carries the email; next handler

── The OAuth round trip ──────────────────────────────────────
17.  GET /auth/google/start
18.    ├─ crypto/rand state + nonce → __Host- cookie (10 min)
19.    └─ 302 accounts.google.com  (scope=openid email)
20.  GET /auth/google/callback
21.    ├─ state compared in constant time, cookie consumed
22.    ├─ POST oauth2.googleapis.com/token   (server to server, TLS)
23.    ├─ validate iss / aud / exp / nonce / email_verified
24.    ├─ IsAllowed(email)?  no → 302 /auth/denied, NO cookie issued
25.    └─ yes → HMAC session cookie → 302 to the validated `next`
```

---

## 6. Key Design Decisions

### A. In-Memory Config Cache

Config loaders (`LoadSettings`, `LoadServices`, etc.) cache results in `atomic.Value`. `ReloadCache()` invalidates all caches when the watcher detects a config change. Handlers call public `LoadXxx()` which returns cached data, while `loadXxx()` (private) reads from disk. This avoids reloading YAMLs on every request while maintaining hot-reload correctness.

### B. Content Negotiation via HX-Request

Instead of separate routes `/api/X` and `/htmx/X`, a single endpoint decides the format by header. This keeps the API surface small and allows both HTMX and JSON clients to use the same endpoints.

### C. SSRF Strict by Default, Self-Hosted Friendly

`proxy.Proxy` blocks RFC1918 by default. `HOMEPAGE_ALLOW_PRIVATE_HOSTS` defaults to `true` because the dominant use case is self-hosted where widgets point to internal services. Loopback (`127.0.0.1`, `::1`) is also allowed when private hosts are enabled, which is required for self-referencing widgets and local testing.

### D. Scripts: Defense in Depth + Hot Reload

- Feature is **opt-in** (`HOMEPAGE_SCRIPTS_ENABLED`)
- Endpoints are registered conditionally (if disabled, routes don't exist)
- Handler does double-check (defense in depth)
- `Origin` validation for CSRF
- `requireConfirm` enforced server-side with custom header
- Execution with minimal env, not inherited
- Path traversal safe with `EvalSymlinks`
- Only `.sh` allowed
- **Hot-reload**: `scripts.yaml` changes trigger `Manager.ReplaceAll()` automatically via the config watcher

### E. Auth: Public by Default, Opt-In Allowlist

The dashboard ships public and expects an auth layer in front (reverse proxy, Cloudflare Access, Authelia…). Since the email-allowlist feature it can also guard itself: a `config/auth.yaml` listing at least one address makes Google sign-in mandatory. There is no user database and no local passwords — identity always comes from an external provider (Google, or a header asserted by a trusted proxy).

The decisive property is what happens when that config cannot be read. `AuthConfig` lives in its **own** `atomic.Value`, not inside `cachedConfig`, because `ReloadCache` discards loader errors (`c.Settings, _ = loadSettings()`) — a policy that silently became `nil` would publish the dashboard. Instead: a broken file keeps the last known good allowlist, and a file that vanishes while sign-in is active locks everything down with 503. The only thing that opens the dashboard is a well-formed file with an empty allowlist.

See [`authentication.md`](./authentication.md).

### F. In-Memory Widget Registry

46 widget types (plus 4 aliases) are registered in a `map[string]Widget` at startup. `GenericProxyHandler` queries the registry for `APITemplate()` and `Mappings()` to build requests dynamically. Widgets are lightweight definitions (API template + endpoint mappings), not heavy instances.

---

## 7. Package Dependencies

```
cmd/myserver
    ├── internal/config          (all packages depend on config)
    │       ├── skeleton/          (embed.FS with default YAMLs)
    │       ├── watcher.go         (fsnotify)
    │       └── *.go              (types + loaders)
    ├── internal/handlers
    │       ├── api.go            (main router)
    │       ├── pages.go          (depends on templates)
    │       ├── proxy.go          (depends on proxy/, widgets/)
    │       ├── docker.go         (depends on docker client)
    │       ├── scripts.go        (depends on scripts/)
    │       └── ...
    ├── internal/templates
    │       ├── *.templ           (compiled to *_templ.go)
    │       ├── helpers.go         (URLs, icons, layout)
    │       ├── i18n.go           (EN/ES translations)
    │       └── types.go          (PageData, etc.)
    ├── internal/widgets
    │       ├── registry.go        (global registry)
    │       ├── types.go          (interfaces)
    │       └── *.go              (per-widget definitions)
    ├── internal/proxy
    │       ├── proxy.go          (secure HTTP proxy)
    │       ├── cache.go          (TTL cache)
    │       └── handlers/
    │           ├── generic.go
    │           ├── credentialed.go
    │           ├── jsonrpc.go
    │           ├── unifi.go
    │           └── synology.go
    ├── internal/auth
    │       ├── allowlist.go      (email/domain matching)
    │       ├── session.go        (signed stateless cookie)
    │       ├── google.go         (OAuth code exchange + claim validation)
    │       └── trustedheader.go  (identity from a front proxy)
    ├── internal/scripts
    │       ├── manager.go        (registration + execution)
    │       ├── executor.go       (os/exec)
    │       ├── types.go          (ScriptConfig, Execution)
    │       └── audit.go          (execution logging)
    ├── internal/discovery
    │       ├── docker.go         (docker client labels)
    │       ├── kubernetes.go     (stub)
    │       └── merger.go         (merge config + discovery)
    └── internal/middleware
            ├── recovery.go
            ├── logging.go
            ├── cors.go
            ├── host_validation.go
            ├── rate_limit.go
            ├── auth.go           (the allowlist gate)
            └── security_headers.go
```

**Rule:** `internal/config` does not depend on any other internal package. It is the foundation for all others.

**Corollary:** `internal/auth` depends only on `internal/config`, so `internal/middleware` can import it for the gate without a cycle. The trusted-proxy check stays in `middleware` (only it can see the peer) and is passed into `auth` as a parameter.

---

## 8. Extension Points

### Adding a New Widget

1. Create `internal/widgets/mywidget.go`
2. Implement `RegisterMyWidget(r *Registry)` with `reg(r, "mywidget", "{url}", mappings)`
3. Call `RegisterMyWidget(r)` in `RegisterBuiltinWidgets()` in `registry.go`
4. If it requires custom proxy, implement `ProxyHandler` and assign it to `BaseWidget.Proxy`

### Adding a New HTTP Handler

1. Create function in `internal/handlers/myhandler.go`
2. Register it in `internal/handlers/api.go` inside `r.Route("/api", ...)`
3. Apply middlewares if needed

### Adding Translations

1. Edit `internal/templates/i18n.go`
2. Add key to `translations["en"]` and `translations["es"]` maps
3. Use `T(lang, "my.new.key")` in templates

### Adding a New Setting Field

1. Edit `internal/config/settings.go` → add field to `Settings` struct
2. Apply default if needed in `LoadSettings()`
3. Use in templates or handlers

---

## 9. Performance Targets

- **Runtime RAM:** ~50-80 MiB
- **Build output:** Static binary with `-ldflags="-s -w"`
- **Docker limits:** 256M mem / 1.0 CPU
- **Healthcheck:** `wget` to `/api/healthcheck` every 30s
- **Proxy body cap:** 1 MiB input, 10 MiB output
- **Script output cap:** 1 MiB per execution
- **Script max concurrent:** 5 by default

