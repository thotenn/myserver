# API Reference

> Every HTTP endpoint MyServer exposes, grouped by purpose, with method,
> path parameters, content-negotiation behaviour, and rate limit.

Handlers do **content negotiation** via the `HX-Request` header:

- `HX-Request: true` → partial HTML (Templ, for `innerHTML` swaps).
- otherwise → JSON (for API clients).

---

## Core

| Method | Path | Description | Rate |
|---|---|---|---|
| GET  | `/` | Dashboard HTML (SSR with Templ). | — |
| GET  | `/api/healthcheck` | 200 OK. Used by Docker healthcheck. | — |
| GET  | `/api/hash` | Current config hash for hot-reload polling. | 1/s |
| POST | `/api/reload` | Reload YAML config. | — |
| GET  | `/api/services` | Sanitized services (config + Docker discovery merged). | — |
| GET  | `/api/bookmarks` | Bookmarks. | — |
| GET  | `/api/widgets` | Sanitized info widgets (credentials stripped). | — |
| GET  | `/api/validate` | Parse all YAMLs; report errors. | — |
| GET  | `/api/config/{path}` | Serve whitelisted file from config dir. | — |

---

## Widget proxy

| Method | Path | Description | Rate |
|---|---|---|---|
| GET/POST | `/api/services/proxy` | Generic proxy. Query: `group`, `service`, `endpoint`. | 10/s |

---

## Docker / Kubernetes / Proxmox

| Method | Path | Description |
|---|---|---|
| GET | `/api/docker/stats/{container}/{server}` | CPU / MEM / RX / TX. |
| GET | `/api/docker/status/{container}/{server}` | Running / exited + health. |
| GET | `/api/proxmox/stats/{vmid}/{server}` | Proxmox VM stats. |

---

## Monitoring

| Method | Path | Description | Rate |
|---|---|---|---|
| GET | `/api/ping` | ICMP/UDP ping. Query: `host` or `groupName`+`serviceName`. | 5/s |
| GET | `/api/siteMonitor` | HTTP HEAD with GET fallback. | 5/s |

---

## Info widgets

| Method | Path | Description |
|---|---|---|
| GET | `/api/widgets/resources` | System CPU / RAM / disk. |
| GET | `/api/widgets/openmeteo` | Weather from Open-Meteo. |
| GET | `/api/widgets/weather` | Alias of openmeteo. |

---

## Scripts

Only registered when `HOMEPAGE_SCRIPTS_ENABLED=true`. See
[`scripts.md`](./scripts.md) for the full feature guide.

| Method | Path | Description | Rate |
|---|---|---|---|
| GET  | `/api/scripts` | List. | 1/s |
| GET  | `/api/scripts/{name}/status` | Current state. | 1/s |
| POST | `/api/scripts/{name}` | Execute (HTML or JSON). | 1/s |
| POST | `/api/scripts/{name}/stream` | Execute with SSE streaming. | 1/s |

`X-Homepage-Confirm: yes` is required when the target script has
`requireConfirm: true` — otherwise the handler returns HTTP 428.

---

## Custom assets

| Method | Path | Description |
|---|---|---|
| GET | `/api/config/custom.css` | User CSS. |
| GET | `/api/config/custom.js`  | User JS. |
| GET | `/api/config/{image}` | Background images (`.png` / `.jpg` / `.jpeg` / `.webp` / `.gif` / `.svg` / `.avif` / `.ico` / `.bmp`). |

The `/api/config/{path}` handler is whitelist-based: anything outside
`custom.css`, `custom.js`, or the image extensions returns 404. Path
traversal (`..`, absolute paths, escaping `ConfigDir()`) is rejected
explicitly.
