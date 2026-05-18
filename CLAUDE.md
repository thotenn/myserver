# CLAUDE.md — MyServer

Self-hosted dashboard. Go rewrite of Homepage (Next.js/React) using
**Templ + HTMX + Tailwind CSS v3**. 100% compatible with Homepage YAML files.
Target: ~50–80 MiB RAM, single ~14 MB binary, ~30 MB Docker image.

- **User documentation**: `README.md`
- **Agent skill (add features without writing Go)**: `.agents/skills/add-widget/`
- **Context docs**: `docs/context/` (architecture, glossaries, workflow)

---

## Commands

```bash
make build       # templ generate + tailwindcss --minify + go build -ldflags="-s -w"
make test        # go test ./...
make test-race   # go test -race ./...   ← MANDATORY before merging changes in internal/scripts/
make test-cover  # coverage per package
make lint        # gofmt -l + go vet
make tidy        # go mod tidy
make dev         # hot reload with air
make templ       # regenerate *_templ.go
make tailwind    # compile web/static/css/main.css
make up | down | logs   # docker compose wrappers
```

---

## Package map

| Package | Role | Notes |
|---|---|---|
| `cmd/myserver` | Entry point. Wires logger, config, watcher, scripts, router, server. | `main()` is decomposed into `initLogger / initConfig / initWidgets / initWatcher / initScriptManager / startServer`. |
| `internal/config` | YAML loaders, env substitution, fsnotify watcher, in-memory cache. | No dependency on other internal packages — it is the foundation. Hash in `atomic.Value`. |
| `internal/handlers` | HTTP handlers. | Content negotiation via `HX-Request` header: HTML (Templ) for HTMX, JSON for API clients. |
| `internal/templates` | Templ sources + `*_templ.go` (committed) + helpers (`urls.go`, `icons.go`, `styles.go`, `layout.go`, `format.go`) + `i18n.go`. | |
| `internal/widgets` | Declarative registry (160+ entries) + `BaseWidget` + interfaces. | `GenericProxyHandler` reads `APITemplate()` and `Mappings()` from the registry at request time. |
| `internal/proxy` | Secure HTTP proxy. SSRF guard, transport pool, gzip/zlib decompression, `file://` scheme, TTL cache. | `scrubError()` sanitizes credentials in error strings. |
| `internal/scripts` | Opt-in script execution. | Strong sandboxing. Hot-reloaded by the config watcher via `Manager.ReplaceAll`. |
| `internal/middleware` | Recovery, Logging, RateLimit, CORS (same-origin), HostValidation (port-aware), SecurityHeaders (CSP, HSTS opt-in), TrustedProxy. | |
| `internal/discovery` | Docker/Podman label discovery + `MergeServices` (config wins over discovery). | |
| `web/static`, `web/tailwind` | Compiled CSS/JS + Tailwind source. `input.css` uses `@layer base` with `@apply`. | |

---

## Critical gotchas

### Templ

- **Never interpolate `{ ... }` inside `<script>` or `<style>` blocks** — Templ
  emits them literally. For dynamic JS/CSS use `data-*` attributes and read
  them from `web/static/js/app.js`.
- **Dynamic attributes**: `href={ "/x?v=" + data.Hash }`, not
  `href="/x?v={ data.Hash }"` (the latter is emitted as a literal `{`).
- **`style` for grid layouts**: the helper must return the complete declaration
  (`grid-template-columns: repeat(4, 1fr);`), not just the value.
- **CSP is `script-src 'self' …`**: no inline `onclick`/`onsubmit`/`onerror`.
  Attach listeners in `app.js` via `addEventListener`.

### Tailwind CSS

- **Tailwind v3 standalone CLI only.** v4 broke `@layer base` with `@apply`,
  which `input.css` relies on. Dockerfile pins `v3.4.17`.
- If a class doesn't render, it's almost always because it lives in a path that
  isn't scanned. Either ensure the file is in `content`, or add a pattern to
  `safelist` in `tailwind.config.js` (the JS config — not any `.json`).
- Icons resolve via `homarr-labs/dashboard-icons` (jsdelivr), MDI SVG
  (`mdi-` and `mdi:`), Simple Icons (`si-` and `si:`), or absolute URL.

### Config & hot-reload

- Handlers MUST read `config.CurrentHash()` (atomic.Value) per request. Never
  capture the hash in a closure at startup.
- The fsnotify watcher reacts to `.yaml`, `.yml`, `.css`, `.js` changes.
  All config caches and the hash are refreshed atomically.
- `scripts.yaml` is hot-reloaded too — the watcher calls
  `scripts.Manager.ReplaceAll()`. No restart needed.
- **`settings.scripts.scriptDirs` is read only at `initScripts()`** in
  `cmd/myserver/main.go`. The watcher calls `registerScripts` which uses the
  EXISTING `Manager`, never rebuilds it. Changes to `scriptDirs`,
  `maxTimeout`, `defaultTimeout`, `maxConcurrent` require a process restart.
- Env substitution: `{{HOMEPAGE_VAR_X}}` → env value, `{{HOMEPAGE_FILE_X}}` →
  file contents. If unresolved, the placeholder is kept literally (fail-visible).
- **`/api/validate` is too lenient.** Go's `yaml.v3` accepts ambiguous syntax
  like `key:{flow}` (missing space after `:`) and silently produces the
  wrong shape. Strict YAML parsers reject it. When debugging "config parses
  but downstream is empty", run a strict linter (`python -c "import yaml; …"`).

### Scripts feature (opt-in)

**Run `go test -race -count=1 ./internal/scripts/` before any change here.**

- Aggressive env scrubbing. Default env is only `PATH`, `HOME=/tmp`,
  `USER=myserver`, `SHELL=/bin/bash`, `TZ`. Anything else (including
  `DOCKER_HOST`, the real `HOME`) must be declared in `scripts.yaml: env:`.
- Podman rootless requires `HOME=/home/$USER` + `XDG_RUNTIME_DIR` (podman reads
  `~/.config/containers/storage.conf`).
- `requireConfirm` is enforced server-side via header `X-Homepage-Confirm: yes`
  → 428 otherwise. Do not rely only on `hx-confirm` from the browser.
- Path safety: `filepath.EvalSymlinks` + `HasPrefix(real, dir+sep)` defeats
  prefix collisions (e.g. `/app/scriptsbak/evil.sh`). Only regular `.sh` files,
  no devices/sockets/FIFOs, no world-writable files (`mode & 0o002 != 0`).
- Env denylist (rejected at registration): `LD_PRELOAD`, `LD_LIBRARY_PATH`,
  `LD_AUDIT`, `BASH_ENV`, `ENV`, `PROMPT_COMMAND`, `IFS`, `PATH`, `BASH_FUNC_*`.
- Process group + `SIGTERM` → 5s grace → `SIGKILL` (`cmd.WaitDelay`). Output
  capped at 1 MiB. Global semaphore (`scripts.maxConcurrent`, default 5).
- Endpoint exposure is conditional on `HOMEPAGE_SCRIPTS_ENABLED=true`; when
  disabled, the routes are not registered (handlers also double-check).

### Security model

- **No internal auth.** The dashboard expects an auth layer in front (Cloudflare
  Access in the production deployment). Do not plan internal login.
- `HostValidation` always seeds `localhost:PORT` / `127.0.0.1:PORT` /
  `[::1]:PORT` defaults; `HOMEPAGE_ALLOWED_HOSTS` extends the list. `*` is the
  explicit wildcard.
- CORS is same-origin and applies **only** to `/api/*` — it reflects `Origin`
  only when it equals `Host`. The HTML dashboard route has no CORS headers.
- Rate limiting: per-IP token bucket. 60/min default; 10/min on script
  execution; 1/min on `/api/hash` and `/api/reload`.
- Security headers globally: CSP (`script-src 'self' unpkg.com cdn.jsdelivr.net`),
  `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`,
  `X-Content-Type-Options: nosniff`. HSTS opt-in via `HOMEPAGE_ENABLE_HSTS`.
- SSRF: `proxy.Proxy` resolves DNS, blocks cloud-metadata IPs always, and
  blocks RFC1918 + loopback unless `HOMEPAGE_ALLOW_PRIVATE_HOSTS=true`
  (default `true`, since the dominant use case is self-hosted).
- Credential sanitization: `/api/widgets` and `/api/services` deep-strip keys
  matched by `IsSensitiveKey` (case-insensitive substring) and remove
  basic-auth userinfo from URLs.

---

## Test focus

| Package | Coverage | Security tests |
|---|---|---|
| `internal/scripts` | 66.7% | 14 adversarial tests: path traversal, `.sh` enforcement, env denylist, race, timeout kill-tree, output cap, hot-reload via `ReplaceAll`. |
| `internal/middleware` | 38.9% | `HostValidation`: defaults, wildcard, port-awareness, case. |
| `internal/handlers` | 10.2% | Basic-auth strip, recursive widget sanitization, scripts disabled → 404. |

---

## Conventions

- Commits in English. Format: `myserver: phase N description` (when working
  through the phased checklist) or `myserver: short description` (one-offs).
- Tests live next to source as `*_test.go`.
- Logs go through `zap` (structured). Never `fmt.Printf`.
- Templates consume `config.Service`, `config.ServiceGroup`, etc. directly.
- i18n: `T(lang, "key")` with hardcoded maps in `internal/templates/i18n.go`.
- Handlers return generic messages to clients; log details internally. Never
  leak paths, stack traces, or upstream URLs.
- Don't break the `internal/config → everything else` dependency direction.

---

## Deploy at a glance

- Multi-stage Docker. Runtime: `alpine:3.21` + `su-exec` + `bash` + `docker-cli`
  + `wget` + `tini` + `tzdata`. User `myserver:1000` non-root.
- Required mounts:
  - `/opt/myserver/config → /app/config` — host bind, hot-reloaded.
  - `/var/run/docker.sock` — Docker stats + script wrappers (mount `:ro` if
    scripts don't need to mutate containers).
- Key env: `HOMEPAGE_ALLOWED_HOSTS`, `HOMEPAGE_SCRIPTS_ENABLED`,
  `HOMEPAGE_ALLOW_PRIVATE_HOSTS`, `HOMEPAGE_ENABLE_HSTS`,
  `HOMEPAGE_TRUSTED_PROXIES`, `HOMEPAGE_PROXY_DISABLE_IPV6`, `TZ`. See
  `README.md` for the full table.

---

## Open work

- Audit log to an append-only file (currently writes to stderr).
- `layout.<group>.tab` is parsed and `TabNavigation` Templ component exists,
  but `index.templ` never invokes it — groups always render as flat `<h2>`
  sections. Either wire it up or remove the field from `Settings`.
- `cmd/myserver/main.go: initScripts` reads `scriptDirs` once. Either rebuild
  the `Manager` on hot-reload or document this as intentional in `README.md`
  alongside the `scripts.yaml` hot-reload note.
- Dockerfile installs `templ@latest`; if it ever drifts ahead of the runtime
  pinned in `go.mod`, the build breaks. Currently pinned to `v0.3.1001` via
  the `TEMPL_VERSION` build arg — bump in lockstep with `go.mod`.
