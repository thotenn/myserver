# Troubleshooting Playbook

> Symptoms → likely cause → fix. Start with the section that matches the
> behaviour you see; each item lists the order in which to check things.

For Podman-specific bind-mount issues (rootless ownership, SELinux relabel)
see also [`deploy.md`](./deploy.md#caveats-on-the-dev-compose).

---

## Dashboard

### "Host validation failed"

The request's `Host` header doesn't match `HOMEPAGE_ALLOWED_HOSTS`.

- Production: `HOMEPAGE_ALLOWED_HOSTS=dashboard.example.com` (or your subdomain).
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
  HOME:            /home/youruser
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

---

## Authentication (email allowlist)

Only applies when `config/auth.yaml` lists at least one address. Full guide:
[`authentication.md`](./authentication.md).

### The whole dashboard answers 503

The auth policy could not be read, and an unreadable policy deliberately never
degrades to a public dashboard.

1. `docker logs <container> 2>&1 | grep -i auth` — the log names the cause.
2. Usual suspects: a YAML syntax error in `auth.yaml`, the file missing (a bind
   mount that did not come up), or a first-ever policy failing validation.
3. Fix the file; the next request recovers, no restart needed.
4. To reopen the dashboard right now, set `emails: []` — a well-formed file
   with an empty allowlist. **Deleting the file does not work**: with sign-in
   active, a vanished file is treated as a failure, not as consent to go
   public.

### The container restarts in a loop after enabling sign-in

Startup validation failed. The log names the field, e.g. *"google.clientId
still holds an unresolved placeholder"*.

1. The environment variables did not reach the container:
   `docker exec <container> printenv | grep HOMEPAGE_VAR_GOOGLE`
2. On Compose, a host variable is only forwarded if the service declares it
   under `environment:`. Setting it in a PaaS UI populates the *deployment*
   environment; the Compose file still has to pass it through.
3. Set the variables **before** writing `auth.yaml`, not after.

### Google rejects the login with `redirect_uri_mismatch`

`redirectURL` in `auth.yaml` and the *Authorized redirect URI* in the Google
console differ. Compare them character by character: `/auth/google/callback`
(slashes, not dots), same scheme, same host, no trailing slash. Console changes
take a few minutes to propagate.

### Sign-in succeeds but the dashboard answers 403

Working as intended: the account is authenticated but not on the allowlist.
`grep "not in allowlist"` in the logs shows the address that tried. Matching
ignores case and whitespace but not Gmail dots — `j.perez@` and `jperez@` are
distinct entries.

### Everyone is signed out after every deploy

`session.secret` is unset, so a new random key is generated at each start (the
startup log warns about it). Set a fixed value to keep sessions across
deployments.

### The login page loads but signing in never completes

Over plain HTTP the cookies are dropped: the session cookie is `Secure` and the
OAuth state cookie uses the `__Host-` prefix, which browsers refuse without
HTTPS. For local testing set `session.secure: false`; never in production.

### A widget card shows the login page

HTMX requests are answered with `401` + `HX-Redirect`, never login HTML, so
this means the request reached the gate without the `HX-Request: true` header —
check any custom JavaScript that issues it.

---

## Several dashboards

### A dashboard URL answers 404

Either there is no directory of that name under `<config dir>/dashboards/`, or
the name was refused. Names are one path segment of `A-Za-z0-9._~-`, and `api`,
`auth` and `static` are reserved because they would shadow the root dashboard's
own routes. A refused directory says so at startup:

```
{"msg":"ignoring a dashboard directory","error":"dashboard slug \"api\" is reserved: it would shadow the /api routes"}
```

The startup log also lists what IS being served — `serving a client dashboard`,
one line each — so compare that against the directories on disk.

### A nested dashboard's URL serves the ROOT dashboard's content

The reverse proxy is rewriting the path. The first segment after any base path
is how MyServer decides which dashboard you asked for, so a rule that strips
`/acme` leaves a request that looks like the root dashboard's. Pass the path
through unchanged: no `strip_prefix`, no `rewrite ^/acme(.*) $1`, no
`StripPrefix` middleware.

### A card on a nested dashboard renders empty

Nested dashboards are read-only, and the endpoints behind those cards are not
registered for them: `/api/services/proxy` (any `widget:` that needs a
credential), `/api/widgets/resources`, `/api/docker/*`, `/api/proxmox/*`,
`/api/scripts/*`. They answer 404 and the frontend leaves the card empty. Links,
descriptions and the `ping:` / `siteMonitor:` status indicators are what a nested
dashboard supports; anything needing a credential belongs in the root dashboard.

### One dashboard answers 503 everywhere, the others are fine

That dashboard's `auth.yaml` cannot be read, or vanished while sign-in was
active. Failing closed is deliberate — a vanished policy is indistinguishable
from a deleted gate — and it is confined to that dashboard so the rest of the
host keeps serving. The log names it. See
[`authentication.md`](./authentication.md).

If the **whole** process refuses to start instead, it is the ROOT dashboard's
policy: that one is fatal at startup.

### A new dashboard directory is not picked up

The watcher notices directories created under `<config dir>/dashboards/` and
logs `dashboard added`. If that line never appears, the directory was created
somewhere else — check that it is directly under `dashboards/`, not nested
deeper — or the name was refused (see above). Restarting the process re-scans
from scratch and will log the reason either way.

### Signing in to one dashboard signs me out of another

It should not, and does not: each dashboard's session cookie has its own name
(`myserver_session`, `myserver_session_acme`), its own `Path` and its own signing
key. If it happens, look for a `session.cookieName` in one dashboard's
`auth.yaml` that collides with another's — an explicit name overrides the
per-dashboard default that exists to prevent exactly this.

### `redirect_uri_mismatch` after adding a dashboard

`google.redirectURL` in that dashboard's `auth.yaml` does not match an
authorised redirect URI in the Google console. Nested dashboards normally point
at the **root** dashboard's callback, which is already registered — the dashboard
the login belongs to travels inside the signed OAuth state, so no new URI is
needed. Only point it at `/<name>/auth/google/callback` if that dashboard has its
own OAuth client, and register that URI too.
