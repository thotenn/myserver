# Add Widget / Feature — MyServer Dashboard Skill

Expert assistant for adding service cards, bookmarks, scripts, info widgets,
data sources, themes, or any dashboard feature to a deployed **MyServer**
instance — without writing Go code.

> **Companion skill.** `sk-clients/` owns the dashboards themselves: several of
> them on one hostname, URL prefixes (`HOMEPAGE_BASE_PATH`), per-dashboard
> `auth.yaml`, and what a *client* dashboard may and may not contain. If the
> request is "add a dashboard for X" rather than "add a card", go there.
>
> **Companion files**
> - `COOKBOOK.md` — complete examples, troubleshooting, advanced recipes.
> - `templates/` — copy-paste YAML templates (one file per `config/` YAML).
> - `guides/customapi.md`, `guides/file-scheme.md`, `guides/allowlist.md` —
>   feature deep dives.
> - `scripts/templates.sh` — shell script templates.

---

## Core rule

**The user never edits Go code.** Every dashboard change happens through YAML
(or JSON / CSS / JS) files inside the bind-mounted `config/` directory. Save
the file → `fsnotify` detects the change → the dashboard hot-reloads.

If a request needs Go changes (new widget type, new handler), say so and stop.

---

## File layout (in the host `config/` directory)

```
config/
  settings.yaml      # title, theme, layout, scripts settings, quicklaunch
  services.yaml      # service groups and cards
  bookmarks.yaml     # bookmark groups
  widgets.yaml       # top-bar info widgets
  docker.yaml        # docker/podman server definitions
  proxmox.yaml       # proxmox token-auth servers
  scripts.yaml       # script definitions (opt-in feature)
  auth.yaml          # email allowlist (optional; absent = public dashboard)
  scripts/           # executable .sh files
  data/              # local JSON data sources (read via file://)
  custom.css         # injected into <head>
  custom.js          # injected before </body>
```

Inside the container the directory is always `/app/config`. On the host it is
wherever the bind mount points — check the `volumes:` entry of the compose file
rather than assuming a path.

---

## Decision tree

| User wants to… | Edit file(s) | Reference |
|---|---|---|
| Add a service card with icon + link | `services.yaml` | [Service cards](#service-cards) |
| Show data from an HTTP JSON API | `services.yaml` (with `widget:`) | [Built-in widgets](#built-in-widgets) · [`customapi`](#customapi-flexible) |
| Show data from a local JSON file | `services.yaml` + `data/*.json` | [`file://` sources](#file-data-sources) |
| Run a shell script from the dashboard | `services.yaml` + `scripts.yaml` + `scripts/*.sh` | [Script cards](#script-cards) |
| Show CPU/MEM/RX/TX of a container | `services.yaml` (set `container` + `server`) + `docker.yaml` | [Docker integration](#docker-integration) |
| Ping or HTTP-monitor badge | `services.yaml` (`ping:` / `siteMonitor:`) | [Monitoring badges](#monitoring-badges) |
| Top-bar widget (clock, search, weather, CPU bars) | `widgets.yaml` | [Info widgets](#info-widgets-top-bar) |
| Bookmark pill | `bookmarks.yaml` | [Bookmarks](#bookmarks) |
| Change theme, title, columns, layout, tabs | `settings.yaml` | [Settings](#settings) |
| Custom CSS / JS | `custom.css` / `custom.js` | `COOKBOOK.md` recipes 3 + 4 |
| Require a login / restrict who can see the dashboard | `auth.yaml` | [Email allowlist](#email-allowlist-login) · [`guides/allowlist.md`](guides/allowlist.md) |

---

## Service cards

```yaml
# config/services.yaml
- Group Name:
    - Service Name:
        href: https://service.example.com    # whole card is clickable
        description: What this service does
        icon: si:plex                         # see Icons below
        container: plex                       # for Docker stats
        server: local                         # references docker.yaml
        showStats: true
        ping: service.example.com             # ICMP/UDP badge
        siteMonitor: https://service.example.com/health   # HTTP badge
        weight: 10                            # sort within group (low = first)
        widget:                                # optional inline widget
          type: plex
          url: http://localhost:32400
          key: "{{HOMEPAGE_VAR_PLEX_TOKEN}}"
```

Field reference:

| Field | Type | Purpose |
|---|---|---|
| `href` | string | Click target. Whole card is a stretched link. |
| `description` | string | Subtitle. |
| `icon` | string | `mdi:`, `mdi-`, `si:`, `si-`, bare icon name, or absolute URL. |
| `container` | string | Docker container name (for stats + status). |
| `server` | string | Key in `docker.yaml`. |
| `showStats` | bool | Show CPU/MEM/RX/TX row. Default true when `container` is set. |
| `ping` | string | Hostname for ICMP (UDP-mode, no `CAP_NET_RAW`). |
| `siteMonitor` | string | URL for HTTP HEAD-with-GET-fallback. Must match the service's URL. |
| `widget` | object | Built-in widget config (see [Built-in widgets](#built-in-widgets)). |
| `script` | string | Script name → renders an "Execute" button instead of a link. |
| `requireConfirm` | bool | Demand confirmation before running the script. |
| `weight` | int | Sort order inside the group (lower first). |
| `tabindex` | int | Tab to place this card in (if tabs are enabled). |

---

## Built-in widgets

46 widget types are registered in the binary. Configure them inline under
`widget:` on a service card. The `GenericProxyHandler` queries the registry
for `APITemplate()` and `Mappings()` automatically.

Common patterns:

```yaml
# Media indexers (sonarr, radarr, lidarr, prowlarr, bazarr, overseerr)
widget: { type: sonarr,   url: http://localhost:8989, key: "{{HOMEPAGE_VAR_SONARR_KEY}}" }

# Media servers (plex, jellyfin, emby, tautulli)
widget: { type: plex,     url: http://localhost:32400, key: "{{HOMEPAGE_VAR_PLEX_TOKEN}}" }

# Download clients (qbittorrent, transmission, deluge, sabnzbd)
widget: { type: qbittorrent, url: http://localhost:8080, username: admin, password: "{{HOMEPAGE_VAR_QBIT_PASS}}" }

# Networking (pihole, adguard, traefik, caddy, npm, cloudflared, tailscale)
widget: { type: pihole,   url: http://localhost/admin, key: "{{HOMEPAGE_VAR_PIHOLE_TOKEN}}" }

# Monitoring (portainer, uptimekuma, netdata, prometheus, grafana)
widget: { type: portainer, url: https://portainer.example.com, key: "{{HOMEPAGE_VAR_PORTAINER_KEY}}" }

# Infrastructure
widget: { type: proxmox,  url: https://pve.example.com:8006, token: "user@pve!id", secret: "{{HOMEPAGE_VAR_PVE_SECRET}}" }

# Productivity (nextcloud, trilium, paperlessngx)
widget: { type: nextcloud, url: https://cloud.example.com }
```

See [Appendix — all widget types](#appendix--all-built-in-widget-types) for
the full list.

### `customapi` (flexible)

The Swiss Army knife for arbitrary JSON APIs. Display modes: `text`,
`dynamic-list`, `graph`, `list`, `tile`.

```yaml
# Single value
widget:
  type: customapi
  url: http://localhost:8080/api/stats
  display: text
  mappings:
    field: active_users
    format: number     # number | bytes | duration | percent | date

# List with links
widget:
  type: customapi
  url: file://data/demos.json
  display: dynamic-list
  mappings:
    items: demos       # path to array
    name: title
    label: version
    target: "{url}"    # template; {field} pulls a value from the item
```

Field paths are dot-separated and support array indices:
`mappings.field: "results.0.name"`. See `guides/customapi.md` for the deep
dive.

---

## `file://` data sources

Widgets can read local JSON straight from `config/` — no HTTP round-trip, no
SSRF check (it isn't a network call), hot-reloaded on save.

```yaml
widget:
  type: customapi
  url: file://data/demos.json        # → $HOMEPAGE_CONFIG_DIR/data/demos.json
  display: dynamic-list
  mappings:
    items: demos
    name: title
    label: version
    target: url
```

`config/data/demos.json`:

```json
{ "demos": [
    { "title": "Auth Service", "version": "v2.1", "url": "https://auth.local" },
    { "title": "API Gateway",   "version": "v1.5", "url": "https://gateway.local" }
] }
```

Absolute paths work: `file:///absolute/path.json`. See
`guides/file-scheme.md` for patterns (status pages, dynamic generation,
cron-refreshed JSON).

---

## Script cards

Opt-in feature. Requires `HOMEPAGE_SCRIPTS_ENABLED=true` on the container.

```bash
# 1) The script — config/scripts/backup.sh
#!/bin/bash
set -euo pipefail
echo "Starting backup at $(date)"
# … your logic
echo "Backup complete"
```

```yaml
# 2) Register it — config/scripts.yaml
scripts:
  backup:
    command: backup.sh        # relative to scriptDirs (default: scripts)
    description: "Run nightly backup"
    timeout: 300              # seconds, capped by maxTimeout
    requireConfirm: true      # demands header X-Homepage-Confirm: yes
    icon: mdi:backup-restore
    env:
      DOCKER_HOST: unix:///var/run/docker.sock   # explicit opt-in
```

```yaml
# 3) Expose it on the dashboard — config/services.yaml
- Administration:
    - Backup:
        description: Run system backup
        icon: mdi:backup-restore
        script: backup
        requireConfirm: true
```

Make the file executable on the host: `chmod +x config/scripts/backup.sh`.

### Script security (server-enforced — always)

- Only `.sh` files inside the configured `scriptDirs`.
- No absolute paths, no `..`. Symlinks resolved with `EvalSymlinks` + prefix
  check.
- Regular files only (no devices, sockets, FIFOs). World-writable files
  (`mode & 0o002 != 0`) are rejected.
- Scripts do **not** inherit the parent env. Defaults: `PATH`, `HOME=/tmp`,
  `USER=myserver`, `SHELL=/bin/bash`, `TZ`. Anything else must be declared in
  `env:`.
- Env denylist: `LD_PRELOAD`, `LD_LIBRARY_PATH`, `LD_AUDIT`, `BASH_ENV`,
  `ENV`, `PROMPT_COMMAND`, `IFS`, `PATH`, `BASH_FUNC_*`.
- `requireConfirm: true` → the frontend MUST send `X-Homepage-Confirm: yes`;
  otherwise HTTP 428.
- Cancellation / timeout → SIGTERM to the process group, 5s grace, SIGKILL.
- Output capped at 1 MiB; concurrency capped by the global semaphore
  (`settings.scripts.maxConcurrent`, default 5).

### Podman rootless

```yaml
env:
  HOME: /home/youruser                                # podman reads ~/.config/containers
  XDG_RUNTIME_DIR: /run/user/1000
  DOCKER_HOST: unix:///run/user/1000/podman/podman.sock
```

---

## Docker integration

```yaml
# config/docker.yaml
local:
  socket: /var/run/docker.sock
  swarm: false

remote:
  host: 10.0.0.5
  port: 2375

secure:
  host: 10.0.0.6
  port: 2376
  tls:
    ca:   /path/to/ca.pem
    cert: /path/to/cert.pem
    key:  /path/to/key.pem
```

Reference the server name from `services.yaml`:

```yaml
- Infrastructure:
    - Nginx:
        icon: si:nginx
        container: nginx
        server: local
        showStats: true   # CPU/MEM/RX/TX badge
```

### Discovery via labels

Containers labelled `homepage.*` are auto-discovered and merged with
`services.yaml` (config wins on conflict):

```bash
docker run -d \
  --label homepage.name="Plex" \
  --label homepage.group="Media" \
  --label homepage.icon="si:plex" \
  --label homepage.href="https://plex.example.com" \
  --label homepage.description="Media server" \
  --label homepage.weight="10" \
  plexinc/pms-docker
```

Supported labels: `name`, `group`, `icon`, `href`, `description`, `ping`,
`siteMonitor`, `weight`, `widget` (JSON-encoded).

---

## Monitoring badges

```yaml
- Monitoring:
    - Router:
        href: http://192.168.1.1
        ping: 192.168.1.1                          # ICMP/UDP

    - API:
        href: https://api.example.com
        siteMonitor: https://api.example.com/health  # HTTP HEAD → GET fallback
```

Notes:

- `siteMonitor` URL must match a configured service — arbitrary URLs are
  rejected (open-proxy guard).
- `ping` uses unprivileged UDP mode (no `CAP_NET_RAW`). Some hosts block ICMP;
  prefer `siteMonitor` for HTTP services in that case.

---

## Info widgets (top bar)

```yaml
# config/widgets.yaml
- search:
    provider: google         # google | duckduckgo | bing
    target: _blank
# The `search:` widget renders the unified search bar (top-right of the
# header). It is both a web-search form (Enter → engine) AND a live
# QuickLaunch: typing 2+ chars shows a dropdown of matching services and
# bookmarks (↑/↓ to navigate, Enter to follow the highlighted one, Esc
# to close). A "Search the web for …" row always appears at the bottom
# so Enter never feels like a dead end.

- datetime:
    text_size: xl
    format:
      dateStyle: long
      timeStyle: short
      hour12: false

- greeting:
    text_size: lg

- openmeteo:                  # weather, no API key needed
    label: London
    latitude: 51.5074
    longitude: -0.1278
    timezone: Etc/UTC
    units: metric
    cache: 5                  # minutes between fetches

- resources:
    label: CPU
    cpu: true
    cputemp: true
    uptime: true

- resources:
    label: RAM
    memory: true

- resources:
    label: Disk
    disk: /
    expanded: true
```

Multiple `resources` and `openmeteo` blocks are allowed (per-mount, per-city).

---

## Bookmarks

```yaml
# config/bookmarks.yaml
- Developer:
    - GitHub:    { abbr: GH, href: https://github.com, icon: si:github }
    - GitLab:    { abbr: GL, href: https://gitlab.com }   # icon auto-detected
- Cloud:
    - AWS:        { abbr: AWS, href: https://console.aws.amazon.com, icon: si:amazonwebservices }
    - Cloudflare: { abbr: CF,  href: https://dash.cloudflare.com,    icon: si:cloudflare }
```

If `icon` is omitted, the bookmark name is matched against a built-in list of
common services and auto-assigned a Simple-Icons icon.

---

## Settings

```yaml
# config/settings.yaml
title: Homelab Dashboard
theme: dark                # dark | light
color: slate               # 23 palettes (see Colors below)
language: en               # en | es
headerStyle: underlined    # underlined | boxed | clean | cleaned | magnified | caterwaul
target: _blank             # _blank | _self | _top
hideVersion: true
backgroundImage: wallpaper.jpg                # optional · see below
cardBlur: true

layout:
  Media:
    style: row
    columns: 3
  Infrastructure:
    style: row
    columns: 2
    tab: Infra              # parsed but currently decorative — see Caveats
  Tools:
    columns: 2
    tab: Tools

# `quicklaunch:` is legacy. The search/QuickLaunch UI is now unified
# inside the `search:` info widget (see widgets.yaml). If `search:` is
# absent but `quicklaunch:` is set, the unified bar still renders — so
# old configs keep working. The individual fields below are no-ops
# today; remove the block once you've added `search:` to widgets.yaml.
quicklaunch:
  searchDescriptions: true
  hideInternetSearch: false
  hideVisitURL: false

scripts:                    # only consumed if HOMEPAGE_SCRIPTS_ENABLED=true
  scriptDirs:
    - scripts
  maxTimeout: 3600
  defaultTimeout: 60
  maxConcurrent: 5
```

**Colors**: `white, slate, gray, zinc, neutral, stone, red, orange, amber,
yellow, lime, green, emerald, teal, cyan, sky, blue, indigo, violet, purple,
fuchsia, pink, rose`.

### Background image

`settings.yaml: backgroundImage` accepts two forms:

| Form | Example | Resolves to |
|---|---|---|
| Local file (under `config/`) | `wallpaper.jpg` | `/api/config/wallpaper.jpg?v=<hash>` |
| Local in a sub-directory | `wallpapers/mountains.webp` | `/api/config/wallpapers/mountains.webp?v=<hash>` |
| HTTP / HTTPS URL | `https://images.example.com/bg.png` | passthrough |
| Data URI | `data:image/svg+xml;base64,…` | passthrough |

Allowed extensions for local files: `.png`, `.jpg`, `.jpeg`, `.webp`,
`.gif`, `.svg`, `.avif`, `.ico`, `.bmp`. Anything else returns 404.

The image is applied as a fixed, cover-sized, centred background on
`<body>` (no repeat). To keep cards readable, pair it with
`cardBlur: true` (frosted-glass card background) or a darker `theme`.

Drop the file into `config/` on the host — `fsnotify` triggers a config
hash refresh and the browser reloads. No restart needed.

---

## Icons

| Prefix | Example | Resolves to |
|---|---|---|
| `si:` / `si-` | `si:github`        | `https://cdn.simpleicons.org/github` (colored) |
| `mdi:` / `mdi-` | `mdi:backup-restore` | `https://cdn.jsdelivr.net/npm/@mdi/svg@latest/svg/backup-restore.svg` |
| (none) | `plex.png`         | `https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/png/plex.png` |
| URL | `https://example.com/logo.png` | passthrough |

Fallback path: when `dashboard-icons` does not have it, pick an `si:` or
`mdi:` equivalent.

---

## Env var substitution (in any YAML)

```yaml
key: "{{HOMEPAGE_VAR_PLEX_TOKEN}}"     # value of env HOMEPAGE_VAR_PLEX_TOKEN
key: "{{HOMEPAGE_FILE_PLEX_KEY}}"      # contents of file at HOMEPAGE_FILE_PLEX_KEY
```

If the variable / file does not exist, the placeholder is preserved literally
(fail-visible), not replaced with an empty string.

---

## Email allowlist (login)

Optional. Turns the whole dashboard private, gated by Google sign-in and an
explicit list of addresses. Full walkthrough — creating the Google OAuth
client, wiring credentials, day-to-day add/remove, troubleshooting — in
**[`guides/allowlist.md`](guides/allowlist.md)**. Template with seven worked
examples: `templates/auth.yaml`.

**The allowlist is the switch.** There is no `enabled` flag:

| `config/auth.yaml` | Result |
|---|---|
| absent | Public dashboard (default, unchanged behaviour) |
| present, `emails: []` | Public dashboard |
| present, one or more addresses | Login required for everything |

```yaml
# config/auth.yaml — minimum working config
allowlist:
  emails:
    - owner@example.com

google:
  clientId:     "{{HOMEPAGE_VAR_GOOGLE_CLIENT_ID}}"
  clientSecret: "{{HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET}}"
  redirectURL:  "https://dashboard.example.com/auth/google/callback"
```

Hot-reload: yes. Adding or removing an address takes effect on the **next
request** — a removed person is evicted immediately, not when their cookie
expires.

### The five things to get right

1. **`redirectURL` must match Google character for character.** The path is
   `/auth/google/callback` — slashes, not dots, no trailing slash, same scheme
   and host the user browses. A mismatch fails with `redirect_uri_mismatch`
   before the consent screen appears. This is the most common setup error.
2. **Environment variables first, file second.** An `auth.yaml` whose
   `{{HOMEPAGE_VAR_*}}` placeholders cannot be resolved makes the process
   refuse to start. Verify with
   `docker exec <container> printenv | grep HOMEPAGE_VAR_GOOGLE`. On Compose,
   a host variable only reaches the container if the service declares it under
   `environment:`.
3. **To go back to public, empty the list — never delete the file.** A file
   that vanishes while sign-in is active gives 503 on everything, on purpose:
   "deleted deliberately" and "the mount broke" are indistinguishable, and
   guessing wrong would publish the dashboard.
4. **A broken `auth.yaml` never opens the dashboard.** A YAML typo keeps the
   last working allowlist and logs the error. That is deliberate — the whole
   point is that no config failure can result in a public dashboard.
5. **Have the user test a non-allowlisted account too.** A login that lets
   everyone in looks exactly like one that works. The second account must get
   403.

### Already behind Cloudflare Access / Authelia / oauth2-proxy?

Skip OAuth entirely — read the identity the proxy already asserts, through the
same allowlist:

```yaml
provider: trustedHeader
trustedHeader:
  header: "Cf-Access-Authenticated-User-Email"
allowlist:
  emails:
    - owner@example.com
```

Only honoured when the immediate peer is in `TRUSTED_PROXIES`. Never suggest
it without a proxy actually in front.

### What gets protected

Everything except `/static/*`, `/auth/*`, `/api/healthcheck`, and anything the
user lists in `publicPaths`. That includes `/api/services`, `/api/widgets`,
`/api/services/proxy` and `/api/scripts/*` — so anonymous scripts hitting the
API stop working, while the container healthcheck keeps working.

---

## Safety rules — never violate

1. **Never commit secrets to YAML files.** Always pull through
   `{{HOMEPAGE_VAR_*}}` or `{{HOMEPAGE_FILE_*}}`.
2. **Set `HOMEPAGE_ALLOWED_HOSTS`** to the real domain (and `localhost:PORT`
   for dev). Avoid `*` in production.
3. **Use `requireConfirm: true`** on every destructive script (backups,
   restarts, cleanups, deletes).
4. **Mount `docker.sock` read-only** unless scripts actually need to mutate
   containers.
5. **Keep scripts under `config/scripts/`**, not `/app/scripts/`, so a single
   bind mount carries config + executables.
6. **Don't try to override** the env denylist (`LD_PRELOAD`, `PATH`, etc.) —
   the manager rejects them at registration.
7. **The dashboard is public unless something guards it.** Either put an
   external auth layer in front (Cloudflare Access, Authelia, oauth2-proxy…)
   or enable the built-in [email allowlist](#email-allowlist-login). Never
   leave a production deployment with neither.
8. **Never write the OAuth client secret into `auth.yaml`** — use
   `{{HOMEPAGE_VAR_GOOGLE_CLIENT_SECRET}}`.
9. **Never tell a user to delete `auth.yaml` to make the dashboard public
   again.** That answers 503 on everything. Empty the list instead:
   `emails: []`.

---

## Verify after every change

1. Save the YAML in the host `config/` directory.
2. Watch the container logs for fsnotify events: `make logs` or
   `docker compose logs -f myserver | grep -i 'config file changed'`.
3. Reload the dashboard in the browser (it auto-reloads within ~10s via
   `/api/hash` polling).
4. Use `GET /api/validate` to confirm all YAMLs parse cleanly.
5. **`/api/validate` is not strict enough** — also run any YAML linter on the
   files you wrote (Go's `yaml.v3` may silently accept `key:{value}` while a
   strict parser rejects it). One-liner:
   `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" config/*.yaml`
6. If a widget shows "Loading…" forever, check `COOKBOOK.md → Troubleshooting`.

---

## Caveats — surprising behaviour worth knowing

- **YAML space after `:`** — Go's `yaml.v3` is lenient and accepts
  `Infrastructure:{ columns: 2 }` (no space after the colon). The file may
  pass `/api/validate` but silently parse the wrong shape and break
  downstream loaders. Strict YAML parsers reject it. **Always put a space
  after `:`**, including inside flow mappings (`{ ... }`).

- **`settings.scripts.scriptDirs` is read once at startup.** The fsnotify
  watcher hot-reloads `scripts.yaml` entries (via
  `scripts.Manager.ReplaceAll`) but does not recreate the manager — so
  changes to `scriptDirs`, `maxTimeout`, `defaultTimeout`, or `maxConcurrent`
  require a container restart. Script files themselves (added / removed /
  edited) and `scripts.yaml` entries are still hot-reloaded.

- **`layout.<group>.tab` is currently decorative.** The field is parsed and
  stored in `Settings`, and a `TabNavigation` Templ component exists, but
  `index.templ` does not invoke it: groups always render as sequential
  `<h2>` sections. Use `tab:` for forward-compatible config, but don't rely
  on a tabbed UI yet.

- **Rootless Podman bind mounts** need `userns_mode: keep-id` in the compose
  service, or the entrypoint's `chown myserver:myserver /app/config` flips
  the host directory to a high subuid the user can't write to. The dev
  compose already sets this.

- **SELinux (Fedora / RHEL / CentOS)** — the bind mount must be declared
  with `:Z` (`./config:/app/config:Z`), otherwise the container can't write
  even though Unix permissions allow it. No-op on hosts without SELinux.

---

## Appendix — all built-in widget types

| Category | Widgets |
|---|---|
| Media management | `sonarr` `radarr` `lidarr` `prowlarr` `bazarr` `overseerr` |
| Media servers | `plex` `jellyfin` `emby` `tautulli` |
| Download clients | `qbittorrent` `transmission` `deluge` `sabnzbd` |
| Networking | `pihole` `adguard` `traefik` `caddy` `npm` `cloudflared` `tailscale` |
| Monitoring | `portainer` `uptimekuma` `netdata` `prometheus` `grafana` |
| Productivity | `nextcloud` `trilium` `paperlessngx` |
| Infrastructure | `proxmox` `argocd` |
| System | `docker` `glances` `resources` `speedtest` `photoprism` `vikunja` `longhorn` |
| Flexible | `customapi` |
| Info (top bar) | `datetime` `greeting` `search` `weather` `openmeteo` `stocks` `kubernetes` `longhorn` |
| Aliases | `jellyseerr` → `overseerr`, `seerr` → `overseerr`, `openweathermap` → `weather` |

> `hoarder` is registered as an alias of `karakeep`, but no `karakeep` widget
> exists in the registry, so the alias resolves to nothing and the widget is
> reported as unknown. Don't suggest either name until that is fixed.

---

## Where to look next

- **Detailed recipes & troubleshooting**: `COOKBOOK.md`
- **Per-file YAML templates** (lots of examples): `templates/`
- **`customapi` deep dive**: `guides/customapi.md`
- **`file://` deep dive**: `guides/file-scheme.md`
- **Login / email allowlist, end to end**: `guides/allowlist.md`
- **Shell script templates**: `scripts/templates.sh`
- **Project docs**: `../../README.md` (user-facing), `../../CLAUDE.md` (agent-facing)
