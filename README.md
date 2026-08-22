# MyServer

**Centralized self-hosted dashboard.** A Go rewrite of [Homepage](https://gethomepage.dev/)
(Next.js / React) using **Templ + HTMX + Tailwind CSS** — same look & feel,
100% compatible with the original Homepage YAML files, ~3× less RAM, single
~14 MB binary, ~30 MB Docker image.

<!-- <p align="center"><img src="#" alt="MyServer dashboard" /></p> -->

---

## Why MyServer

| | |
|---|---|
| ⚡ **Lightweight** | ~10–30 MiB RAM at runtime, single static binary, no Node.js at build or runtime. |
| 🧩 **Zero-code customization** | Everything lives in `config/` — YAMLs, scripts, local JSON. Edit on the host, save, the dashboard hot-reloads. |
| 🛡️ **Secure by default** | CSP, HostValidation, per-IP rate limits, SSRF guard, credential sanitization, sandboxed scripts. |
| 🔌 **160+ built-in widgets** | Sonarr, Radarr, Plex, Pi-hole, Proxmox, Portainer, … plus a `customapi` for anything else. |
| 🐳 **Container-aware** | Auto-discovery via Docker labels, live CPU/MEM/network stats, status badges, Swarm support. |
| 🌐 **No JS framework** | HTMX over server-rendered HTML. The dashboard works without a build pipeline. |

---

## Features at a glance

### Dashboard

- Responsive grid with per-group layout (`columns`, auto-fit, tabs in progress).
- Whole card clickable, dark/light toggle, **23 color themes**, no FOUC.
- **Background image** — full URL, data URI, or any image dropped into
  `config/` (`.png` / `.jpg` / `.jpeg` / `.webp` / `.gif` / `.svg` /
  `.avif` / `.ico` / `.bmp`).
- Bookmarks with automatic Simple-Icons defaults.
- **Unified search bar** — web search on Enter + live QuickLaunch dropdown
  that filters services and bookmarks as you type (↑ / ↓ to navigate).
- Custom CSS / JS injected from `config/custom.css` / `config/custom.js`.

### Live data

- System resource bars in the top bar: CPU%, temperature, uptime, RAM, disk
  per mount — color-changing by threshold.
- Weather without an API key (Open-Meteo) — multiple cities supported.
- `datetime` and `greeting` widgets, i18n EN / ES.
- Docker / Podman per-service stats (CPU%, MEM, RX/TX) refreshed every 5 s.
- ICMP ping in unprivileged UDP mode + HTTP site-monitor (HEAD with GET
  fallback).

### Widgets & data sources

- **160+ pre-registered widgets** (Sonarr, Radarr, Plex, Jellyfin, Pi-hole,
  Traefik, Proxmox, Portainer, Uptime Kuma, …).
- `customapi` widget for any JSON API, with display modes `text`, `list`,
  `dynamic-list`, `graph`, `tile`, field-path traversal, and format helpers
  (number / bytes / date / percent / duration).
- **`file://` scheme** — widgets can read local JSON directly from
  `config/data/` without an HTTP round-trip and hot-reload on save.

### Scripts (opt-in)

- Run shell scripts from the dashboard. Strict whitelist, path-traversal
  safe, env scrubbed, 1 MiB output cap, global concurrency semaphore.
- `requireConfirm: true` enforced server-side via the
  `X-Homepage-Confirm: yes` header.
- Optional SSE output streaming and per-execution audit log.

### Operations

- **Hot reload** via `fsnotify` — every YAML / CSS / JS change is reflected
  without a restart.
- **Service discovery** — containers with `homepage.*` labels are
  auto-merged with `services.yaml` (config wins on conflict).
- **In-memory cache** for parsed configs, proxy responses, DNS SSRF
  lookups, and Docker clients.
- **Gzip compression** on every HTML/JSON/CSS/JS response.

---

## Quick start

```bash
# 1) Build + run with hot reload (templ + tailwind + go)
make dev

# 2) Or the Docker compose dev stack (bind-mounts ./config from the host)
make up && make logs

# 3) Run the binary directly (after `make build`)
./myserver -port 3000

# 4) Generate a full demo dashboard (every widget, scripts, customapi
#    display modes, background image, etc.)
./bootstrap-demo-config.sh
```

The dashboard listens on `:3000` by default — override with `-port`.

> **Deploying behind a public hostname?** Set
> `HOMEPAGE_ALLOWED_HOSTS=your.domain.com` (comma-separated, port-aware).
> Without it, MyServer accepts `localhost` only — the HTML cards render but
> every HTMX call to `/api/*` (Docker stats, resources, `customapi`)
> returns 403 and the dashboard looks empty. See
> [`docs/context/troubleshooting.md`](docs/context/troubleshooting.md#host-validation-failed).

---

## Configuration in 30 seconds

Drop YAMLs into `config/`:

```yaml
# config/settings.yaml
title: My Dashboard
theme: dark
color: slate
backgroundImage: wallpaper.jpg          # local file under config/ — or full URL

# config/services.yaml
- Applications:
    - Plex:
        href: https://plex.example.com
        icon: si:plex
        widget:
          type: plex
          url: http://plex:32400
          key: "{{HOMEPAGE_VAR_PLEX_TOKEN}}"

# config/widgets.yaml
- search:    { provider: google }
- datetime:  { format: { dateStyle: long, timeStyle: short } }
- openmeteo: { latitude: 51.5074, longitude: -0.1278, units: metric }
- resources: { label: CPU, cpu: true, memory: true, disk: / }
```

Save → `fsnotify` picks up the change → browser reloads within ~10 s.

Full schema, every widget type, env vars, and the icon resolver →
[`docs/context/configuration.md`](docs/context/configuration.md).

---

## Security model in one paragraph

The dashboard ships **without internal auth** — production deployments put
an external auth layer in front (Cloudflare Access, Authelia, oauth2-proxy,
…). The container itself enforces: host validation (`HOMEPAGE_ALLOWED_HOSTS`),
same-origin CORS only on `/api/*`, per-IP token-bucket rate limiting,
strict CSP + security headers (HSTS opt-in), an SSRF guard on the widget
proxy with cloud-metadata-IP blocking, and recursive credential
sanitization on `/api/services` and `/api/widgets`. The scripts feature is
opt-in (`HOMEPAGE_SCRIPTS_ENABLED=true`), `.sh`-only, sandboxed with a
minimal env, path-traversal-safe, and demands a server-side confirmation
header for any destructive script.

Full details: [`docs/context/configuration.md#environment-variables`](docs/context/configuration.md#environment-variables).

---

## Where to go next

| If you want to… | Open |
|---|---|
| Add a service / bookmark / widget without touching Go | [Agent skill — `add-widget`](.agents/skills/add-widget/SKILL.md) + [`COOKBOOK.md`](.agents/skills/add-widget/COOKBOOK.md) |
| Read the full YAML schema | [`docs/context/configuration.md`](docs/context/configuration.md) |
| Copy a working config to start from | [`.agents/skills/add-widget/templates/`](.agents/skills/add-widget/templates/) · [`bootstrap-demo-config.sh`](bootstrap-demo-config.sh) |
| Use the scripts feature | [`docs/context/scripts.md`](docs/context/scripts.md) |
| Hit the HTTP API directly | [`docs/context/api.md`](docs/context/api.md) |
| Deploy to production, or run locally | [`docs/context/deploy.md`](docs/context/deploy.md) |
| Diagnose a broken widget / icon / hot-reload | [`docs/context/troubleshooting.md`](docs/context/troubleshooting.md) |
| Understand the package layout and data flows | [`docs/context/architecture.md`](docs/context/architecture.md) · [`directories.md`](docs/context/directories.md) |
| See what's changed | [`CHANGELOG.md`](CHANGELOG.md) |

---

## Stack

Go 1.25 · `chi` router · [Templ](https://templ.guide/) ·
[HTMX](https://htmx.org/) · Tailwind CSS v3 (standalone CLI) ·
`fsnotify` · `ttlcache/v3` · `zap` · `go-ping`.

Single multi-stage Docker build · `alpine:3.21` runtime · non-root user
(`myserver:1000`) · `tini` PID 1.

---

## References

- **Homepage (upstream)** — https://gethomepage.dev/
- **Templ** — https://templ.guide/
- **HTMX** — https://htmx.org/
- **dashboard-icons** (homarr-labs) — https://github.com/homarr-labs/dashboard-icons
