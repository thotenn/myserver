# CLAUDE.md — MyServer

Self-hosted dashboard. Go rewrite of Homepage (Next.js/React) using
**Templ + HTMX + Tailwind CSS v3**. 100% compatible with Homepage YAML files.
Target: ~50–80 MiB RAM, single ~14 MB binary, ~30 MB Docker image.

- **User documentation**: `README.md`
- **Agent skill — configure a running instance, no Go**: `.agents/skills/add-widget/`
  (the content of one dashboard: cards, widgets, bookmarks, icons)
- **Agent skill — the dashboards themselves**: `.agents/skills/sk-clients/`.
  How many there are, which config directory feeds each one, which URL prefix it
  answers on, and who is allowed in. Read it before adding or changing a
  dashboard, and before touching `HOMEPAGE_BASE_PATH` or a per-dashboard
  `auth.yaml`.
- **Agent skill — change the UI code itself** (Templ, Tailwind, HTMX, the
  handlers that render HTML): `.agents/skills/sk-ui/`. Read it before touching
  anything under `internal/templates/` or `web/`.
- **Context docs**: `docs/context/` (architecture, configuration schema, API
  reference, features, deploy, scripts, authentication, troubleshooting,
  glossaries)

> `docs/context/` is public documentation about the software. Keep it free of
> deployment specifics — hostnames, host ports, host paths, reverse-proxy or
> tunnel topology, PaaS particulars. Use placeholders (`dashboard.example.com`,
> `<HOST_PORT>`, "the reverse proxy") and let real values live in the
> deployment environment.

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
make dashboard SLUG=acme  # scaffold a client dashboard config (sk-clients)
```

---

## Package map

| Package | Role | Notes |
|---|---|---|
| `cmd/myserver` | Entry point. Wires logger, config, watcher, scripts, router, server. | `main()` is decomposed into `initLogger / initConfig / initWidgets / initWatcher / initScriptManager / startServer`. |
| `internal/config` | YAML loaders, env substitution, fsnotify watcher, in-memory cache. | No dependency on other internal packages — it is the foundation. Hash in `atomic.Value`. |
| `internal/handlers` | HTTP handlers. | Content negotiation via `HX-Request` header: HTML (Templ) for HTMX, JSON for API clients. |
| `internal/templates` | Templ sources + `*_templ.go` (committed) + helpers (`urls.go`, `icons.go`, `styles.go`, `layout.go`, `format.go`) + `i18n.go`. | |
| `internal/widgets` | Declarative registry (46 widget types + 4 aliases) + `BaseWidget` + interfaces. | `GenericProxyHandler` reads `APITemplate()` and `Mappings()` from the registry at request time. |
| `internal/auth` | Optional email allowlist: session cookie (HMAC), Google OAuth, `trustedHeader` provider. Stdlib only. | Depends on `internal/config` and nothing else, so `internal/middleware` can import it without a cycle. |
| `internal/proxy` | Secure HTTP proxy. SSRF guard, transport pool, gzip/zlib decompression, `file://` scheme, TTL cache. | `scrubError()` sanitizes credentials in error strings. |
| `internal/scripts` | Opt-in script execution. | Strong sandboxing. Hot-reloaded by the config watcher via `Manager.ReplaceAll`. |
| `internal/middleware` | Recovery, Logging, RateLimit, CORS (same-origin), HostValidation (port-aware), SecurityHeaders (CSP, HSTS opt-in), TrustedProxy, Auth (the allowlist gate). | |
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
- **`style` helpers must return a complete declaration** (`width: 42.0%;`), not
  a bare value.
- **Never put a responsive property in an inline `style`.** An inline style
  beats every class and every media query, so a server-rendered
  `grid-template-columns` handed phones the desktop column count and the card
  text overflowed the card. Hand the value to the stylesheet as a custom
  property instead — `gridColumns` emits `--service-cols` and `.service-grid`
  in `web/tailwind/input.css` decides at which breakpoints it applies.
  `TestServiceGroup_ColumnsTravelAsCustomProperty` fails if this comes back.
- **Custom properties need `templ.SafeCSS`.** Templ's CSS sanitiser drops
  unknown properties, `--*` among them, so a plain `string` return silently
  loses the value. Only build such a string from data you control.
- **A grid or flex item needs `min-w-0` to be allowed to shrink.** Its default
  `min-width: auto` is why the CPU/MEM/RX/TX values overlapped each other
  instead of clipping when the card got narrow.
- **One element that cannot wrap or shrink breaks the whole page on a phone.**
  The header's right cluster was `flex-shrink-0` with no `flex-wrap`, so it set
  a floor on the document width; the page then scrolled sideways and every
  `width: 100%` element rendered narrower than the document around it — which
  looks like "the cards do not fill the screen", nowhere near the real cause.
  `make shots` in the `opensource/` workspace names the offending element.
- **Card text wraps, it does not truncate.** Name, description and the status
  line use `wrap-anywhere` and the card grows vertically; an ellipsis hides
  information the user came for. `break-words` alone is NOT enough: it breaks
  the line but leaves the element's intrinsic min-content width at the full
  token, so a spaceless name or URL still spills out of the card and scrolls
  the page. The `.wrap-anywhere` utility in `input.css` exists because
  Tailwind v3 has no `overflow-wrap: anywhere`. The numeric stats row is the
  exception — it keeps `truncate` as a last-resort guard that the container
  query above means it never reaches.
- **Card-sized things need a container query, not a media query.** The stats row
  fits four columns or two depending on the CARD width, and at 1024px a
  4-column group gives ~239px cards while a 2-column group gives ~500px ones —
  the viewport cannot tell them apart. `.service-card` is
  `container-type: inline-size` and `.service-stats` queries it. Container
  queries resolve against the **content box**, so the threshold excludes the
  card padding.
- **CSP is `script-src 'self' …`**: no inline `onclick`/`onsubmit`/`onerror`,
  **and no inline `<script>` body either**. Two cases were shipping blocked:
  the early theme script lived inline in `head.templ`, so the FOUC it prevents
  came back and every page logged a violation — it is now
  `web/static/js/theme-init.js`, a blocking `<script src>`. And an
  `hx-trigger` **filter** (`every 30s [document.visibilityState === 'visible']`)
  is compiled by htmx with `new Function(...)`, which the same CSP blocks with
  no `'unsafe-eval'`: every polled element logged an `EvalError` and lost its
  filter, so hidden tabs kept polling — the opposite of the intent. The pause
  now lives in `app.js` (`htmx:beforeRequest` + `document.hidden`), and
  `hx-trigger` stays free of `[...]` expressions.

### Tailwind CSS

- **Tailwind v3 standalone CLI only.** v4 broke `@layer base` with `@apply`,
  which `input.css` relies on. Dockerfile pins `v3.4.17`.
- If a class doesn't render, it's almost always because it lives in a path that
  isn't scanned. Either ensure the file is in `content`, or add a pattern to
  `safelist` in `tailwind.config.js` (the JS config — not any `.json`).
- Icons resolve via `homarr-labs/dashboard-icons` (jsdelivr), MDI SVG
  (`mdi-` and `mdi:`), Simple Icons (`si-` and `si:`), or absolute URL.

### Static assets & cache busting

- **The `?v=` on `/static/*` is `handlers.AssetVersion()`, a content hash of the
  BUILD OUTPUT — never `config.CurrentHash()`.** The config hash is derived from
  the user's YAML files alone, so a deploy that shipped a new `main.css` without
  a config change produced the identical URL and browsers kept the cached
  stylesheet (`max-age=86400`) against freshly rendered markup. New class names
  met an old file; the page looks structurally broken, which sends you hunting
  through CSS instead of through caching. Covered by
  `TestHashStaticAssets_ChangesWithContent`.
- **`config.CurrentHash()` still owns the `config-hash` meta tag and the
  `/api/hash` reload poll**, and must keep tracking the config and only the
  config. The two hashes answer different questions; do not merge them.
- Consequence for any markup change: a new custom class in `input.css` only
  works if the CSS is recompiled AND the browser refetches it. Both are now
  automatic; a stale view in dev is a hard reload away.

### Base path (`HOMEPAGE_BASE_PATH`)

- **Inside a handler, a path is always relative to the dashboard. The prefix is
  added only when a URL is EMITTED.** `middleware.BasePath` strips it at the
  edge and puts it on the request context, so chi's patterns,
  `http.StripPrefix("/static/")` and the auth gate's public-path allowlist keep
  matching the same paths they matched before the option existed. Never "fix" a
  path comparison by prepending the prefix to it — that reintroduces the bug
  this design avoids (`/team/static/…` not matching `/static/`, which puts the
  login page's CSS behind the login gate).
- **Every emitted URL goes through a builder in `internal/templates/urls.go`**,
  and every redirect through `handlers.redirect` / `config.PrefixPathFrom`. A
  literal `"/api/…"` or `http.Redirect(w, r, "/auth/login", …)` is a bug that
  only shows up in a prefixed deployment.
- **Two accessors, and they are not interchangeable.** `config.BasePath()` reads
  the env var and is memoised; only `handlers.API` and `cmd/myserver` may call
  it. Everything else reads `config.BasePathFrom(ctx)` — the prefix of *this*
  request. Inside a `templ` component `ctx` is in scope, which is why the URL
  builders take it instead of the components growing a parameter.
- **`next` stays dashboard-relative** and is re-rooted under the prefix when
  used, which is what confines a caller-supplied destination to this dashboard.
  `safeNext` keeps doing only what it did: reject off-site and
  protocol-relative values.
- **Cookies:** the session cookie's `Path` is the prefix. The OAuth state cookie
  cannot be scoped that way — `__Host-` requires `Path=/` — so its NAME carries
  the base path instead.
- **The prefix must not be interpolated into a `<script>`** (see §Templ). It
  reaches the browser as `<meta name="base-path">`, emitted only when non-empty
  so the unprefixed HTML stays byte-identical, and `app.js` reads it.
- Unset means the pre-feature code path, verified byte-for-byte against the
  previous binary (HTML, `/api/services`, headers, cookies, redirect targets).
  Keep `TestBasePath_AbsentLeavesTheHTMLUntouched` passing.

### customapi display modes

- **Only `display: dynamic-list` is content-negotiated to HTML.** `text`,
  `list`, `tile` and `graph` run through `widgets.ProcessDisplay` and are
  returned as JSON (`handlers/proxy.go`), and the `htmx:beforeSwap` guard in
  `app.js` refuses to swap non-HTML — so those cards render EMPTY, including
  the ones in the demo config's `Data` group. Either add the HTML branch and a
  Templ component per mode, or stop documenting them as working.

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
- **`/api/validate` re-reads from disk on purpose** (`config.ValidateFromDisk`).
  It must never be rebuilt on the cached loaders: `ReloadCache` discards their
  errors (`c.Services, _ = loadServices()`), so a cache-backed check answers
  `valid: true` for a file it failed to parse. That was the behaviour until it
  was fixed.
- **`/api/validate` is still not a strict parser.** Go's `yaml.v3` accepts
  ambiguous syntax like `key:{flow}` (missing space after `:`) and silently
  produces the wrong shape. Strict YAML parsers reject it. When debugging
  "config parses but downstream is empty", run a strict linter
  (`python -c "import yaml; …"`).

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

### Authentication (optional, `config/auth.yaml`)

- **The allowlist is the switch.** No `enabled` flag: a file with at least one
  email or domain requires login, an absent file or an empty allowlist keeps
  the dashboard public. Auth off must stay byte-for-byte identical to the
  pre-feature behaviour — no cookies, no redirects, same CSP (`form-action` is
  added only when auth is on), `/auth/*` answers 404. There is a regression
  test for this; keep it passing.
- **Never let a config failure mean "public".** `AuthConfig` lives in its own
  `atomic.Value` (`internal/config/auth.go`), NOT in `cachedConfig` — that one
  discards load errors (`c.Settings, _ = loadSettings()`), and a policy that
  silently became nil would publish the dashboard. Broken file with a previous
  good policy ⇒ keep it, degraded. Broken with nothing to fall back on, or a
  file that vanished ⇒ lockdown 503. Only a well-formed empty allowlist opens
  up. Full table in `docs/context/authentication.md`.
- **Startup may be fatal, hot-reload never is.** `initAuth` in
  `cmd/myserver/main.go` refuses to start on a bad policy; the watcher only
  logs and keeps the last known good one.
- **The gate reads the policy per request**, never captured in the middleware
  closure — that is what makes the allowlist hot-reloadable and evicts a
  removed address on their next request. Same rule as `config.CurrentHash()`.
  Do not repeat the `initScripts` pattern here.
- **Public paths are an allowlist, not a denylist** (`internal/middleware/auth.go`).
  Gating only `/` gates nothing: `/api/services` + `/api/widgets` +
  `/api/services/proxy` rebuild the dashboard from outside and `/api/scripts/*`
  runs shell. A route added later is protected by default.
- **`/auth/*` routes are registered unconditionally** and 404 while the
  allowlist is empty. Registering them conditionally (the scripts pattern)
  would lock the operator out when they enable auth by editing the YAML,
  since the gate arms live but the login page would not exist until a restart.
- **No new dependencies.** The id_token's signature is intentionally not
  verified: it arrives over direct TLS from the token endpoint, the case OIDC
  Core §3.1.3.7(6) allows. `iss`/`aud`/`exp`/`nonce`/`email_verified` are still
  validated. Accepting a token from anywhere else (One Tap, implicit, a
  generic IdP) reinstates the need for JWKS.
- `auth.yaml` has **no skeleton** in `internal/config/skeleton/`: the file must
  stay absent by default, since its content is what turns login on.
- User-facing docs: `docs/context/authentication.md` (schema, setup, flow) and
  `docs/context/troubleshooting.md` §Authentication (symptom-first).

### Security model

- **No internal auth by default.** The dashboard expects an auth layer in front
  (Cloudflare Access, Authelia, oauth2-proxy, …), or the optional built-in
  allowlist above. Do not plan local username/password login.
- `HostValidation` always seeds `localhost:PORT` / `127.0.0.1:PORT` /
  `[::1]:PORT` defaults; `HOMEPAGE_ALLOWED_HOSTS` extends the list. `*` is the
  explicit wildcard.
- CORS is same-origin and applies **only** to `/api/*` — it reflects `Origin`
  only when it equals `Host`. The HTML dashboard route has no CORS headers.
- Rate limiting: per-IP token bucket. 60/min default; 10/min on script
  execution; 1/min on `/api/hash` and `/api/reload`.
- Security headers globally: CSP (`script-src 'self' unpkg.com cdn.jsdelivr.net`),
  `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`,
  `X-Content-Type-Options: nosniff`. HSTS opt-in via `HOMEPAGE_HSTS`.
- SSRF: `proxy.Proxy` resolves DNS, blocks cloud-metadata IPs always, and
  blocks RFC1918 + loopback unless `HOMEPAGE_ALLOW_PRIVATE_HOSTS=true`
  (default `true`, since the dominant use case is self-hosted).
- Credential sanitization: `/api/widgets` and `/api/services` deep-strip keys
  matched by `IsSensitiveKey` (case-insensitive substring) and remove
  basic-auth userinfo + sensitive query params from **every** URL-bearing
  field — `widget.url`, `href` and `siteMonitor`. `Service.Labels` goes
  through `sanitizeLabels`: sensitive keys dropped, URL values scrubbed,
  non-URL values returned byte for byte (round-tripping arbitrary text
  through `url.Parse` would rewrite escapes). The HTML dashboard renders
  from `config.LoadServices()` and is not affected by this stripping.
- **`Service.Labels` is parsed and sanitized but never rendered.** No template
  prints it, so it only reaches `/api/services`. Anything meant to be visible
  on a card goes in `description` (or a `customapi` widget); do not plan a
  feature around labels showing up without adding the markup first.
- `handlers.Services` caches an **already sanitized** copy. Never
  post-process the cached slice in place: it is shared across requests, so
  editing it is a data race and any per-request view (e.g. filtering by the
  caller) would leak into other responses. Derive a copy —
  `sanitizeServiceGroups` is the pattern.

---

## Test focus

| Package | Coverage | Security tests |
|---|---|---|
| `internal/scripts` | 66.7% | 14 adversarial tests: path traversal, `.sh` enforcement, env denylist, race, timeout kill-tree, output cap, hot-reload via `ReplaceAll`. |
| `internal/middleware` | 38.9% | `HostValidation`: defaults, wildcard, port-awareness, case. |
| `internal/auth` + auth tests in `config`/`handlers` | — | Allowlist matching, forged/expired/foreign-key cookies, id_token claim validation, open-redirect `next`, the gate over every content path, scripts gated, lockdown, `trustedHeader` from an untrusted peer, broken YAML on hot-reload, and auth-off regression. |
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
  leak paths, stack traces, or upstream URLs. `/api/validate` keeps the YAML
  parse error (it is what makes the endpoint useful) but runs it through
  `scrubConfigPaths` first, and `auth.yaml` is deliberately excluded from that
  report — its errors name environment variables.
- Don't break the `internal/config → everything else` dependency direction.

---

## Deploy at a glance

- Multi-stage Docker. Runtime: `alpine:3.21` + `su-exec` + `bash` + `docker-cli`
  + `wget` + `tini` + `tzdata`. User `myserver:1000` non-root.
- Required mounts:
  - `/srv/myserver/config → /app/config` — host bind, hot-reloaded.
  - `/var/run/docker.sock` — Docker stats + script wrappers (mount `:ro` if
    scripts don't need to mutate containers).
- Key env: `HOMEPAGE_ALLOWED_HOSTS`, `HOMEPAGE_SCRIPTS_ENABLED`,
  `HOMEPAGE_ALLOW_PRIVATE_HOSTS`, `HOMEPAGE_HSTS`,
  `TRUSTED_PROXIES`, `HOMEPAGE_PROXY_DISABLE_IPV6`, `TZ`. See
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
- `karakeep` has no widget definition. The `hoarder` alias that pointed at it
  was removed; if the widget is ever added, restore the alias. Aliases are
  covered by `TestBuiltinAliasesResolve`, which fails on a target that is not
  registered.
