# Directory Structure — MyServer

> Complete reference of the project's file and directory structure.

---

## Project Root

```
myserver/                              # repository root
├── cmd/myserver/
│   └── main.go                       # Entry point. Initializes everything and starts the HTTP server.
│
├── internal/
│   ├── config/                       # YAML parsing, env var substitution, watcher
│   │   ├── skeleton/                 # Default YAMLs embedded (go:embed)
│   │   │   ├── bookmarks.yaml
│   │   │   ├── auth.yaml          # optional — presence of an allowlist requires login
│   │   │   ├── docker.yaml
│   │   │   ├── kubernetes.yaml
│   │   │   ├── proxmox.yaml
│   │   │   ├── scripts.yaml
│   │   │   ├── services.yaml
│   │   │   ├── settings.yaml
│   │   │   └── widgets.yaml
│   │   ├── bookmarks.go             # Bookmark, BookmarkGroup, LoadBookmarks()
│   │   ├── config.go                # ConfigDir, ConfigHash, CurrentHash/SetCurrentHash, EnsureConfigDir, CheckAndCopyConfig, ReadConfigFile
│   │   ├── config_test.go           # Config tests
│   │   ├── docker.go                # DockerConfig, TLSConfig, LoadDocker()
│   │   ├── env.go                   # SubstituteEnvVars, RawAllowedHosts, AllowedHosts, ProxyDisableIPv6, ScriptsEnabled, AllowPrivateHosts
│   │   ├── env_test.go              # Env substitution tests
│   │   ├── kubernetes.go            # KubernetesConfig, LoadKubernetes()
│   │   ├── proxmox.go             # ProxmoxConfig, LoadProxmox()
│   │   ├── scripts.go             # ScriptsFile, ScriptEntry, LoadScriptsFile()
│   │   ├── services.go            # Service, ServiceGroup, WidgetConfig, LoadServices(), SanitizeService()
│   │   ├── services_test.go       # Services tests
│   │   ├── settings.go            # Settings, LayoutGroup, QuickLaunch, ScriptSettings, LoadSettings()
│   │   ├── settings_test.go       # Settings tests
│   │   ├── watcher.go             # Watcher (fsnotify), Start/Stop/eventLoop
│   │   └── widgets.go             # InfoWidget, LoadWidgets(), IsSensitiveKey(), SanitizeWidgets()
│   │
│   ├── discovery/                  # Service discovery via Docker/Podman/K8s labels
│   │   ├── docker.go               # DockerDiscoverer, containerToService, swarmServiceToService
│   │   ├── kubernetes.go           # KubernetesDiscoverer (stub)
│   │   ├── merger.go               # MergeServices, sortServices, sortByLayout
│   │   └── merger_test.go          # Merger tests
│   │
│   ├── handlers/                   # All HTTP handlers
│   │   ├── api.go                  # Main router (chi). Registers all routes + middlewares.
│   │   ├── bookmarks.go            # GET /api/bookmarks
│   │   ├── config.go               # GET /api/config/{path}
│   │   ├── docker.go               # GET /api/docker/stats|status/{container}/{server}
│   │   ├── handlers_test.go        # Handler tests
│   │   ├── hash.go                 # GET /api/hash
│   │   ├── health.go               # GET /api/healthcheck
│   │   ├── kubernetes.go           # GET /api/kubernetes/stats|status (stub — not registered)
│   │   ├── monitor.go              # GET /api/siteMonitor
│   │   ├── pages.go                # GET / (Dashboard)
│   │   ├── ping.go                 # GET /api/ping
│   │   ├── proxmox.go              # GET /api/proxmox/stats/{vmid}/{server}
│   │   ├── proxy.go                # GET/POST /api/services/proxy
│   │   ├── reload.go               # POST /api/reload (no-op)
│   │   ├── scripts.go              # GET/POST /api/scripts/*
│   │   ├── services.go             # GET /api/services
│   │   ├── validate.go             # GET /api/validate
│   │   ├── widgets.go              # GET /api/widgets
│   │   ├── widgets_resources.go    # GET /api/widgets/resources
│   │   └── widgets_weather.go      # GET /api/widgets/openmeteo|weather
│   │
│   ├── middleware/                 # HTTP middlewares
│   │   ├── cors.go                 # Strict same-origin CORS
│   │   ├── host_validation.go      # HostValidation with port-awareness
│   │   ├── host_validation_test.go # Host validation tests
│   │   ├── logging.go              # Request logging with zap
│   │   └── recovery.go             # Panic recovery with stack trace
│   │
│   ├── proxy/                      # Secure HTTP proxy client
│   │   ├── cache.go                # TTL cache for proxy responses
│   │   ├── proxy.go                # Proxy(), scrubError, checkSSRF, SanitizeURL, FormatAPICall, IsJSON
│   │   ├── proxy_test.go           # Proxy tests
│   │   └── handlers/               # Specialized proxy handlers
│   │       ├── credentialed.go     # Form POST login + cookie jar
│   │       ├── generic.go          # GenericProxyHandler (placeholder substitution, auth, request)
│   │       ├── jsonrpc.go          # JSON-RPC 2.0 proxy (Deluge, Transmission)
│   │       ├── synology.go         # Synology SID-based auth
│   │       └── unifi.go            # UniFi cookie auth + TLS skip
│   │
│   ├── scripts/                    # Script execution (opt-in feature)
│   │   ├── audit.go                # AuditLogger (writes to stderr)
│   │   ├── executor.go             # Executor (os/exec), buildCommand, Execute, StreamOutput
│   │   ├── manager.go              # Manager (registration, Run, Stream, Status, validateScript)
│   │   ├── manager_test.go         # Manager tests (path traversal, .sh, env denylist, race, etc.)
│   │   ├── types.go                # ScriptConfig, Execution, ScriptStatus
│   │   └── ... (executor has no separate test; tested via manager)
│   │
│   ├── templates/                  # Templ templates + helpers
│   │   ├── bookmark.templ          # BookmarkGroup template
│   │   ├── bookmark_templ.go       # Compiled (generated)
│   │   ├── footer.templ            # Footer template
│   │   ├── footer_templ.go         # Compiled
│   │   ├── head.templ              # Head (meta, css, custom css/js)
│   │   ├── head_templ.go           # Compiled
│   │   ├── header.templ            # Header (title, greeting, search, quicklaunch)
│   │   ├── header_templ.go         # Compiled
│   │   ├── format.go               # Formatting helpers: FormatBytes, FormatPercent, FormatDuration, etc.
│   │   ├── icons.go                # Icon resolution: iconURL, defaultBookmarkIcon, resolveBookmarkIcon
│   │   ├── i18n.go                 # EN/ES translations
│   │   ├── index.templ             # Main template (bookmarks + service groups)
│   │   ├── index_templ.go          # Compiled
│   │   ├── info_widgets.templ      # Info widgets (datetime, greeting, search, weather, resources)
│   │   ├── info_widgets_templ.go   # Compiled
│   │   ├── layout.go               # Layout helpers: layoutForGroup, linkTarget
│   │   ├── layout.templ            # Base layout (html, body, head, footer wrapper)
│   │   ├── layout_templ.go         # Compiled
│   │   ├── scripts.templ           # Script cards + results
│   │   ├── scripts_templ.go          # Compiled
│   │   ├── script_card.templ       # Individual script card
│   │   ├── script_card_templ.go    # Compiled
│   │   ├── service_card.templ      # Service card (icon, name, ping, status, widget)
│   │   ├── service_card_templ.go   # Compiled
│   │   ├── service_group.templ     # Service group (tab or section)
│   │   ├── service_group_templ.go  # Compiled
│   │   ├── styles.go               # Visual helpers: gridColumns, barWidth, datetimeLocale
│   │   ├── types.go                # PageData, TabGroup, DynamicListItem, ResourceBarData
│   │   ├── urls.go                 # URL builders: pingURL, proxyURL, dockerStatsURL, etc.
│   │   ├── widget.templ            # Generic widget partial
│   │   └── widget_templ.go         # Compiled
│   │
│   └── widgets/                    # Widget definitions (registry)
│       ├── customapi.go            # customapi widget (real implementation with mappings)
│       ├── docker.go               # docker widget
│       ├── downloads.go            # qbittorrent, transmission, deluge, sabnzbd
│       ├── glances.go              # glances widget
│       ├── info.go                 # datetime, greeting, search, weather, openmeteo, stocks, k8s-info, longhorn
│       ├── infrastructure.go       # proxmox, argocd
│       ├── media.go                # plex, jellyfin, emby, tautulli
│       ├── monitoring.go           # portainer, uptimekuma, netdata, prometheus, grafana
│       ├── networking.go           # pihole, adguard, traefik, caddy, npm, cloudflared, tailscale
│       ├── photoprism.go           # photoprism widget
│       ├── productivity.go           # nextcloud, trilium, paperlessngx
│       ├── registry.go             # Registry, DefaultRegistry, RegisterBuiltinWidgets(), simpleWidget
│       ├── registry_test.go        # Registry tests
│       ├── resources.go            # resources widget (local system)
│       ├── speedtest.go            # speedtest widget
│       ├── types.go                # Widget interface, EndpointMapping, BaseWidget, ProxyHandler
│       ├── vikunja.go              # vikunja widget
│       └── ... (one file per category)
│
├── web/
│   ├── static/
│   │   ├── css/
│   │   │   └── main.css            # Generated by Tailwind CSS (DO NOT edit manually)
│   │   ├── js/
│   │   │   └── app.js              # Frontend JS: hot-reload, datetime, greeting, HTMX setup, search, quicklaunch
│   │   └── themes.css              # CSS theme variables (colors, dark/light)
│   └── tailwind/
│       └── input.css               # Tailwind source: @layer base/components with @apply
│
├── scripts/                        # User .sh executable scripts (opt-in feature)
│   └── (user scripts)
│
├── docs/
│   └── context/
│       ├── workflow.md
│       ├── features.md
│       ├── glosario-controladores.md
│       ├── glosario-funciones.md
│       ├── arquitectura.md
│       └── directorios.md
│
├── .air.toml                       # Air config (hot reload)
├── .dockerignore
├── .githooks/                      # Git hooks (pre-commit: lint + test-race)
│   └── pre-commit
├── .github/
│   └── workflows/
│       ├── ci.yml                  # GitHub Actions CI (lint, test, build)
│       └── release.yml             # Release workflow (goreleaser)
├── .gitignore
├── CHANGELOG.md
├── CLAUDE.md                       # Agent documentation for this project
├── Dockerfile                      # Multi-stage build (Go builder → alpine runtime)
├── Makefile                        # Build, test, lint, dev, docker commands
├── README.md                       # User documentation (1200+ lines)
├── docker-compose.yml              # Docker Compose deployment
├── docker-entrypoint.sh            # Entrypoint with su-exec to myserver user
├── go.mod                          # Go module
├── go.sum                          # Checksums
└── tailwind.config.js              # Tailwind config: content paths + safelist
```

---

## Naming Conventions

| Pattern | Meaning | Example |
|---------|---------|---------|
| `*_templ.go` | File generated by Templ compiler. DO NOT edit manually. | `index_templ.go` |
| `*_test.go` | Go tests (unit). Placed next to the code they test. | `manager_test.go` |
| `*.templ` | Source template in Templ syntax. | `index.templ` |
| `*.yaml` (in skeleton/) | Default configurations embedded in the binary. | `settings.yaml` |
| `*.yaml` (in /app/config) | User configurations (runtime). | `services.yaml` |

---

## Critical Deployment Directories

| Directory (container) | Content | Mount Type |
|-----------------------|---------|------------|
| `/app/config` | User YAMLs + scripts + data | **Host bind mount** (`/srv/myserver/config:/app/config`) |
| `/app/config/scripts` | `.sh` executable scripts | Inside bind mount |
| `/app/config/data` | Local JSON data sources | Inside bind mount |
| `/app/web/static` | Compiled assets (CSS, JS) | Copied at build time |
| `/var/run/docker.sock` | Docker socket | Bind mount from host |

---

## User Configuration Files (YAML)

| File | Structure | Loader | Purpose |
|------|-----------|--------|---------|
| `settings.yaml` | Flat map | `LoadSettings()` | Title, theme, color, language, layout, quicklaunch, scripts config |
| `services.yaml` | List of `{groupName: [{svcName: {fields}}]}` | `LoadServices()` | Dashboard service groups |
| `bookmarks.yaml` | List of `{groupName: [{bmName: {fields}}]}` | `LoadBookmarks()` | Dashboard bookmarks |
| `widgets.yaml` | List of `{widgetType: {options}}` | `LoadWidgets()` | Global widgets (datetime, search, weather) |
| `docker.yaml` | Map of `{serverName: {socket/host/port/tls}}` | `LoadDocker()` | Docker server configs |
| `kubernetes.yaml` | Map of `{serverName: {kubeconfig}}` | `LoadKubernetes()` | K8s cluster configs |
| `proxmox.yaml` | Map of `{serverName: {url, token, secret}}` | `LoadProxmox()` | Proxmox server configs |
| `scripts.yaml` | `{scripts: {name: {command, description, ...}}}` | `LoadScriptsFile()` | Script definitions |
| `auth.yaml` | `{allowlist, google, session, ...}` | `config.Auth()` | Optional email allowlist. Not in `cachedConfig`: it has its own atomic value with last-known-good semantics ([`authentication.md`](./authentication.md)) |
| `custom.css` | Plain CSS | — | Injected into `<head>` |
| `custom.js` | Plain JS | — | Injected before `</body>` |

---

## Generated Files (do not commit)

These files are regenerated on each build and **should not** be committed:

- `web/static/css/main.css` — generated by `tailwindcss`
- `myserver` (binary) — generated by `go build`
- `cover.out` — coverage report

**Note:** Files `*_templ.go` **ARE committed** because they are the output of the Templ compiler and must be available for `go build` without requiring `templ` to be installed.

