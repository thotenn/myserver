# MyServer Dashboard — Cookbook

Recipes, complete examples, troubleshooting, and patterns for building a
MyServer dashboard.

> Start with `SKILL.md` for the quick reference. Come here for end-to-end
> examples and to debug specific symptoms.

---

## Contents

1. [A complete production dashboard](#a-complete-production-dashboard)
2. [Troubleshooting playbook](#troubleshooting-playbook)
3. [Recipes](#recipes)
4. [Security best practices](#security-best-practices)
5. [Template library](#template-library)

---

## A complete production dashboard

A realistic homelab setup demonstrating every feature: built-in widgets,
`customapi` + `file://`, scripts, Docker integration, info widgets, bookmarks,
tabbed layout, env-var substitution.

### `config/settings.yaml`

```yaml
title: Homelab Dashboard
theme: dark
color: slate
language: en
headerStyle: underlined
target: _blank
cardBlur: true

layout:
  Media:
    columns: 3
    tab: Media
  Infrastructure:
    columns: 2
    tab: Infra
  Monitoring:
    columns: 2
    tab: Monitoring
  Tools:
    columns: 2
    tab: Tools
  Administration:
    columns: 2
    tab: Admin

# `quicklaunch:` is legacy — the unified search/QuickLaunch bar is now
# rendered by the `search:` info widget (see widgets.yaml below).
# Leaving the block here only triggers the bar when the `search:`
# widget is absent; with both present it has no extra effect.

scripts:
  scriptDirs:
    - scripts
  maxTimeout: 3600
  defaultTimeout: 60
  maxConcurrent: 5
```

### `config/services.yaml`

```yaml
- Media:
    - Plex:
        href: https://plex.example.com
        description: Media server
        icon: si:plex
        container: plex
        server: local
        widget:
          type: plex
          url: http://localhost:32400
          key: "{{HOMEPAGE_VAR_PLEX_TOKEN}}"

    - Sonarr:
        href: https://sonarr.example.com
        description: TV shows
        icon: si:sonarr
        widget:
          type: sonarr
          url: http://localhost:8989
          key: "{{HOMEPAGE_VAR_SONARR_KEY}}"

    - Radarr:
        href: https://radarr.example.com
        description: Movies
        icon: si:radarr
        widget:
          type: radarr
          url: http://localhost:7878
          key: "{{HOMEPAGE_VAR_RADARR_KEY}}"

    - Jellyfin:
        href: https://jellyfin.example.com
        description: Media server
        icon: si:jellyfin
        widget:
          type: jellyfin
          url: http://localhost:8096

- Infrastructure:
    - Nginx:
        href: https://nginx.example.com
        description: Reverse proxy
        icon: si:nginx
        container: nginx
        server: local
        ping: nginx.example.com

    - Pi-hole:
        href: http://localhost/admin
        description: DNS blocker
        icon: si:pihole
        widget:
          type: pihole
          url: http://localhost/admin
          key: "{{HOMEPAGE_VAR_PIHOLE_TOKEN}}"

    - Proxmox:
        href: https://proxmox.example.com:8006
        description: Virtualization
        icon: si:proxmox
        widget:
          type: proxmox
          url: https://proxmox.example.com:8006
          token: "{{HOMEPAGE_VAR_PROXMOX_TOKEN}}"
          secret: "{{HOMEPAGE_VAR_PROXMOX_SECRET}}"

    - Traefik:
        href: https://traefik.example.com
        description: Edge router
        icon: si:traefik
        widget:
          type: traefik
          url: https://traefik.example.com

    - Portainer:
        href: https://portainer.example.com
        description: Container management
        icon: si:portainer
        container: portainer
        server: local
        widget:
          type: portainer
          url: https://portainer.example.com
          key: "{{HOMEPAGE_VAR_PORTAINER_KEY}}"

- Monitoring:
    - Grafana:
        href: https://grafana.example.com
        description: Metrics & dashboards
        icon: si:grafana
        widget: { type: grafana,    url: https://grafana.example.com }

    - Prometheus:
        href: https://prometheus.example.com
        description: Time series database
        icon: si:prometheus
        widget: { type: prometheus, url: https://prometheus.example.com }

    - Uptime Kuma:
        href: https://uptime.example.com
        description: Service uptime
        icon: si:uptimekuma
        widget: { type: uptimekuma, url: https://uptime.example.com }

- Tools:
    - Demos:
        href: https://demos.example.com
        description: Internal demo catalog
        icon: si:github
        widget:
          type: customapi
          url: file://data/demos.json
          display: dynamic-list
          mappings:
            items: demos
            name: title
            label: version
            target: url

- Administration:
    - Backup:
        description: Run nightly backup
        icon: mdi:backup-restore
        script: backup
        requireConfirm: true

    - Update:
        description: Update all containers
        icon: mdi:update
        script: update
        requireConfirm: true

    - Cleanup:
        description: Clean Docker system
        icon: mdi:broom
        script: cleanup
```

### `config/widgets.yaml`

```yaml
- search:    { provider: google, target: _blank }
- datetime:  { text_size: xl, format: { dateStyle: long, timeStyle: short, hour12: false } }
- greeting:  { text_size: lg }
- openmeteo: { label: London, latitude: 51.5074, longitude: -0.1278, timezone: Etc/UTC, units: metric, cache: 5 }
- resources: { label: CPU, cpu: true, cputemp: true, uptime: true }
- resources: { label: RAM, memory: true }
- resources: { label: Disk, disk: /, expanded: true }
```

### `config/bookmarks.yaml`

```yaml
- Developer:
    - GitHub:     { abbr: GH, href: https://github.com,     icon: si:github }
    - GitLab:     { abbr: GL, href: https://gitlab.com,     icon: si:gitlab }
    - Docker Hub: { abbr: DH, href: https://hub.docker.com, icon: si:docker }

- Cloud:
    - AWS:        { abbr: AWS, href: https://console.aws.amazon.com, icon: si:amazonwebservices }
    - Cloudflare: { abbr: CF,  href: https://dash.cloudflare.com,    icon: si:cloudflare }
    - Vercel:     { abbr: VE,  href: https://vercel.com,             icon: si:vercel }
```

### `config/docker.yaml`

```yaml
local:
  socket: /var/run/docker.sock
  swarm: false
```

### `config/scripts.yaml`

```yaml
scripts:
  backup:
    command: backup.sh
    description: "Run nightly backup"
    timeout: 300
    requireConfirm: true
    icon: mdi:backup-restore

  update:
    command: update.sh
    description: "Update all containers"
    timeout: 600
    requireConfirm: true
    icon: mdi:update
    env:
      DOCKER_HOST: unix:///var/run/docker.sock

  cleanup:
    command: cleanup.sh
    description: "Clean Docker system"
    timeout: 120
    icon: mdi:broom
    env:
      DOCKER_HOST: unix:///var/run/docker.sock
```

### `config/scripts/backup.sh`

```bash
#!/bin/bash
set -euo pipefail
echo "Starting backup at $(date)"
# … your backup logic
echo "Backup complete"
```

### `config/data/demos.json`

```json
{ "demos": [
    { "title": "Auth Service", "version": "v2.1", "url": "https://auth.local" },
    { "title": "API Gateway",   "version": "v1.5", "url": "https://gateway.local" }
] }
```

### Environment (set on the container, never in YAML)

```
HOMEPAGE_ALLOWED_HOSTS=dash.example.com,localhost:3000
HOMEPAGE_SCRIPTS_ENABLED=true
HOMEPAGE_ENABLE_HSTS=true
TZ=Etc/UTC
HOMEPAGE_VAR_PLEX_TOKEN=…
HOMEPAGE_VAR_SONARR_KEY=…
HOMEPAGE_VAR_RADARR_KEY=…
HOMEPAGE_VAR_PIHOLE_TOKEN=…
HOMEPAGE_VAR_PROXMOX_TOKEN=user@pve!id
HOMEPAGE_VAR_PROXMOX_SECRET=…
HOMEPAGE_VAR_PORTAINER_KEY=…
```

---

## Troubleshooting playbook

### Widget shows "Loading…" forever

1. DevTools → Network → find the `/api/services/proxy?...` request.
2. Inspect the response:

   | Status / body | Likely cause |
   |---|---|
   | `200` + HTML | Already loaded; the `display` mode may be wrong (lists need `display: dynamic-list`). |
   | `200` + JSON | The browser is not in HTMX mode. `dynamic-list` returns HTML only for HTMX. |
   | `502` / `504` | Upstream unreachable from inside the container (network, DNS, port). |
   | `429` | Rate limited. The `Retry-After` header tells you how long. |
   | `428` | Script `requireConfirm: true` but no `X-Homepage-Confirm: yes` header. |
3. Error bodies are scrubbed of credentials, so they're safe to share.

### Script returns 404 or "not found"

Checklist:

1. `HOMEPAGE_SCRIPTS_ENABLED=true` on the container?
2. File executable on the host? `chmod +x config/scripts/<name>.sh`.
3. `command:` is relative (no leading `/`), ends in `.sh`, and lives in a
   directory listed in `settings.yaml: scripts.scriptDirs`.
4. **Did you just add or change `settings.scripts.scriptDirs`?** That field
   is read only when the process starts. The fsnotify watcher re-registers
   `scripts.yaml` entries hot, but does not rebuild the script manager.
   Restart the container after editing `scriptDirs`, `maxTimeout`,
   `defaultTimeout`, or `maxConcurrent`.
5. World-writable? `chmod o-w config/scripts/<name>.sh` (rejected if `0o002`).
6. `docker compose logs -f myserver` will print a precise rejection reason on
   startup (path-traversal, denylisted env, bad extension, etc.).

### `/api/validate` says valid but the page is missing groups / widgets

Go's `yaml.v3` is lenient. It accepts ambiguous syntax that strict YAML
parsers reject — most commonly missing space after `:` in flow mappings:

```yaml
Infrastructure:{ columns: 2, tab: Infra }   # silent bad parse
Infrastructure: { columns: 2, tab: Infra }  # correct
```

The file looks valid to MyServer but downstream code reads the wrong shape
and silently drops keys (often everything after the bad line). Lint with a
strict parser:

```bash
python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" config/*.yaml
```

### Tabs in `settings.layout` don't render

`layout.<group>.tab` is currently parsed but unused: `index.templ` renders
every group as a sequential `<h2>` section. The `TabNavigation` Templ
component exists but is not invoked. This is a known open item — set
`tab:` for forward-compatible config, but expect a flat list today.

### Container can't write to `./config` (rootless Podman)

The image entrypoint does `chown myserver:myserver /app/config` on first
boot. With default rootless-Podman userns mapping, the container's
`myserver` (uid 1000) maps to a high host subuid (e.g. 100999), so the host
user (uid 1000) ends up unable to write the bind-mounted directory.

Fix in `docker-compose.dev.yml`:

```yaml
services:
  myserver:
    userns_mode: keep-id     # maps container uid 1000 ↔ host uid 1000
```

If you already hit it, repair ownership with:

```bash
podman unshare chown -R 0:0 ./config   # 0:0 inside userns = host uid 1000
```

### Container "Permission denied" on `./config` (SELinux / Fedora / RHEL)

The bind mount needs the `:Z` flag so Podman relabels it with a private
container context. Without it SELinux blocks writes even with permissive
Unix permissions:

```yaml
volumes:
  - ./config:/app/config:Z
```

This is a no-op on hosts without SELinux.

### Docker stats show "not found"

1. `docker ps --format '{{.Names}}'` on the host — container name must match
   `container:` in `services.yaml` exactly (no leading `/`).
2. `server:` in `services.yaml` must reference a key in `docker.yaml`.
3. Mount the socket: `/var/run/docker.sock:/var/run/docker.sock` in the
   compose (read-only is fine if scripts don't mutate containers).
4. Container user can access the socket. The entrypoint auto-adds `myserver`
   to the socket GID. Verify with `docker exec myserver id myserver`.
5. **First call** always returns 0% CPU — the calculation needs a 5s delta.

### Podman rootless: 0 containers seen by a script

Podman reads `~/.config/containers/storage.conf`. Inside the scrubbed env,
`HOME=/tmp`. Declare both in `scripts.yaml`:

```yaml
env:
  HOME: /home/youruser
  XDG_RUNTIME_DIR: /run/user/1000
  DOCKER_HOST: unix:///run/user/1000/podman/podman.sock
```

### Icons not loading

1. DevTools → Network → check the `<img>` request.
2. 404 from `jsdelivr.net` → the icon name is not in the
   [homarr-labs/dashboard-icons](https://github.com/homarr-labs/dashboard-icons)
   set. Switch prefix:
   - `si:gitlab` → Simple Icons (colored)
   - `mdi:database` → Material Design Icons
3. CDN reachability: `docker exec myserver wget -qO- https://cdn.jsdelivr.net`.
4. Last resort: absolute URL → `icon: https://example.com/logo.png`.

### Config changes not picked up

1. File saved on the **host**, in the bind-mounted directory (not inside the
   container)?
2. Logs show fsnotify events: `make logs | grep -i 'config file changed'`?
3. `vim` atomic-save can fire only a `RENAME` event — try `:set
   backupcopy=yes` or `touch config/<file>.yaml` after editing.
4. `fsnotify` watches `.yaml`, `.yml`, `.css`, `.js` in the **top level** of
   the config directory only.
5. `GET /api/validate` will report YAML parse errors.

### `siteMonitor` shows ERR

1. Reachable from inside the container?
   `docker exec myserver wget -qO- https://api.example.com/health`
2. The URL must match an actual service in `services.yaml` (open-proxy guard).
3. Endpoint must accept `HEAD` or `GET` (HEAD is tried first).
4. Self-signed certs fail TLS validation — terminate TLS at a reverse proxy.

### Ping shows "offline"

1. Reachable? `docker exec myserver ping -c 1 192.168.1.1`.
2. UDP-mode ping is used (no `CAP_NET_RAW`). Some hosts/firewalls drop ICMP —
   prefer `siteMonitor` for HTTP services.

### Rate limit (429)

Default limits: 60/min for most routes, 10/min for scripts, 1/min for `/api/hash`.
`Retry-After` is set. If too many widgets are polling, reduce the number of
active widgets or extend their intervals via `cache:` (where supported).

---

## Recipes

### 1. Tabbed layout with per-group columns

```yaml
# config/settings.yaml
layout:
  Infrastructure: { columns: 2, tab: Infra }
  Media:          { columns: 3, tab: Media }
  Monitoring:     { columns: 2, tab: Monitoring }
  Tools:          { columns: 2, tab: Admin }
```

The dashboard renders tabs as soon as any group has a `tab`. Groups without a
`tab` render below.

### 2. Env-var and file-secret substitution

```yaml
# services.yaml
widget:
  type: plex
  url: http://localhost:32400
  key:  "{{HOMEPAGE_VAR_PLEX_TOKEN}}"     # env value
  apiKey: "{{HOMEPAGE_FILE_PLEX_KEY}}"    # file contents

# scripts.yaml
scripts:
  deploy:
    command: deploy.sh
    env:
      DEPLOY_TOKEN: "{{HOMEPAGE_VAR_DEPLOY_TOKEN}}"
```

Set the env vars on the container (compose `environment:` or your platform's env UI). If
unresolved, the placeholder is kept literally (fail-visible).

### 3. Custom CSS for card styling

```css
/* config/custom.css */
.service-card    { border-radius: 16px; }
.status-dot      { box-shadow: 0 0 6px currentColor; }
.htmx-indicator  { display: none; }
.bookmark-item   { padding: 4px 12px; border-radius: 999px; }
@media (max-width: 640px) {
  .service-card  { padding: 12px; }
}
```

### 4. Custom JS for extra interactivity

```javascript
// config/custom.js
document.addEventListener('DOMContentLoaded', () => {
  document.addEventListener('keydown', (e) => {
    if (e.key === '/' && document.activeElement.tagName !== 'INPUT') {
      e.preventDefault();
      document.getElementById('search-input')?.focus();
    }
  });

  document.body.addEventListener('htmx:afterRequest', (e) => {
    if (e.detail.elt.hasAttribute('data-widget-type')) {
      const type = e.detail.elt.getAttribute('data-widget-type');
      console.debug(`widget ${type} loaded in ${e.detail.requestConfig.elapsed}ms`);
    }
  });
});
```

### 5. Docker discovery via labels

Skip `services.yaml` and let MyServer auto-discover labelled containers:

```bash
docker run -d \
  --name plex \
  --label homepage.name="Plex" \
  --label homepage.group="Media" \
  --label homepage.icon="si:plex" \
  --label homepage.href="https://plex.example.com" \
  --label homepage.description="Media server" \
  --label homepage.weight="10" \
  --label homepage.ping="plex.example.com" \
  plexinc/pms-docker
```

Discovery is merged with `services.yaml` — config takes priority, so any field
you set in YAML overrides the label.

### 6. Cron-refreshed JSON status card

```bash
#!/bin/bash
# config/scripts/generate-status.sh
set -euo pipefail

cat > /app/config/data/status.json <<EOF
{
  "services": [
    { "name": "API", "status": "$(curl -fsS https://api.example.com/health >/dev/null 2>&1 && echo running || echo down)" },
    { "name": "DB",  "status": "$(pg_isready -h localhost              >/dev/null 2>&1 && echo running || echo down)" }
  ]
}
EOF

echo "Status JSON updated"
```

```yaml
# scripts.yaml
scripts:
  generate-status:
    command: generate-status.sh
    description: "Refresh service status JSON"
    timeout: 30
    icon: mdi:refresh

# services.yaml
- Status:
    - Services:
        description: Service health overview
        icon: mdi:heart-pulse
        widget:
          type: customapi
          url: file://data/status.json
          display: dynamic-list
          mappings: { items: services, name: name, label: status }
```

### 7. Multiple weather widgets

```yaml
# widgets.yaml
- openmeteo: { label: London,  latitude: 51.5074, longitude: -0.1278, units: metric, cache: 5 }
- openmeteo: { label: New York,  latitude:  40.7128, longitude: -74.0060, units: metric, cache: 5 }
```

### 8. Proxmox token format

```yaml
widget:
  type: proxmox
  url: https://proxmox.example.com:8006
  token:  "root@pam!mytoken"          # {user}@{realm}!{tokenname}
  secret: "{{HOMEPAGE_VAR_PVE_SECRET}}"   # UUID from Proxmox UI
```

Create the token in **Proxmox UI → Datacenter → Permissions → API Tokens**.

### 9. Restart-a-container script

```yaml
# scripts.yaml
scripts:
  restart-traefik:
    command: restart-container.sh
    description: "Restart the reverse proxy"
    args: ["traefik"]
    timeout: 30
    requireConfirm: true
    icon: mdi:restart
    env:
      DOCKER_HOST: unix:///var/run/docker.sock
```

```bash
#!/bin/bash
# config/scripts/restart-container.sh
set -euo pipefail
NAME="${1:?container name required}"
CT=$(docker ps --filter "name=^${NAME}" --format '{{.Names}}' | head -1)
[ -z "$CT" ] && { echo "ERROR: no container matched prefix ${NAME}"; exit 1; }
echo "Restarting $CT…"
docker restart "$CT"
echo "Done."
```

### 10. Unified search bar (web search + live filter)

The top-right input rendered by the `search:` widget is one bar that does
both jobs:

- Type 2+ characters → dropdown shows up to 8 matching service cards and
  bookmarks (filtered by name).
- `↓` / `↑` → navigate suggestions (highlighted with a tint).
- `Enter` → if a suggestion is highlighted, follow it; otherwise the
  query goes to the configured engine (Google by default, or a pasted
  URL is opened directly).
- `Esc` → close dropdown and blur.
- A "Search the web for …" row is always pinned at the bottom of the
  dropdown so `Enter` never feels like a dead end.

Minimum config:

```yaml
# config/widgets.yaml
- search:
    provider: google           # google | duckduckgo | bing
    target: _blank
```

Legacy: `settings.yaml: quicklaunch:` still triggers the same bar even
without a `search:` widget, but its sub-fields
(`searchDescriptions`, `hideInternetSearch`, `hideVisitURL`) are no-ops
today. Prefer the `search:` widget for new configs.

---

## Security best practices

1. **Never commit secrets**. Pull every credential through `{{HOMEPAGE_VAR_*}}`
   or `{{HOMEPAGE_FILE_*}}`.
2. **Always set `HOMEPAGE_ALLOWED_HOSTS`** to the real domain plus
   `localhost:3000` (for healthcheck). Avoid `*` in production.
3. **`requireConfirm: true` on every destructive script** (backups, restarts,
   cleanups, deletes).
4. **Mount `docker.sock` read-only** unless scripts must mutate containers.
5. **Bind-mount `config/`** (not a Docker named volume) so fsnotify sees host
   edits.
6. **Keep `.sh` files under `config/scripts/`** — one bind mount, one place.
7. **Audit script permissions** periodically:
   `find config/scripts -type f -perm -002`.
8. **Don't override the env denylist** (`LD_PRELOAD`, `PATH`, `BASH_ENV`, …) —
   registration will reject the script.
9. **Enable HSTS** when serving over HTTPS: `HOMEPAGE_ENABLE_HSTS=true`.
10. **Treat the dashboard as unauthenticated** — put it behind Cloudflare
    Access / oauth2-proxy / Authelia.

---

## Template library

Copy-paste YAML snippets under `templates/` (one file per `config/` YAML):

| File | Content |
|---|---|
| `templates/services.yaml`  | 30 service-card patterns |
| `templates/widgets.yaml`   | 25 info-widget patterns |
| `templates/bookmarks.yaml` | 15 bookmark patterns |
| `templates/scripts.yaml`   | 15 script definitions |
| `templates/settings.yaml`  | 15 settings configurations |
| `templates/docker.yaml`    | 10 Docker / Podman server definitions |

Feature deep dives under `guides/`:

| File | Content |
|---|---|
| `guides/customapi.md`   | `customapi` widget — mappings, display modes, field paths |
| `guides/file-scheme.md` | `file://` data sources — patterns and recipes |

Shell-script templates under `scripts/`:

| File | Content |
|---|---|
| `scripts/templates.sh` | 15 shell-script templates |
