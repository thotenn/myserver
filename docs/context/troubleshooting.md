# Troubleshooting Playbook

> Symptoms → likely cause → fix. Start with the section that matches the
> behaviour you see; each item lists the order in which to check things.

For Podman-specific bind-mount issues (rootless ownership, SELinux relabel)
see also [`deploy.md`](./deploy.md#caveats-on-the-dev-compose).

---

## Dashboard

### "Host validation failed"

The request's `Host` header doesn't match `HOMEPAGE_ALLOWED_HOSTS`.

- Production: `HOMEPAGE_ALLOWED_HOSTS=htop.thotenn.com` (or your subdomain).
- Dev: defaults include `localhost:3000`, `127.0.0.1:3000`, `[::1]:3000`. If
  you changed `-port`, extend the env var accordingly.
- Logs show the rejected `Host` before returning 400.

### `/api/validate` says `valid:true` but groups / widgets are missing

Go's `yaml.v3` is lenient and accepts ambiguous syntax that strict parsers
reject — most commonly missing space after `:` in flow mappings:

```yaml
Infrastructure:{ columns: 2, tab: Infra }   # silent bad parse
Infrastructure: { columns: 2, tab: Infra }  # correct
```

The file looks valid to MyServer but downstream code reads the wrong shape
and silently drops keys (often everything after the bad line). Lint with a
strict parser:

```bash
python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" \
  config/*.yaml
```

### Icons not loading

1. Verify the icon name exists in
   [homarr-labs/dashboard-icons](https://github.com/homarr-labs/dashboard-icons).
2. Inspect the rendered `<img src=…>`.
3. Confirm the client can reach `cdn.jsdelivr.net` (firewall?).
4. Fallback: absolute URL in the YAML.

### Background image not rendering

1. For a local file, confirm
   `GET /api/config/<path>` returns 200 and the right `Content-Type`.
   Allowed extensions: `.png` `.jpg` `.jpeg` `.webp` `.gif` `.svg`
   `.avif` `.ico` `.bmp`.
2. Inspect `<body style="...">` — there should be
   `background-image: url(/api/config/<path>?v=<hash>)` (unquoted).
   If you see `&amp;#34;` or `\000022` in the URL, you're on a stale
   binary; rebuild.
3. For remote URLs, confirm the host is reachable from the browser and
   served over HTTPS (CSP `img-src` allows `https:` and `data:` but not
   `http:`).
4. Pair with `cardBlur: true` if cards look unreadable on top of a busy
   image.

---

## Widget data

### Widget shows "Loading…" forever

1. Open DevTools → Network → find the `/api/services/proxy?...` request.
2. Inspect the response:

   | Status / body | Likely cause |
   |---|---|
   | `200` + HTML | Already loaded; the `display` mode may be wrong (lists need `display: dynamic-list`). |
   | `200` + JSON | The browser is not in HTMX mode. `dynamic-list` returns HTML only for HTMX. |
   | `502` / `504` | Upstream unreachable from inside the container. |
   | `429` | Rate limited. `Retry-After` header tells you how long. |
   | `428` | Script `requireConfirm: true` but no `X-Homepage-Confirm: yes`. |

3. Error bodies are scrubbed of credentials, so they're safe to share.

### Resources widget shows 0 %

- First call always returns 0 % (the calc is a delta between two samples
  — wait one polling interval).
- Containers with isolated `/proc` (rare) report container-level stats
  instead of host stats.

### `siteMonitor` shows ERR

1. Reachable from inside the container?
   `docker exec myserver wget -qO- https://api.example.com/health`
2. The URL must match an actual service in `services.yaml` (open-proxy
   guard).
3. Endpoint must accept `HEAD` or `GET` (HEAD is tried first).
4. Self-signed certs fail TLS validation — terminate TLS at a reverse
   proxy.

### Ping shows "offline"

1. Reachable? `docker exec myserver ping -c 1 192.168.1.1`.
2. UDP-mode ping is used (no `CAP_NET_RAW`). Some hosts/firewalls drop
   ICMP — prefer `siteMonitor` for HTTP services.

### 503 / 502 on `customapi`

1. Upstream reachable from the container?
2. `HOMEPAGE_ALLOW_PRIVATE_HOSTS=true` (default) is required for
   intra-network targets.
3. For local data, prefer `url: file://data/<file>.json`.
4. Verify `mappings` matches the JSON structure.

### Rate limit (429)

Default limits: 60/min for most routes, 10/min for scripts, 1/min for
`/api/hash`. `Retry-After` is set. If too many widgets are polling,
reduce the number of active widgets or extend their intervals via
`cache:` where supported.

---

## Docker / containers

### Docker stats: 0 % CPU / 0 bytes

1. Socket mounted in the container and readable by the user.
2. `docker.yaml` server has the correct socket path.
3. Container name matches exactly (no leading `/`).
4. **The first call always returns 0 % CPU** — the calc is a delta
   between two samples; wait 5 s.

---

## Scripts feature

### Script returns 404 or "not found"

Checklist:

1. `HOMEPAGE_SCRIPTS_ENABLED=true` on the container?
2. File executable on the host? `chmod +x config/scripts/<name>.sh`.
3. `command:` is relative (no leading `/`), ends in `.sh`, and lives in a
   directory listed in `settings.yaml: scripts.scriptDirs`.
4. **Did you just add or change `settings.scripts.scriptDirs`?** That
   field is read only when the process starts. The fsnotify watcher
   re-registers `scripts.yaml` entries hot, but does not rebuild the
   script manager. Restart the container after editing `scriptDirs`,
   `maxTimeout`, `defaultTimeout`, or `maxConcurrent`.
5. World-writable? `chmod o-w config/scripts/<name>.sh` (rejected if
   `0o002`).
6. `docker compose logs -f myserver` will print a precise rejection
   reason on startup (path-traversal, denylisted env, bad extension,
   etc.).

### Scripts execute but can't see Docker / Podman containers

The parent process env is not inherited. Declare what's needed in
`scripts.yaml`:

```yaml
env:
  DOCKER_HOST: unix:///var/run/docker.sock
  # Podman rootless:
  XDG_RUNTIME_DIR: /run/user/1000
  HOME:            /home/tho
```

---

## Hot-reload

### YAML hot-reload not firing

1. `fsnotify` watches `.yaml`, `.yml`, `.css`, `.js`.
2. Atomic-save editors (e.g. `vim` with `:set nobackup`) can fire only a
   `RENAME` event — try `:set backupcopy=yes` or `touch <file>` after
   save.
3. Files must be in `HOMEPAGE_CONFIG_DIR` (top level only).
4. `scripts.yaml` entries hot-reload via `Manager.ReplaceAll()`. Changes
   to `settings.scripts.scriptDirs` (etc.) do NOT — see
   [`scripts.md`](./scripts.md#hot-reload-caveat).
