# Function & Utility Glossary

> Quick reference of key functions, methods, and utilities organized by package.

---

## `internal/config` — YAML Configuration

### General Config

| Function | File | Description |
|----------|------|-------------|
| `ConfigDir() string` | `config.go` | Returns config directory. Reads `HOMEPAGE_CONFIG_DIR`, default `/app/config`. Thread-safe with RWMutex. |
| `SetConfigDir(dir)` | `config.go` | Override for testing. |
| `ResetConfigDir()` | `config.go` | Reset for testing. |
| `EnsureConfigDir() error` | `config.go` | Creates config directory if it doesn't exist. |
| `CheckAndCopyConfig(filename) error` | `config.go` | If file doesn't exist, copies from `internal/config/skeleton/` (embed.FS). |
| `ReadConfigFile(filename) ([]byte, error)` | `config.go` | Reads file + applies `SubstituteEnvVars()`. |
| `ConfigHash() (string, error)` | `config.go` | SHA256 of all YAMLs + custom.css + custom.js. Truncated to 16 chars. Used for cache busting. |
| `CurrentHash() string` | `config.go` | Reads current hash from `atomic.Value`. Thread-safe. |
| `SetCurrentHash(h)` | `config.go` | Stores hash in `atomic.Value`. Called by the watcher. |

### Env Var Substitution

| Function | File | Description |
|----------|------|-------------|
| `SubstituteEnvVars(input string) string` | `env.go` | Replaces `{{HOMEPAGE_VAR_*}}` and `{{HOMEPAGE_FILE_*}}`. If unresolved, keeps placeholder literal. |
| `RawAllowedHosts() string` | `env.go` | Raw value of `HOMEPAGE_ALLOWED_HOSTS`. |
| `AllowedHosts() []string` | `env.go` | Parsed list of allowed hosts. |
| `ProxyDisableIPv6() bool` | `env.go` | `HOMEPAGE_PROXY_DISABLE_IPV6 == "true"`. |
| `ScriptsEnabled() bool` | `env.go` | `HOMEPAGE_SCRIPTS_ENABLED == "true"`. |
| `AllowPrivateHosts() bool` | `env.go` | Default `true` (self-hosted). `HOMEPAGE_ALLOW_PRIVATE_HOSTS`. |

### YAML Loaders

| Function | File | Description |
|----------|------|-------------|
| `LoadSettings() (*Settings, error)` | `settings.go` | Loads `settings.yaml`. Applies defaults (theme=dark, color=slate, lang=en, target=_blank, headerStyle=underlined). |
| `LoadServices() ([]ServiceGroup, error)` | `services.go` | Loads `services.yaml`. Special structure: list of `{groupName: [{svcName: {svcFields}}]}`. |
| `LoadBookmarks() ([]BookmarkGroup, error)` | `bookmarks.go` | Loads `bookmarks.yaml`. Same nested structure as services. |
| `LoadWidgets() ([]InfoWidget, error)` | `widgets.go` | Loads `widgets.yaml`. List of `{widgetType: {options}}`. |
| `LoadDocker() (map[string]DockerConfig, error)` | `docker.go` | Loads `docker.yaml` with Docker server configs. |
| `LoadKubernetes() (map[string]KubernetesConfig, error)` | `kubernetes.go` | Loads `kubernetes.yaml`. |
| `LoadProxmox() (map[string]ProxmoxConfig, error)` | `proxmox.go` | Loads `proxmox.yaml` with URL, token, secret. |
| `LoadScriptsFile() (*ScriptsFile, error)` | `scripts.go` | Loads `scripts.yaml` with script definitions. |

### Credential Sanitization

| Function | File | Description |
|----------|------|-------------|
| `SanitizeService(s Service) Service` | `services.go` | Strips widget credentials + basic-auth from URL + recursive sanitize of Body/Options. |
| `sanitizeWidgetURL(raw string) string` | `services.go` | Removes userinfo (basic-auth) and sensitive query params from URLs. |
| `SanitizeWidgets(widgets []InfoWidget) []InfoWidget` | `widgets.go` | Recursive strip of sensitive options in info widgets. |
| `IsSensitiveKey(key string) bool` | `widgets.go` | Case-insensitive substring match against sensitive patterns (key, token, secret, password, auth, etc.). |
| `sanitizeValue(v interface{}) interface{}` | `widgets.go` | Deep-clone recursive removal of sensitive keys in maps/slices. |

### Config Watcher

| Function/Method | File | Description |
|-----------------|------|-------------|
| `NewWatcher(logger) (*Watcher, error)` | `watcher.go` | Creates fsnotify watcher over the config directory. |
| `(*Watcher) Start(onChange func()) error` | `watcher.go` | Starts event loop goroutine. Reacts to Write/Create/Remove/Rename on `.yaml`, `.yml`, `.css`, `.js`. |
| `(*Watcher) Stop() error` | `watcher.go` | Stops the watcher. |

---

## `internal/handlers` — Shared Helpers

| Function | File | Description |
|----------|------|-------------|
| `isHTMXRequest(r) bool` | `docker.go` | `HX-Request == "true"` |
| `writeUpstreamError(w, msg)` | `docker.go` | Generic 502 |
| `writeJSONError(w, msg, status)` | `ping.go` | JSON error with key `"errors"` |
| `extractDynamicListItems(data, mappings) []DynamicListItem` | `proxy.go` | Extracts items from JSON response for `display: dynamic-list` mode. |
| `resolveTemplate(tpl, item) string` | `proxy.go` | Replaces `{field}` placeholders in template with JSON item values. |
| `stringifyJSON(v) string` | `proxy.go` | Formats JSON-decoded scalar as string. |
| `writeProxyError(w, err)` | `proxy.go` | Maps proxy errors to clean HTTP statuses without credential leaks. |
| `findServiceWidget(group, service) *WidgetConfig` | `proxy.go` | Looks up widget in services.yaml. |
| `findServiceByScript(name) *Service` | `scripts.go` | Looks up service by script name. |
| `preferredLang(r) string` | `scripts.go` | Reads lang from settings, fallback "en". |
| `clientIPFromRequest(r) string` | `scripts.go` | Real IP. Trusts XFF/X-Real-IP only if peer is loopback. |
| `isOriginAllowed(r) bool` | `scripts.go` | Same-origin check for script endpoints. |

---

## `internal/proxy` — Secure HTTP Proxy

| Function | File | Description |
|----------|------|-------------|
| `Proxy(ctx, rawURL, params) (*Result, error)` | `proxy.go` | HTTP request with SSRF guard, gzip/zlib decompress, body cap 10 MiB, error scrubbing. Supports `file://` scheme (reads local JSON). Shared transport pool. |
| `CachedRequest(ctx, rawURL, ttl)` | `proxy.go` | Back-compat, does NOT cache (uses Proxy directly). |
| `SetTestSkipSSRF(v bool)` | `proxy.go` | Test-only hook to bypass SSRF checks (needed for `httptest.Server` on loopback). |
| `scrubError(err) string` | `proxy.go` | Sanitizes credentials from URLs in error messages. |
| `checkSSRF(u) error` | `proxy.go` | Blocks private/loopback/link-local/multicast/cloud-metadata IPs. Requires DNS resolution. |
| `isBlockedIP(ip) bool` | `proxy.go` | Detailed IP blocking logic. RFC1918 blocked unless `AllowPrivateHosts()`. |
| `SanitizeURL(rawURL) string` | `proxy.go` | Removes credentials from URL and redacts sensitive query params. |
| `FormatAPICall(template, args) string` | `proxy.go` | Replaces `{key}` placeholders in template. |
| `IsJSON(contentType) bool` | `proxy.go` | `strings.Contains(contentType, "application/json")`. |
| `NewCookieJar() (http.CookieJar, error)` | `proxy.go` | Creates cookie jar for sessions. |
| `Doer` | `proxy.go` | Interface abstracting `http.Client.Do`. Used by `Proxy()` for testability. |
| `NewCache(defaultTTL) *Cache` | `cache.go` | TTL cache with capacity 500. |
| `CacheKey(name, group, service, index) string` | `cache.go` | Generates cache key. |
| `CachedJSON(cache, key) (map[string]interface{}, bool)` | `cache.go` | Reads cached JSON data. |
| `SetJSON(cache, key, data, ttl) error` | `cache.go` | Stores JSON in cache. |

---

## `internal/proxy/handlers` — Specialized Proxy Handlers

| Function | File | Description |
|----------|------|-------------|
| `GenericProxyHandler(ctx, widget, endpoint, body) (interface{}, error)` | `generic.go` | Main handler for widgets. Validates endpoint, substitutes placeholders, adds auth, makes request, parses JSON. |
| `NewCredentialedHandler(loginURL, usernameKey, passwordKey) *CredentialedProxyHandler` | `credentialed.go` | Creates handler that does form POST login + cookie jar before API request. |
| `JSONRPCProxyHandler(ctx, widget, method, params) (interface{}, error)` | `jsonrpc.go` | JSON-RPC 2.0 handler (Deluge, Transmission). |
| `UniFiProxyHandler(ctx, widget, endpoint) (interface{}, error)` | `unifi.go` | Cookie-based login + TLS skip verify for UniFi. |
| `SynologyProxyHandler(ctx, widget, endpoint) (interface{}, error)` | `synology.go` | SID-based login + async logout for Synology. |

---

## `internal/templates` — Template Helpers

### URL Helpers

| Function | File | Description |
|----------|------|-------------|
| `pingURL(group, service) string` | `urls.go` | `/api/ping?groupName=X&serviceName=Y` |
| `siteMonitorURL(group, service) string` | `urls.go` | `/api/siteMonitor?groupName=X&serviceName=Y` |
| `proxyURL(group, service, endpoint) string` | `urls.go` | `/api/services/proxy?group=X&service=Y&endpoint=Z` |
| `dockerStatsURL(container, server) string` | `urls.go` | `/api/docker/stats/{container}/{server}` |
| `dockerStatusURL(container, server) string` | `urls.go` | `/api/docker/status/{container}/{server}` |
| `resourcesURL(opts) string` | `urls.go` | `/api/widgets/resources?cpu=true&memory=...` |
| `weatherURL(opts) string` | `urls.go` | `/api/widgets/openmeteo?latitude=...` |

### Icon Helpers

| Function | File | Description |
|----------|------|-------------|
| `iconURL(icon) string` | `icons.go` | Resolves icons: full URLs → verbatim; `mdi:xxx` / `mdi-xxx` → jsdelivr; `si:xxx` / `si-xxx` → simpleicons; default → dashboard-icons CDN (homarr-labs). Supports png/svg/webp. |
| `defaultBookmarkIcon(name string) string` | `icons.go` | Returns default Simple-Icons identifier for common services (e.g., `proxmox` → `si-proxmox`). |
| `resolveBookmarkIcon(svc *Service) string` | `icons.go` | Resolves icon for a service: explicit → default → empty. |

### Visual Helpers

| Function | File | Description |
|----------|------|-------------|
| `gridColumns(columns int) string` | `styles.go` | Returns complete CSS declaration `grid-template-columns: ...;` |
| `barWidth(percent float64) string` | `styles.go` | Returns `width: N%;` with clamp [0,100]. |
| `datetimeLocale(lang) string` | `styles.go` | Maps `es` → `es-ES`, default `en-US`. |

### Layout Helpers

| Function | File | Description |
|----------|------|-------------|
| `linkTarget(svc) string` | `layout.go` | Returns `"_blank"` (hardcoded). |
| `layoutForGroup(settings, groupName) *LayoutGroup` | `layout.go` | Looks up layout config for a group. |

### i18n

| Function | File | Description |
|----------|------|-------------|
| `T(lang, key) string` | `i18n.go` | Translation with English fallback. |

### Formatting

| Function | File | Description |
|----------|------|-------------|
| `FormatBytes(bytes float64) string` | `format.go` | B → KB → MB → GB → TB with 1 decimal. |
| `FormatPercent(value float64) string` | `format.go` | `%.1f%%` |
| `FormatDuration(seconds float64) string` | `format.go` | `Xd`, `Xh`, `Xm`, `Xs` |
| `FormatLatency(ms int64) string` | `format.go` | `Xms` or `Xs` |
| `FormatStatusCode(status int) string` | `format.go` | `"ERR"` if <=0, otherwise number. |
| `FormatTemp(c float64) string` | `format.go` | `%.0f°C` |

---

## `internal/widgets` — Widget Registry

| Function/Method | File | Description |
|-----------------|------|-------------|
| `RegisterBuiltinWidgets()` | `registry.go` | Registers all built-in widgets + aliases. Must be called before using the registry. |
| `NewRegistry() *Registry` | `registry.go` | Creates new registry. |
| `(*Registry) Register(w Widget)` | `registry.go` | Adds widget to registry. |
| `(*Registry) RegisterAlias(alias, target)` | `registry.go` | Registers alias (backward compatibility). |
| `(*Registry) Get(name) (Widget, bool)` | `registry.go` | Gets widget by name, resolving aliases. |
| `(*Registry) GetProxyHandler(name) ProxyHandler` | `registry.go` | Gets proxy handler for a widget. |
| `(*Registry) List() []string` | `registry.go` | Lists registered widget names. |
| `(*Registry) Has(name) bool` | `registry.go` | Checks if widget exists. |
| `reg(r, typeName, api, mappings)` | `registry.go` | Helper to register `simpleWidget` with API template and mappings. |

---

## `internal/scripts` — Script Execution

### Manager

| Method | File | Description |
|--------|------|-------------|
| `NewManager(dirs, defaultTimeout, maxTimeout, maxConcurrent) *Manager` | `manager.go` | Creates manager with whitelisted dirs, timeouts, and concurrency semaphore. |
| `(*Manager) Register(cfg) error` | `manager.go` | Validates and registers script. Rejects absolute paths, non-.sh, dangerous env vars. |
| `(*Manager) ReplaceAll(newScripts) []error` | `manager.go` | Atomic registry replacement (for future hot-reload). |
| `(*Manager) Has(name) bool` | `manager.go` | Checks if script exists. |
| `(*Manager) Get(name) (*ScriptConfig, bool)` | `manager.go` | Gets script config. |
| `(*Manager) List() []ScriptStatus` | `manager.go` | Lists statuses of all scripts (safe copy). |
| `(*Manager) Run(ctx, name, clientIP) (*Execution, error)` | `manager.go` | Synchronous execution. Per-name and global concurrency control. Timeout with context. History limited to 10 entries. |
| `(*Manager) Stream(ctx, name, clientIP) (<-chan string, error)` | `manager.go` | Execution with SSE streaming. Output capped at 1 MiB. |
| `(*Manager) Status(name) (*ScriptStatus, error)` | `manager.go` | Current status of a script. |

### Executor

| Method | File | Description |
|--------|------|-------------|
| `NewExecutor() *Executor` | `executor.go` | Creates executor. |
| `(*Executor) Execute(ctx, cfg, clientIP) *Execution` | `executor.go` | Executes script, captures combined stdout+stderr. Returns complete Execution. |
| `(*Executor) StreamOutput(ctx, cfg, lines) *Execution` | `executor.go` | Executes script and sends output lines to channel in real time. |

### Audit

| Method | File | Description |
|--------|------|-------------|
| `NewAuditLogger() *AuditLogger` | `audit.go` | Creates logger that writes to stderr with prefix `[SCRIPTS AUDIT]`. |
| `(*AuditLogger) Log(ex *Execution)` | `audit.go` | Logs execution with name, status, exit_code, duration, client_ip, started_at. |

---

## `internal/discovery` — Service Discovery

| Function/Method | File | Description |
|-----------------|------|-------------|
| `NewDockerDiscoverer(cfg) (*DockerDiscoverer, error)` | `docker.go` | Creates Docker client (unix socket, tcp, or FromEnv). Negotiates API version. |
| `(*DockerDiscoverer) DiscoverServices(ctx) ([]ServiceGroup, error)` | `docker.go` | Lists containers with `homepage.*` labels. If Swarm=true, also lists swarm services. |
| `(*DockerDiscoverer) Close() error` | `docker.go` | Closes Docker client. |
| `containerToService(c) *Service` | `docker.go` | Extracts Service from container labels. |
| `swarmServiceToService(s) *Service` | `docker.go` | Extracts Service from swarm service labels. |
| `NewKubernetesDiscoverer(cfg) (*KubernetesDiscoverer, error)` | `kubernetes.go` | Stub. |
| `(*KubernetesDiscoverer) DiscoverServices() ([]ServiceGroup, error)` | `kubernetes.go` | Stub — returns nil. |
| `MergeServices(configGroups, dockerGroups, layout) []ServiceGroup` | `merger.go` | Merge: config takes priority over Docker. Sorts by weight then name. Groups ordered by layout alphabetically (known limitation). |
| `findServiceByName(services, name) *Service` | `merger.go` | Linear search by name. |
| `mergeService(dst, src)` | `merger.go` | Merges non-empty fields from src to dst. |
| `sortServices(services)` | `merger.go` | Stable sort by weight asc, then name asc. |
| `sortByLayout(merged, layout) []ServiceGroup` | `merger.go` | Sorts groups: layout-defined first (alphabetically), then remaining alphabetically. |

---

## `internal/middleware` — HTTP Middleware

| Function | File | Description |
|----------|------|-------------|
| `Recovery(logger) func(http.Handler) http.Handler` | `recovery.go` | Recovers panics. Logs error + stack. Returns 500 adapted to Content-Type (JSON/HTML/text). |
| `Logging(logger) func(http.Handler) http.Handler` | `logging.go` | Logs method, path, duration, remote addr. Debug level. |
| `CORS(next) http.Handler` | `cors.go` | Strict same-origin CORS. Only for `/api/*`. Preflight OPTIONS → 204. Allowed headers: Content-Type, X-Homepage-Confirm, HX-Request, HX-Current-URL. |
| `HostValidation(port, logger) func(http.Handler) http.Handler` | `host_validation.go` | Validates Host header. Defaults: localhost:PORT, 127.0.0.1:PORT, [::1]:PORT. `*` = allow all. Port-aware. Case-insensitive. |

---

## `cmd/myserver` — Entry Point

| Function | File | Description |
|----------|------|-------------|
| `main()` | `main.go` | Order: 1) init logger, 2) ensure config dir, 3) RegisterBuiltinWidgets, 4) compute initial hash, 5) start config watcher, 6) init script manager (opt-in), 7) build router, 8) start server, 9) graceful shutdown with 10s timeout. |
