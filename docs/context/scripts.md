# Scripts Feature — Complete Guide

> Opt-in feature for safe execution of `.sh` scripts from the dashboard.
> Disabled by default (`HOMEPAGE_SCRIPTS_ENABLED=false`).

For the YAML schema and quick start, see
[`configuration.md`](./configuration.md#scriptsyaml). For the HTTP endpoints,
see [`api.md`](./api.md#scripts).

---

## Pipeline

1. `.sh` files live in a whitelisted directory (default `/app/scripts/` or
   `/app/config/scripts/`).
2. `scripts.yaml` registers each script with metadata and explicit env vars.
3. `services.yaml` references the script with `type: script` to render a
   button card.
4. Click → HTMX `POST /api/scripts/{name}` with `X-Homepage-Confirm: yes`
   when `requireConfirm: true`.
5. Backend validates the whitelist, forks `/bin/bash <script>` in an
   isolated process group with a timeout, captures output (1 MiB cap).
6. Result rendered back into the card: status badge
   (success / error / timeout / cancelled), exit code, duration,
   scrollable log.
7. Each execution is recorded in the audit log.

---

## Minimal setup

```yaml
# settings.yaml
scripts:
  scriptDirs:
    - /app/scripts
  maxTimeout: 3600
  defaultTimeout: 60
  maxConcurrent: 5
```

```yaml
# scripts.yaml
scripts:
  hello:
    command: hello.sh
    description: "Test script"
    timeout: 10
    icon: mdi-rocket
```

```yaml
# services.yaml
- Maintenance:
    - Hello:
        type: script
        script: hello
        icon: mdi-rocket
```

```bash
# /app/scripts/hello.sh
#!/bin/bash
set -e
echo "Hello world from MyServer"
echo "Hostname: $(hostname)"
echo "Date: $(date)"
```

---

## Env vars in scripts

Scripts do **not** inherit the parent process env. The executor builds a
minimal env:

```
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
HOME=/tmp
USER=myserver
SHELL=/bin/bash
TZ=<parent process TZ>
```

Any additional variable must be declared in `scripts.yaml`:

```yaml
scripts:
  my-script:
    command: my-script.sh
    env:
      DOCKER_HOST: unix:///var/run/docker.sock

      # Podman rootless (local dev):
      XDG_RUNTIME_DIR: /run/user/1000
      DOCKER_HOST:     unix:///run/user/1000/podman/podman.sock
      HOME:            /home/tho       # podman reads ~/.config/containers/

      BACKUP_MODE:   s3
      BACKUP_BUCKET: my-bucket
```

**Denylist** (rejected at registration): `LD_PRELOAD`, `LD_LIBRARY_PATH`,
`LD_AUDIT`, `BASH_ENV`, `ENV`, `PROMPT_COMMAND`, `IFS`, `PATH`, `BASH_FUNC_*`.

---

## Common recipes

### List Docker containers

```yaml
# scripts.yaml
scripts:
  list-containers:
    command: list-containers.sh
    description: "Active containers"
    timeout: 10
    icon: mdi-format-list-bulleted
    env:
      DOCKER_HOST: unix:///var/run/docker.sock
```

```bash
#!/bin/bash
# /app/scripts/list-containers.sh
set -e
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
echo
echo "Total: $(docker ps -q | wc -l) containers"
```

### Restart a container by name

```yaml
scripts:
  restart-cloudflared:
    command: restart-container.sh
    description: "Restart Cloudflare Tunnel"
    args: ["cloudflared"]
    timeout: 30
    requireConfirm: true
    icon: mdi-restart
    env:
      DOCKER_HOST: unix:///var/run/docker.sock
```

```bash
#!/bin/bash
# /app/scripts/restart-container.sh
set -euo pipefail
NAME="${1:?container name required}"
CT=$(docker ps --filter "name=^${NAME}" --format '{{.Names}}' | head -1)
[ -z "$CT" ] && { echo "ERROR: no container with prefix ${NAME}"; exit 1; }
echo "Restarting $CT…"
docker restart "$CT"
echo "Done."
```

### Regenerate `services.yaml` from running containers

```yaml
scripts:
  deploy-config:
    command: deploy-config.sh
    description: "Regenerate services.yaml"
    timeout: 60
    requireConfirm: true
    icon: mdi-cog-refresh
    env:
      DOCKER_HOST: unix:///var/run/docker.sock
      CONFIG_DIR:  /app/config
```

```bash
#!/bin/bash
# /app/scripts/deploy-config.sh
set -euo pipefail
CONFIG_DIR="${CONFIG_DIR:-/app/config}"
detect() { docker ps --format '{{.Names}}' | grep "^$1" | head -1; }

CT_VIKUNJA=$(detect vikunja)
CT_WORDPRESS=$(detect wordpress)
# …

cat > "$CONFIG_DIR/services.yaml" <<YAML
- Applications:
    - Vikunja:
        container: ${CT_VIKUNJA}
        # …
YAML

echo "services.yaml regenerated"
# fsnotify in myserver detects the change and hot-reloads
```

### Full backup

```yaml
scripts:
  backup:
    command: backup.sh
    description: "Full backup of all services"
    timeout: 1800
    requireConfirm: true
    icon: mdi-backup
    allowConcurrent: false
    logOutput: true
    env:
      BACKUP_MODE: s3
      DOCKER_HOST: unix:///var/run/docker.sock
```

```bash
#!/bin/bash
# /app/scripts/backup.sh
set -euo pipefail
cd /app/backup-manager || exit 1
exec python3 backup_manager.py --auto --mode "${BACKUP_MODE:-filesystem}"
```

---

## Security rules

1. **`.sh` only** — every other extension is rejected at registration.
2. Whitelisted `scriptDirs` only (default `/app/scripts/`).
3. **No absolute paths** — `command: /etc/passwd` is rejected.
4. **No path traversal** — `..` is rejected; symlinks resolved with
   `EvalSymlinks` and prefix-checked (`HasPrefix(real, dir+sep)`).
5. **Regular files only** — devices, sockets, FIFOs rejected.
6. **`requireConfirm` server-side** — missing `X-Homepage-Confirm: yes` →
   HTTP 428 (`hx-confirm` from the browser alone is not enough).
7. **Process group + SIGTERM-graceful** — on cancel/timeout, SIGTERM goes
   to the entire process group; after 5 s grace `cmd.WaitDelay` escalates
   to SIGKILL.
8. **1 MiB output cap** — output beyond that is truncated with
   `[output truncated at 1MB]`.
9. **Audit log** — each execution records timestamp, name, status, exit
   code, duration, client IP, started-at. In production redirect stderr
   to an append-only file (open work item).

---

## Hot-reload caveat

`scripts.yaml` entries are hot-reloaded — the watcher calls
`scripts.Manager.ReplaceAll()` on any change. **But** `scripts.scriptDirs`,
`scripts.maxTimeout`, `scripts.defaultTimeout`, and `scripts.maxConcurrent`
in `settings.yaml` are read **once** at `initScripts()` and frozen on the
existing `*scripts.Manager`. Changing those four fields requires a
container restart.
