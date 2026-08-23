# API Reference

> Every HTTP endpoint MyServer exposes, grouped by purpose, with method,
> path parameters, content-negotiation behaviour, and rate limit.

Handlers do **content negotiation** via the `HX-Request` header:

- `HX-Request: true` → partial HTML (Templ, for `innerHTML` swaps).
- otherwise → JSON (for API clients).

Every path below is written from the root of the host, which is where the
**root dashboard** — the one in `HOMEPAGE_CONFIG_DIR` — is served.

Two things move them:

- A **nested dashboard** (`config/dashboards/<name>/`) is served at `/<name>`,
  so its endpoints are `/<name>/api/services` and so on. It answers on a
  **subset** of this reference: the page, `/static`, `/auth/*`, and
  `/api/{services,bookmarks,widgets,hash,config/{path},ping,siteMonitor,healthcheck}`.
  Everything else — the widget proxy, the scripts endpoints, Docker, Proxmox,
  the info-widget data endpoints, `/api/reload`, `/api/validate` — is **not
  registered** for it and answers `404`. Each dashboard answers from its own
  config directory.
- `HOMEPAGE_BASE_PATH` moves all of them under one more prefix
  (`/team/api/services`, `/team/partners/api/services`), and the unprefixed form
  answers `404`.

See
[`configuration.md#serving-several-dashboards`](./configuration.md#serving-several-dashboards).

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
| GET  | `/api/validate` | Parse `services` / `bookmarks` / `widgets` / `settings`; report parse errors with filesystem paths scrubbed. Excludes `auth.yaml` (its errors name env vars). | — |
| GET  | `/api/config/{path}` | Serve whitelisted file from config dir. | — |

---

## Authentication

Registered always, but every route answers **404 while `config/auth.yaml`
lists nobody** — the allowlist is what turns the feature on. See
[`authentication.md`](./authentication.md).

| Method | Path | Description | Rate |
|---|---|---|---|
| GET  | `/auth/login` | Login page. Redirects to `/` when already signed in. | — |
| GET  | `/auth/denied` | 403 page for an address that is not on the allowlist. | — |
| GET  | `/auth/google/start` | Starts the OAuth flow; sets the state/nonce cookie. | 1/s |
| GET  | `/auth/google/callback` | Completes it; issues the session cookie. | 1/s |
| POST | `/auth/logout` | Clears the session cookie. | — |

**When an allowlist is configured, every other endpoint on this page requires
a session**, except `/api/healthcheck` and `/static/*`. Anonymous callers get
`302` to the login (navigation), `401` + `HX-Redirect` (HTMX) or `401` JSON
(API clients); an authenticated address that is not on the allowlist gets
`403`.

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
