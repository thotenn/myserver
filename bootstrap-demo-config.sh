#!/usr/bin/env bash
#
# bootstrap-demo-config.sh
#
# Regenerate `config/` with a comprehensive demo dashboard that exercises
# every MyServer feature documented in `.agents/skills/add-widget/`:
#
#   - Top-bar info widgets (datetime, greeting, search, openmeteo×2, resources)
#   - Service cards with built-in widgets across every registry category
#   - `customapi` widget in all five display modes (text / list / dynamic-list
#     / graph / tile) backed by `file://` JSON sources
#   - Monitoring badges (ping + siteMonitor)
#   - Script feature: 5 scripts + matching service cards (incl. requireConfirm)
#   - Bookmarks with grouped pills and icon auto-detection
#   - Tabbed layout with per-group columns
#   - Docker server definition
#   - Env-var substitution placeholders
#   - custom.css + custom.js
#
# The script is idempotent: it wipes `config/`'s contents (keeping the
# directory itself, so the SELinux label survives) and rewrites everything.
#
# Run from the repo root:
#
#   ./bootstrap-demo-config.sh
#
# Optional env: CONFIG_DIR (default: ./config)
#
set -euo pipefail

# `readlink -f` is GNU-only — BSD readlink (macOS) has no -f and the script dies
# on its first line. dirname + `pwd -P` resolves the same thing on any platform.
cd "$(cd "$(dirname "$0")" && pwd -P)"
if [ ! -f Makefile ] || [ ! -d internal/config/skeleton ]; then
  echo "ERROR: run this from the myserver repo root" >&2
  exit 1
fi

CONFIG_DIR="${CONFIG_DIR:-config}"
SCRIPTS_DIR="${CONFIG_DIR}/scripts"
DATA_DIR="${CONFIG_DIR}/data"

mkdir -p "${CONFIG_DIR}" "${SCRIPTS_DIR}" "${DATA_DIR}"

# Wipe contents only — keep the directory itself so its SELinux label
# (set by the bind mount with `:Z`) survives. Skip nothing-hidden files.
find "${CONFIG_DIR}"  -mindepth 1 -maxdepth 1 -type f \( -name '*.yaml' -o -name '*.yml' -o -name '*.css' -o -name '*.js' \) -delete 2>/dev/null || true
find "${SCRIPTS_DIR}" -mindepth 1 -maxdepth 1 -type f -delete 2>/dev/null || true
find "${DATA_DIR}"    -mindepth 1 -maxdepth 1 -type f -delete 2>/dev/null || true

log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
done_() { printf '    \033[1;32m✓\033[0m %s\n' "$*"; }

# ---------------------------------------------------------------------------
log "settings.yaml — title, theme, tabbed layout, scripts enabled"
cat > "${CONFIG_DIR}/settings.yaml" <<'YAML'
title: MyServer Demo
theme: dark
color: slate
language: en
headerStyle: clean
target: _blank
hideVersion: true
cardBlur: true

# `backgroundImage` accepts either a full URL or a path relative to the
# config directory. Local files are served via /api/config/<path>; only
# image extensions (png/jpg/jpeg/webp/gif/svg/avif/ico/bmp) are allowed.
# Examples:
#   backgroundImage: wallpaper.jpg                    # → config/wallpaper.jpg
#   backgroundImage: wallpapers/dark-mountains.webp   # → config/wallpapers/dark-mountains.webp
#   backgroundImage: https://example.com/photo.png    # remote URL (requires HTTPS)
# Uncomment to enable:
# backgroundImage: wallpaper.jpg

# Tabbed layout (any group with `tab` triggers tabs; groups without `tab`
# render below the tabs as plain sections).
layout:
  Media:          { columns: 3, tab: Media }
  Downloads:      { columns: 2, tab: Downloads }
  Networking:     { columns: 3, tab: Network }
  Monitoring:     { columns: 2, tab: Monitoring }
  Productivity:   { columns: 3, tab: Productivity }
  Infrastructure: { columns: 2, tab: Infra }
  Data:           { columns: 2, tab: Data }
  Administration: { columns: 2, tab: Admin }
  Health:         { columns: 2 }   # no tab → renders below the tabs

# `quicklaunch:` is legacy — the unified search bar (web search + live
# filter) is rendered by the `search:` info widget in widgets.yaml.
# Omitted here intentionally.

scripts:
  # Absolute path because scriptDirs entries are not resolved relative to
  # HOMEPAGE_CONFIG_DIR. Inside the container this points to the bind mount.
  scriptDirs:
    - /app/config/scripts
  defaultTimeout: 60
  maxTimeout: 3600
  maxConcurrent: 5
YAML
done_ "settings.yaml"

# ---------------------------------------------------------------------------
log "services.yaml — every widget category + customapi display modes + scripts"
cat > "${CONFIG_DIR}/services.yaml" <<'YAML'
- Media:
    - Plex:
        href: https://plex.example.com
        description: Media server
        icon: si:plex
        widget:
          type: plex
          url: http://plex.local:32400
          key: "{{HOMEPAGE_VAR_PLEX_TOKEN}}"

    - Jellyfin:
        href: https://jellyfin.example.com
        description: Media server
        icon: si:jellyfin
        widget:
          type: jellyfin
          url: http://jellyfin.local:8096

    - Emby:
        href: https://emby.example.com
        description: Media server
        icon: si:emby
        widget:
          type: emby
          url: http://emby.local:8096

    - Tautulli:
        href: https://tautulli.example.com
        description: Plex analytics
        icon: si:tautulli
        widget:
          type: tautulli
          url: http://tautulli.local:8181

    - Sonarr:
        href: https://sonarr.example.com
        description: TV shows
        icon: si:sonarr
        widget:
          type: sonarr
          url: http://sonarr.local:8989
          key: "{{HOMEPAGE_VAR_SONARR_KEY}}"

    - Radarr:
        href: https://radarr.example.com
        description: Movies
        icon: si:radarr
        widget:
          type: radarr
          url: http://radarr.local:7878
          key: "{{HOMEPAGE_VAR_RADARR_KEY}}"

    - Lidarr:
        href: https://lidarr.example.com
        description: Music
        icon: si:lidarr
        widget:
          type: lidarr
          url: http://lidarr.local:8686
          key: "{{HOMEPAGE_VAR_LIDARR_KEY}}"

    - Prowlarr:
        href: https://prowlarr.example.com
        description: Indexer manager
        icon: si:prowlarr
        widget:
          type: prowlarr
          url: http://prowlarr.local:9696
          key: "{{HOMEPAGE_VAR_PROWLARR_KEY}}"

    - Bazarr:
        href: https://bazarr.example.com
        description: Subtitles
        icon: si:bazarr
        widget:
          type: bazarr
          url: http://bazarr.local:6767
          key: "{{HOMEPAGE_VAR_BAZARR_KEY}}"

    - Overseerr:
        href: https://overseerr.example.com
        description: Media requests
        icon: si:overseerr
        widget:
          type: overseerr
          url: http://overseerr.local:5055
          key: "{{HOMEPAGE_VAR_OVERSEERR_KEY}}"

- Downloads:
    - qBittorrent:
        href: https://qbittorrent.example.com
        description: Torrent client
        icon: si:qbittorrent
        widget:
          type: qbittorrent
          url: http://qbittorrent.local:8080
          username: admin
          password: "{{HOMEPAGE_VAR_QBIT_PASS}}"

    - Transmission:
        href: https://transmission.example.com
        description: Torrent client
        icon: si:transmission
        widget:
          type: transmission
          url: http://transmission.local:9091

    - Deluge:
        href: https://deluge.example.com
        description: Torrent client
        icon: si:deluge
        widget:
          type: deluge
          url: http://deluge.local:8112
          password: "{{HOMEPAGE_VAR_DELUGE_PASS}}"

    - SABnzbd:
        href: https://sabnzbd.example.com
        description: Usenet client
        icon: si:sabnzbd
        widget:
          type: sabnzbd
          url: http://sabnzbd.local:8080
          key: "{{HOMEPAGE_VAR_SABNZBD_KEY}}"

- Networking:
    - Pi-hole:
        href: http://pihole.local/admin
        description: DNS sinkhole
        icon: si:pihole
        widget:
          type: pihole
          url: http://pihole.local/admin
          key: "{{HOMEPAGE_VAR_PIHOLE_TOKEN}}"

    - AdGuard:
        href: https://adguard.example.com
        description: DNS filter
        icon: si:adguard
        widget:
          type: adguard
          url: https://adguard.example.com
          username: admin
          password: "{{HOMEPAGE_VAR_ADGUARD_PASS}}"

    - Traefik:
        href: https://traefik.example.com
        description: Edge router
        icon: si:traefik
        widget:
          type: traefik
          url: https://traefik.example.com

    - Caddy:
        href: https://caddy.example.com
        description: Reverse proxy
        icon: si:caddy
        widget:
          type: caddy
          url: http://caddy.local:2019

    - Nginx Proxy Manager:
        href: https://npm.example.com
        description: Proxy host manager
        icon: si:nginx
        widget:
          type: npm
          url: http://npm.local:81
          username: "{{HOMEPAGE_VAR_NPM_USER}}"
          password: "{{HOMEPAGE_VAR_NPM_PASS}}"

    - Cloudflared:
        href: https://one.dash.cloudflare.com
        description: Tunnel
        icon: si:cloudflare
        widget:
          type: cloudflared
          url: https://api.cloudflare.com
          key:   "{{HOMEPAGE_VAR_CF_ACCOUNT_ID}}"
          token: "{{HOMEPAGE_VAR_CF_API_TOKEN}}"

    - Tailscale:
        href: https://login.tailscale.com/admin
        description: VPN mesh
        icon: si:tailscale
        widget:
          type: tailscale
          url: https://api.tailscale.com
          key: "{{HOMEPAGE_VAR_TAILSCALE_KEY}}"

- Monitoring:
    - Portainer:
        href: https://portainer.example.com
        description: Container management
        icon: si:portainer
        widget:
          type: portainer
          url: https://portainer.example.com
          key: "{{HOMEPAGE_VAR_PORTAINER_KEY}}"

    - Uptime Kuma:
        href: https://uptime.example.com
        description: Status pages
        icon: si:uptimekuma
        widget:
          type: uptimekuma
          url: https://uptime.example.com

    - Netdata:
        href: https://netdata.example.com
        description: Real-time metrics
        icon: si:netdata
        widget:
          type: netdata
          url: https://netdata.example.com

    - Prometheus:
        href: https://prometheus.example.com
        description: Time series DB
        icon: si:prometheus
        widget:
          type: prometheus
          url: https://prometheus.example.com

    - Grafana:
        href: https://grafana.example.com
        description: Dashboards
        icon: si:grafana
        widget:
          type: grafana
          url: https://grafana.example.com

- Productivity:
    - Nextcloud:
        href: https://cloud.example.com
        description: File sync
        icon: si:nextcloud
        widget:
          type: nextcloud
          url: https://cloud.example.com

    - Trilium:
        href: https://trilium.example.com
        description: Note taking
        icon: mdi:note-text-outline
        widget:
          type: trilium
          url: https://trilium.example.com

    - Paperless-ngx:
        href: https://paperless.example.com
        description: Document management
        icon: si:paperlessngx
        widget:
          type: paperlessngx
          url: https://paperless.example.com
          token: "{{HOMEPAGE_VAR_PAPERLESS_TOKEN}}"

- Infrastructure:
    - Proxmox:
        href: https://proxmox.example.com:8006
        description: Virtualization
        icon: si:proxmox
        widget:
          type: proxmox
          url: https://proxmox.example.com:8006
          token:  "root@pam!myserver"
          secret: "{{HOMEPAGE_VAR_PROXMOX_SECRET}}"

    - ArgoCD:
        href: https://argocd.example.com
        description: GitOps CD
        icon: si:argo
        widget:
          type: argocd
          url: https://argocd.example.com
          key: "{{HOMEPAGE_VAR_ARGOCD_TOKEN}}"

    - Docker engine:
        href: https://docs.docker.com
        description: Local daemon
        icon: si:docker
        widget:
          type: docker
          url: tcp://docker.local:2375

    - Glances:
        href: https://glances.example.com
        description: System monitor
        icon: si:glances
        widget:
          type: glances
          url: https://glances.example.com

    - Photoprism:
        href: https://photos.example.com
        description: Photo library
        icon: si:photoprism
        widget:
          type: photoprism
          url: https://photos.example.com
          username: "{{HOMEPAGE_VAR_PHOTOPRISM_USER}}"
          password: "{{HOMEPAGE_VAR_PHOTOPRISM_PASS}}"

    - Speedtest Tracker:
        href: https://speedtest.example.com
        description: ISP performance
        icon: si:speedtest
        widget:
          type: speedtest
          url: https://speedtest.example.com

    - Longhorn:
        href: https://longhorn.example.com
        description: K8s storage
        icon: si:rancher
        widget:
          type: longhorn

- Data:
    # customapi — `text` display mode (single formatted value)
    - Active users:
        description: customapi · display=text
        icon: mdi:account-multiple
        widget:
          type: customapi
          url: file://data/stats.json
          display: text
          mappings:
            field: active_users
            format: number

    # customapi — `list` display mode (flat key/value)
    - Build info:
        description: customapi · display=list
        icon: mdi:information-outline
        widget:
          type: customapi
          url: file://data/buildinfo.json
          display: list
          mappings:
            fields:
              - { field: version,   label: Version }
              - { field: commit,    label: Commit }
              - { field: builtAt,   label: Built,   format: date }
              - { field: sizeBytes, label: Size,    format: bytes }

    # customapi — `dynamic-list` display mode (scrollable list with links)
    - Demos catalog:
        description: customapi · display=dynamic-list
        icon: mdi:format-list-bulleted
        widget:
          type: customapi
          url: file://data/demos.json
          display: dynamic-list
          mappings:
            items: demos
            name:  title
            label: version
            target: "{url}"

    # customapi — `graph` display mode (sparkline)
    - Request rate:
        description: customapi · display=graph
        icon: mdi:chart-line
        widget:
          type: customapi
          url: file://data/metrics.json
          display: graph
          mappings:
            items: points
            name:  t
            value: rps

    # customapi — `tile` display mode (grid of metric tiles)
    - Cluster tiles:
        description: customapi · display=tile
        icon: mdi:view-grid
        widget:
          type: customapi
          url: file://data/tiles.json
          display: tile
          mappings:
            items: tiles
            name:  label
            value: value

- Administration:
    - Hello world:
        description: Trivial smoke test
        icon: mdi:rocket-launch
        script: hello

    - List containers:
        description: docker ps output
        icon: mdi:format-list-checks
        script: list-containers

    - Refresh status:
        description: Regenerate config/data/status.json
        icon: mdi:refresh
        script: generate-status

    - Restart proxy:
        description: Restart the reverse proxy
        icon: mdi:restart
        script: restart-traefik
        requireConfirm: true

    - Full backup:
        description: Run nightly backup
        icon: mdi:backup-restore
        script: backup
        requireConfirm: true

- Health:
    - Router:
        href: http://192.168.1.1
        description: ICMP ping
        icon: mdi:router-wireless
        ping: 192.168.1.1

    - Public API:
        href: https://api.github.com
        description: HTTP siteMonitor (HEAD with GET fallback)
        icon: si:github
        siteMonitor: https://api.github.com
YAML
done_ "services.yaml"

# ---------------------------------------------------------------------------
log "widgets.yaml — every top-bar widget type"
cat > "${CONFIG_DIR}/widgets.yaml" <<'YAML'
- search:
    provider: google
    target: _blank

- datetime:
    text_size: xl
    format:
      dateStyle: long
      timeStyle: short
      hour12: false

- greeting:
    text_size: lg

# Two instances to demonstrate multi-city support
- openmeteo:
    label: London
    latitude: 51.5074
    longitude: -0.1278
    timezone: Etc/UTC
    units: metric
    cache: 5

- openmeteo:
    label: New York
    latitude:  40.7128
    longitude: -74.0060
    timezone: America/New_York
    units: imperial
    cache: 5

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
YAML
done_ "widgets.yaml"

# ---------------------------------------------------------------------------
log "bookmarks.yaml — grouped pills, with and without explicit icons"
cat > "${CONFIG_DIR}/bookmarks.yaml" <<'YAML'
- Developer:
    - GitHub: { abbr: GH, href: https://github.com, icon: si:github }
    - GitLab: { abbr: GL, href: https://gitlab.com, icon: si:gitlab }
    - Docker Hub: { abbr: DH, href: https://hub.docker.com, icon: si:docker }
    - Stack Overflow: { abbr: SO, href: https://stackoverflow.com, icon: si:stackoverflow }

- Cloud:
    - AWS: { abbr: AWS, href: https://console.aws.amazon.com, icon: si:amazonwebservices }
    - Cloudflare: { abbr: CF, href: https://dash.cloudflare.com, icon: si:cloudflare }
    - Vercel: { abbr: VE, href: https://vercel.com, icon: si:vercel }
    - DigitalOcean: { abbr: DO, href: https://cloud.digitalocean.com, icon: si:digitalocean }

- AI / ML:
    - Anthropic: { abbr: AN, href: https://console.anthropic.com }
    - OpenAI: { abbr: AI, href: https://platform.openai.com }
    - Hugging Face: { abbr: HF, href: https://huggingface.co, icon: si:huggingface }

- Communities:
    - Reddit: { abbr: RE, href: https://reddit.com, icon: si:reddit }
    - HN: { abbr: HN, href: https://news.ycombinator.com, icon: si:ycombinator }
    - Lobsters: { abbr: LO, href: https://lobste.rs }
YAML
done_ "bookmarks.yaml"

# ---------------------------------------------------------------------------
log "docker.yaml — local socket"
cat > "${CONFIG_DIR}/docker.yaml" <<'YAML'
# Local Docker / Podman daemon. Uncomment the right entry for your host.
local:
  socket: /var/run/docker.sock
  swarm: false

# Remote TCP example
# remote:
#   host: 10.0.0.5
#   port: 2375

# Remote TLS example
# secure:
#   host: 10.0.0.6
#   port: 2376
#   tls:
#     ca:   /run/secrets/docker-ca.pem
#     cert: /run/secrets/docker-cert.pem
#     key:  /run/secrets/docker-key.pem
YAML
done_ "docker.yaml"

# ---------------------------------------------------------------------------
log "scripts.yaml — 5 demo scripts with metadata"
cat > "${CONFIG_DIR}/scripts.yaml" <<'YAML'
scripts:
  hello:
    command: hello.sh
    description: "Trivial hello-world (no confirm)"
    timeout: 10
    icon: mdi:rocket-launch

  list-containers:
    command: list-containers.sh
    description: "Show running containers via docker CLI"
    timeout: 15
    icon: mdi:format-list-checks
    env:
      DOCKER_HOST: unix:///var/run/docker.sock

  generate-status:
    command: generate-status.sh
    description: "Regenerate config/data/status.json"
    timeout: 30
    icon: mdi:refresh

  restart-traefik:
    command: restart-container.sh
    description: "Restart the traefik container"
    args: ["traefik"]
    timeout: 30
    requireConfirm: true
    icon: mdi:restart
    env:
      DOCKER_HOST: unix:///var/run/docker.sock

  backup:
    command: backup.sh
    description: "Run nightly backup"
    timeout: 600
    requireConfirm: true
    icon: mdi:backup-restore
    allowConcurrent: false
    logOutput: true
    env:
      BACKUP_MODE: filesystem
YAML
done_ "scripts.yaml"

# ---------------------------------------------------------------------------
log "scripts/*.sh — 5 demo scripts"
cat > "${SCRIPTS_DIR}/hello.sh" <<'BASH'
#!/bin/bash
set -e
echo "Hello from MyServer"
echo "Hostname: $(hostname)"
echo "Date:     $(date -Is)"
echo "User:     $(id -un)"
echo "Shell:    $SHELL"
echo
echo "Available env (scrubbed):"
env | sort
BASH

cat > "${SCRIPTS_DIR}/list-containers.sh" <<'BASH'
#!/bin/bash
set -euo pipefail
if ! command -v docker >/dev/null 2>&1; then
    echo "docker CLI not available in the container"; exit 1
fi
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
echo
echo "Total: $(docker ps -q | wc -l) running containers"
BASH

cat > "${SCRIPTS_DIR}/generate-status.sh" <<'BASH'
#!/bin/bash
# Refresh the data/status.json file consumed by the customapi dynamic-list widget.
set -euo pipefail
TS="$(date -u +%FT%TZ)"
cat > /app/config/data/status.json <<JSON
{
  "services": [
    { "name": "API",      "status": "running", "checkedAt": "${TS}" },
    { "name": "Database", "status": "running", "checkedAt": "${TS}" },
    { "name": "Cache",    "status": "degraded","checkedAt": "${TS}" }
  ]
}
JSON
echo "status.json regenerated at ${TS}"
BASH

cat > "${SCRIPTS_DIR}/restart-container.sh" <<'BASH'
#!/bin/bash
set -euo pipefail
NAME="${1:?container name prefix required}"
CT=$(docker ps --filter "name=^${NAME}" --format '{{.Names}}' | head -1)
[ -z "$CT" ] && { echo "ERROR: no container matched prefix ${NAME}"; exit 1; }
echo "Restarting $CT…"
docker restart "$CT"
echo "Done."
BASH

cat > "${SCRIPTS_DIR}/backup.sh" <<'BASH'
#!/bin/bash
# Demo backup — does NOT touch anything real. Replace with your own logic.
set -euo pipefail
MODE="${BACKUP_MODE:-filesystem}"
echo "Starting ${MODE} backup at $(date -Is)"
sleep 1
echo "Step 1/3 — snapshotting volumes…   OK"
sleep 1
echo "Step 2/3 — uploading to ${MODE}…   OK"
sleep 1
echo "Step 3/3 — pruning old backups…    OK"
echo "Backup complete at $(date -Is)"
BASH

chmod +x "${SCRIPTS_DIR}"/*.sh
done_ "scripts/{hello,list-containers,generate-status,restart-container,backup}.sh"

# ---------------------------------------------------------------------------
log "data/*.json — five JSON sources for the customapi display modes"
cat > "${DATA_DIR}/stats.json" <<'JSON'
{
  "active_users": 1283,
  "online_now":   142,
  "since":        "2026-05-18T00:00:00Z"
}
JSON

cat > "${DATA_DIR}/buildinfo.json" <<'JSON'
{
  "version":   "1.3.0",
  "commit":    "0f8a786",
  "builtAt":   "2026-05-18T19:00:00Z",
  "sizeBytes": 15550391
}
JSON

cat > "${DATA_DIR}/demos.json" <<'JSON'
{
  "demos": [
    { "title": "Auth service",    "version": "v2.1",  "url": "https://auth.local" },
    { "title": "API Gateway",     "version": "v1.5",  "url": "https://gateway.local" },
    { "title": "Search index",    "version": "v0.9",  "url": "https://search.local" },
    { "title": "Notification hub","version": "v3.0",  "url": "https://notify.local" },
    { "title": "Reporting",       "version": "v1.0",  "url": "https://reports.local" }
  ]
}
JSON

cat > "${DATA_DIR}/metrics.json" <<'JSON'
{
  "points": [
    { "t": "10:00", "rps": 110 },
    { "t": "10:05", "rps": 145 },
    { "t": "10:10", "rps": 132 },
    { "t": "10:15", "rps": 198 },
    { "t": "10:20", "rps": 165 },
    { "t": "10:25", "rps": 220 },
    { "t": "10:30", "rps": 188 },
    { "t": "10:35", "rps": 244 }
  ]
}
JSON

cat > "${DATA_DIR}/tiles.json" <<'JSON'
{
  "tiles": [
    { "label": "Nodes",       "value": 6 },
    { "label": "Pods",        "value": 142 },
    { "label": "Containers",  "value": 318 },
    { "label": "CPU %",       "value": 47 },
    { "label": "Memory GiB",  "value": 38 },
    { "label": "Network MiB", "value": 124 }
  ]
}
JSON

# `status.json` is initially generated by the `generate-status` script. We
# seed a baseline so the corresponding widget renders on first load.
cat > "${DATA_DIR}/status.json" <<'JSON'
{
  "services": [
    { "name": "API",      "status": "running",  "checkedAt": "1970-01-01T00:00:00Z" },
    { "name": "Database", "status": "running",  "checkedAt": "1970-01-01T00:00:00Z" },
    { "name": "Cache",    "status": "degraded", "checkedAt": "1970-01-01T00:00:00Z" }
  ]
}
JSON
done_ "data/{stats,buildinfo,demos,metrics,tiles,status}.json"

# ---------------------------------------------------------------------------
log "custom.css + custom.js — subtle visual tweaks"
cat > "${CONFIG_DIR}/custom.css" <<'CSS'
/* Slightly rounded cards + glow on status dots. */
.service-card  { border-radius: 14px; }
.status-dot    { box-shadow: 0 0 5px currentColor; }
.bookmark-item { border-radius: 999px; }
CSS

cat > "${CONFIG_DIR}/custom.js" <<'JS'
// Press "/" to focus the search box (anywhere outside an input).
document.addEventListener('DOMContentLoaded', () => {
  document.addEventListener('keydown', (e) => {
    if (e.key === '/' && document.activeElement?.tagName !== 'INPUT') {
      e.preventDefault();
      document.getElementById('search-input')?.focus();
    }
  });
});
JS
done_ "custom.css + custom.js"

# ---------------------------------------------------------------------------
echo
log "Done. Summary:"
ls -1 "${CONFIG_DIR}"        | sed 's/^/    /'
echo "    scripts/"
ls -1 "${SCRIPTS_DIR}"       | sed 's/^/      /'
echo "    data/"
ls -1 "${DATA_DIR}"          | sed 's/^/      /'

echo
echo "Next steps:"
echo "  1. (Optional) set HOMEPAGE_VAR_* env vars for any upstream widgets:"
echo "       export HOMEPAGE_VAR_PLEX_TOKEN=…  HOMEPAGE_VAR_SONARR_KEY=…  …"
echo "     Cards without a working backend will show a placeholder error;"
echo "     the layout and customapi+file:// widgets render fine without any."
echo "  2. The dashboard hot-reloads on save via fsnotify — no restart needed."
echo "  3. Open the dashboard:    http://localhost:8085/"
echo "  4. Validate parse status: curl -s http://localhost:8085/api/validate"
