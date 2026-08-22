# Changelog

## Unreleased

### Added — optional email allowlist with Google sign-in

Opt-in authentication, configured by a new `config/auth.yaml`. It is an
extension, not a compatibility break: Homepage never reads that file.

- **The allowlist is the switch.** No `enabled` flag — a file listing at least
  one email or domain makes sign-in mandatory; an absent file or an empty
  allowlist leaves the dashboard exactly as public as before. With
  authentication off, responses are byte-for-byte unchanged: no cookies, no
  redirects, the same CSP, and `/auth/*` answers 404. A regression test covers
  every content path for this.
- **Failing closed is the point.** The auth policy lives in its own
  `atomic.Value` rather than in `cachedConfig`, which discards load errors — a
  policy that silently became nil would publish the dashboard. A broken edit
  keeps the last working allowlist; a broken file with nothing to fall back on,
  or one that disappears while sign-in is active, answers 503 everywhere except
  `/api/healthcheck`. Only a well-formed, empty allowlist opens the dashboard.
  A bad policy at startup is fatal; on hot-reload it never is.
- **The gate covers everything**, via an allowlist of public paths rather than
  a denylist: `/static/*`, `/auth/*` and `/api/healthcheck` are open, all else
  requires a session — including `/api/services`, `/api/widgets` and
  `/api/services/proxy`, which together rebuild the dashboard from outside, and
  `/api/scripts/*`, which runs shell. Routes added later are protected by
  default.
- **The allowlist is re-checked on every request**, not just at login, so
  removing an address evicts that person immediately instead of when their
  cookie lapses. Adding one grants access with no restart.
- **Unauthenticated requests are answered in the caller's own terms**: `302`
  for navigation, `401` + `HX-Redirect` for HTMX (so a polling widget does not
  paint the login form inside its card), `401` JSON for API clients. An address
  that signs in but is not listed gets `403` and no cookie.
- **No new dependencies.** The ID Token arrives over direct TLS from the token
  endpoint — the case OIDC Core §3.1.3.7(6) covers — so its claims (`iss`,
  `aud`, `exp`, `nonce`, `email_verified`) are validated without JWKS, a JWT
  library, or an OAuth package. Sessions are stateless: HMAC-SHA256 over
  `email | expiry | nonce`, `HttpOnly` (mandatory, since `custom.js` is
  operator JavaScript on the same page), `SameSite=Lax`, `Secure`, sliding
  renewal past half-life.
- **Listing a public mail provider under `domains:` is refused at startup**
  (`gmail.com` and friends) unless `allowPublicDomains: true` is set — it would
  admit anyone who can register an address there.
- **`provider: trustedHeader`** reads the identity a front proxy asserts
  (`Cf-Access-Authenticated-User-Email`, `Remote-Email`, `X-Forwarded-Email`)
  and puts it through the same allowlist, but only when the immediate peer is
  in `TRUSTED_PROXIES`. Deployments already behind SSO get the allowlist with
  no OAuth setup.
- `HOMEPAGE_AUTH_REQUIRED=true` refuses to start without an allowlist and
  answers 503 whenever the policy is unavailable.
- Docs: [`docs/context/authentication.md`](docs/context/authentication.md), plus
  a README section and `/auth/*` in the API reference.

### Fixed — documented environment variables that did not exist

Two variables were documented under names the code never reads. Anyone who
configured them by following the docs got silent no-ops:

- **`HOMEPAGE_ENABLE_HSTS` → `HOMEPAGE_HSTS`.** Setting the documented name did
  nothing, so deployments believed HSTS was on when the header was never sent.
- **`HOMEPAGE_TRUSTED_PROXIES` → `TRUSTED_PROXIES`.** Setting the documented
  name left the default `127.0.0.1/8,::1/128` in force, so `X-Forwarded-For`
  was ignored and per-IP rate limiting bucketed every request under the
  proxy's address. With the new `trustedHeader` auth provider it would also
  reject every request.

The code was already correct and the names are unchanged; only the docs were
wrong (README, `CLAUDE.md`, four files under `docs/context/`, and the
`add-widget` skill). **Check your deployment for the old names.**

Also fixed:

- `templates/bookmarks.yaml` in the `add-widget` skill was not valid YAML
  (`abbr: >_` — a bare `>` opens a block scalar), so copying that template
  broke the user's config.
- The docs claimed **160+ built-in widgets**. The registry holds **46 widget
  types, 50 endpoint mappings and 4 aliases** (counted by running
  `Registry.List()`); the inflated figure appears to have been inherited from
  the original Homepage project. Corrected in the README, the architecture
  diagram, the configuration reference and the skill.

### Fixed — `/api/services` credential leak and shared-slice data race

- **Basic-auth in `href` / `siteMonitor` was returned verbatim**
  (`internal/config/services.go`). `SanitizeService` scrubbed
  `widget.url` but left the other two URL-bearing fields untouched, so a
  service written as `href: https://user:pass@host` leaked its
  credentials through `/api/services`. All three fields now go through
  the same sanitizer (renamed `sanitizeWidgetURL` → `sanitizeURL`, since
  it is no longer widget-specific). The HTML dashboard is unaffected: it
  renders from `config.LoadServices()`, not from the sanitized API view.
- **Data race on the merged-services cache**
  (`internal/handlers/services.go`). The handler sanitized the service
  list *in place* on the very slice stored in `mergedServicesCache`, so
  two concurrent requests wrote to the same structs while a third was
  serializing them — confirmed by the race detector. The cache now holds
  an already-sanitized copy built by the new `sanitizeServiceGroups`,
  and `writeServices` only encodes. `nil` in / `nil` out is preserved, so
  an empty dashboard still serializes as JSON `null` exactly as before.
- Regression tests: 16 concurrent `/api/services` requests must return
  byte-identical, fully sanitized bodies (fails under `-race` on the old
  code), plus a direct check that `sanitizeServiceGroups` leaves its
  input — and therefore the credentials the widget proxy needs —
  untouched.

### Changed — deployment config carries less about one deployment

- `docker-compose.yml` no longer documents a specific hosting setup
  (PaaS recipe, tunnel topology, public hostname, migration state). The
  published host port and the host config directory stay **literal**: the
  file is a deploy descriptor, and some platforms reject shell-style
  variables inside a volume source, so parameterizing them broke the
  deploy — a config directory pointing at the wrong path fails silently
  (the entrypoint seeds the skeleton and the stock dashboard comes up as
  if nothing were wrong). The comment at the top of the file says so.
- `TZ` is passed through from the deployment environment instead of being
  hardcoded to one timezone.
- Docs, skill templates and test fixtures use `example.com` hostnames,
  `Etc/UTC`, and neutral host paths / usernames throughout.
- `docs/context/deploy.md` documents the generic Compose / PaaS flow.
- `CLAUDE.md` states the rule the docs follow: `docs/context/` describes
  the software, not any particular deployment — hostnames, ports, host
  paths and proxy topology stay as placeholders.

## v1.5.0 (2026-05-18)

Background-image feature: paint the dashboard with either a local file
under `config/` or any HTTPS / data-URI image.

### `settings.yaml: backgroundImage`

- **Field was already parsed but never reached the page**
  (`internal/templates/head.templ`): the previous implementation
  interpolated `{ data.Settings.BackgroundImage }` inside a `<style>`
  block — exactly the gotcha documented in `CLAUDE.md`, which Templ
  emits literally. The CSS reaching the browser contained the raw
  placeholder string, so no image ever loaded. The broken `<style>`
  block is gone.
- **Background now applied via a dynamic `style=""` on `<body>`**
  (`internal/templates/layout.templ`): Templ supports attribute-value
  interpolation, so the helper-driven value is emitted safely.
- **`backgroundStyle()` helper** (`internal/templates/styles.go`):
  returns the full CSS declaration (`background-image: url(...);
  background-size: cover; background-attachment: fixed;
  background-position: center; background-repeat: no-repeat;`) — or
  empty when no image is configured or the value contains unsafe
  characters (quotes, line breaks, `..`, backslashes).
- **Two input forms** (`backgroundImageURL()`):
  - Local file under `HOMEPAGE_CONFIG_DIR` — referenced by relative
    path (`wallpaper.jpg`, `wallpapers/mountains.webp`). The helper
    rewrites it to `/api/config/<path>?v=<hash>` so it travels through
    the existing whitelisted asset endpoint and gets cache-busted on
    config-hash changes.
  - Absolute `http://`, `https://` or `data:` URL — passthrough.
- **Allowed local extensions**: `.png`, `.jpg`, `.jpeg`, `.webp`,
  `.gif`, `.svg`, `.avif`, `.ico`, `.bmp`.

### `/api/config/{path}` extended whitelist

- **Image files** under the config directory are now servable, in
  addition to the legacy `custom.css` / `custom.js` literals
  (`internal/handlers/config.go`). Whitelisting is extension-based:
  if the requested file ends in one of the image extensions above,
  the handler resolves it under `ConfigDir()` (after the existing `..`
  / absolute-path / `HasPrefix` guards) and serves it with the right
  `Content-Type` and a 5-minute `Cache-Control` header.
- Subdirectories work: `config/wallpapers/dark.webp` →
  `GET /api/config/wallpapers/dark.webp`.
- Missing-file behaviour now matches type: `custom.css`/`custom.js`
  still return empty body (so the dashboard `<link>` does not 404),
  images return 404 so the browser falls back to no background.

### Content Security Policy

- **`img-src` relaxed to `'self' https: data:`**
  (`internal/middleware/security_headers.go`): required for remote
  `backgroundImage` URLs and for custom icon URLs to load at all. The
  previous CDN-allowlist (`cdn.jsdelivr.net cdn.simpleicons.org`) was
  a subset of `https:`. The dashboard sits behind an external auth
  layer in production, so the additional surface is acceptable.

### Docs & demo

- **`bootstrap-demo-config.sh`** ships a commented-out `backgroundImage`
  example showing both the local-path and HTTPS-URL forms.
- **`SKILL.md`** has a new "Background image" subsection under
  Settings, with a resolution table and the cardBlur pairing tip.
- **`templates/settings.yaml`** documents both forms inline.
- **`README.md`** mentions the feature in the dashboard-features list
  and the settings example block.

---

## v1.4.0 (2026-05-18)

Unified search bar (web search + live QuickLaunch in one input), deploy-and-dev
ergonomics for Podman / SELinux hosts, a comprehensive demo-config bootstrap,
and a documentation pass across the agent skill and root docs.

### Unified search bar

- **One input does both jobs**
  (`internal/templates/info_widgets.templ`, `header.templ`,
  `web/static/js/app.js`): the top-right `search:` widget now renders both
  the web-search form AND a live filter dropdown that searches services
  and bookmarks as the user types. Replaces the previous separate
  centred-input `QuickLaunch` component.
- **Keyboard UX**: `↓` / `↑` navigate the suggestion list, `Enter`
  follows the highlighted suggestion (or web-searches if none is
  highlighted), `Esc` closes and blurs. A "Search the web for …" row is
  always pinned to the bottom of the dropdown so `Enter` is never a dead
  end.
- **CSP-safe DOM construction**: suggestion rows are built with
  `createElement` + `textContent` — no `innerHTML` interpolation of user
  input, no relaxation of `script-src 'self'`.
- **New `showSearchBar()` helper** (`internal/templates/layout.go`): the
  bar renders when either `widgets.yaml: search:` or
  `settings.yaml: quicklaunch:` is configured. Old configs keep working
  without edits — `quicklaunch:` is now legacy and its sub-fields are
  no-ops.
- **`QuickLaunch` templ component is now an empty no-op** kept only so
  any out-of-tree caller still compiles; the centred secondary input is
  gone.

### Deploy & dev container

- **Pin `templ` in the Dockerfile** (`Dockerfile`): added
  `ARG TEMPL_VERSION=v0.3.1001` and installed the generator at that
  version instead of `@latest`. Newer templ generators emit calls
  (`templ.ResolveAttributeValue`, …) that don't exist in the runtime
  pinned in `go.mod`, breaking the build. Bump this in lockstep with
  `go.mod`.
- **`docker-compose.dev.yml` works on rootless Podman / SELinux hosts**:
  - `userns_mode: keep-id` so the container's `myserver` (uid 1000) maps
    to the host user that started Podman; without this, the entrypoint's
    `chown myserver:myserver /app/config` leaves the bind-mounted
    directory unwritable to the host user.
  - `:Z` on the `./config:/app/config` bind mount so Podman re-labels it
    with a private SELinux context on Fedora / RHEL / CentOS. No-op on
    hosts without SELinux.
  - Docker / Podman socket mount commented out by default with both
    canonical paths documented (`/var/run/docker.sock` and
    `/run/user/1000/podman/podman.sock`).
  - Default port changed from 3000 to **8085** to avoid clashing with
    common dev stacks; `HOMEPAGE_ALLOWED_HOSTS` pre-lists both 8085 and
    3000 variants.

### `bootstrap-demo-config.sh`

- **New top-level script** that regenerates `config/` with a comprehensive
  demo dashboard exercising every documented feature: 9 service groups
  / 48 cards, every widget category in the registry, all five
  `customapi` display modes (`text`, `list`, `dynamic-list`, `graph`,
  `tile`) backed by `file://` JSON sources under `config/data/`, five
  demo scripts under `config/scripts/`, monitoring badges (ping +
  siteMonitor), `custom.css` and `custom.js`. Idempotent — it wipes
  `config/`'s contents but keeps the directory so its SELinux label
  survives.

### Agent skill restructure

- **`.agents/skills/add-widget/templates/` is now YAML-only** (1:1 with
  `config/*.yaml`). The two narrative deep-dives moved to a new
  **`.agents/skills/add-widget/guides/`** directory: `customapi.md` and
  `file-scheme.md`. `git mv` preserved history.
- **`SKILL.md` reshaped as a true skill index**: decision tree → minimal
  copy-paste snippets → pointers to `COOKBOOK.md`, `templates/`,
  `guides/`. Trimmed from 713 to ~570 lines without losing scope.
- **Caveats captured from a fresh end-to-end deploy** (in `SKILL.md`,
  `COOKBOOK.md`, `CLAUDE.md`):
  - **`yaml.v3` lenient parser** silently accepts `key:{flow}` (no space
    after `:`) and produces the wrong shape; `/api/validate` still
    reports `valid:true`. Always put a space after `:`, including in
    flow mappings.
  - **`settings.scripts.scriptDirs`** (and `maxTimeout`, `defaultTimeout`,
    `maxConcurrent`) is read once at process start; the watcher
    hot-reloads script entries but not the manager itself. Changes
    require a container restart.
  - **`layout.<group>.tab`** is parsed but unused — `TabNavigation`
    template is never invoked by `index.templ`. Set it for forward
    compatibility, but expect flat `<h2>` sections today.

### Documentation pass

- **`CLAUDE.md`** rewritten for density: package-map table, gotchas
  grouped by subsystem (Templ / Tailwind / Config / Scripts / Security),
  "Open work" lists only real pending items (audit log, `tab` wiring,
  `scriptDirs` rebuild on reload, templ-version drift).
- **`README.md`** trimmed (~165 lines): consolidated tables, removed the
  completed "Known Issues" block, prose density up, scripts section
  cleaner.
- **`COOKBOOK.md`** rewritten: troubleshooting playbook with
  status-code-to-cause table, recipe 10 rewritten for the unified
  search bar (no more inaccurate `Ctrl+K` claim), new entries for
  rootless-Podman ownership and SELinux relabel.

---

## v1.3.0 (2026-05-18)

User-experience improvements: zero-code customization, CSP-safe UI,
self-contained config directory, and local data source support.

### Zero-code customization — everything lives in `config/`

- **`config/` as a bind mount** (`docker-compose.yml`): changed from an
  opaque Docker named volume (`myserver-config`) to a host bind mount
  (`/srv/myserver/config:/app/config`). Users edit YAMLs directly on the
  host with any editor; `fsnotify` inside the container detects changes
  and hot-reloads the dashboard without restart.
- **`file://` scheme support** (`internal/proxy/proxy.go`): widgets can
  read local JSON files directly from the config directory without an
  HTTP round-trip. Relative paths resolve against `HOMEPAGE_CONFIG_DIR`;
  absolute paths work too. Enables fully self-contained dashboards that
  do not depend on external endpoints for static data.
- **`config/data/` directory**: new convention for local JSON data sources.
  Example: `config/data/demos.json` feeds a `customapi` dynamic-list
  widget via `url: file://data/demos.json`.
- **Scripts moved to `config/scripts/`**: executable `.sh` files now live
  inside the config directory alongside their metadata (`scripts.yaml`).
  A single bind mount carries both configuration and scripts.
- **Eliminated `demos.go` handler**: removed the hard-coded `/api/demos`
  endpoint and its route registration. The "Demos Production" card now
  reads `file://data/demos.json` — proving that users can add arbitrary
  data cards without writing Go code.
- **`docker-compose.dev.yml` simplified**: single bind mount
  `./config:/app/config` for local development. `config/` is in
  `.gitignore` so every developer keeps their own local setup.

### CSP-safe UI — no inline event handlers

- **Removed inline `onclick` / `onsubmit` / `oninput` / `onkeydown`**
  (`internal/templates/info_widgets.templ`): the theme-toggle button,
  search form, and quicklaunch input no longer carry inline event
  attributes that are blocked by the CSP (`script-src 'self'`).
- **Event attachment via `addEventListener`** (`web/static/js/app.js`):
  `setupEventListeners()` attaches click, submit, input, and keydown
  listeners after `DOMContentLoaded` using element IDs (`#theme-toggle`,
  `#search-form`, `#quicklaunch-input`).
- **Removed inline `onerror`** (`internal/templates/bookmark.templ`,
  `service_card.templ`, `script_card.templ`): replaced with CSS classes
  (`.bookmark-icon`, `.service-icon`, `.script-icon`) and a single
  delegated `error` listener in `app.js` that hides broken images.
- **Fixed dark/light mode toggle**: previously broken because the CSP
  blocked `onclick="toggleTheme()"`. Now works via the JS listener.
- **Fixed search widget**: previously broken because the CSP blocked
  `onsubmit="handleSearch(...)"`. Now submits via the JS listener and
  opens Google / direct URLs correctly.

### Bookmark icons — automatic defaults

- **`si-` prefix support** (`internal/templates/icons.go`): `iconURL()` now
  resolves `si-github` to `https://cdn.simpleicons.org/github`, just like
  the existing `si:` colon syntax.
- **`defaultBookmarkIcon()`** (`internal/templates/icons.go`): returns a
  default Simple-Icons icon for common services (GitHub, GitLab,
  Stack Overflow, AWS, Cloudflare, Vercel, Postman, Ngrok, LocalStack,
  etc.) when the user has not set an explicit `icon` in `bookmarks.yaml`.
- **`resolveBookmarkIcon()`** (`internal/templates/icons.go`): helper that
  prefers the explicit icon, then falls back to the default by name.

### SSRF & proxy fixes

- **Loopback allowed with `HOMEPAGE_ALLOW_PRIVATE_HOSTS`**
  (`internal/proxy/proxy.go`): `isBlockedIP()` now treats loopback
  (`127.0.0.1`, `::1`) the same as private RFC1918 addresses — allowed
  when the env var is `true` (default). This unblocks widgets that point
  to the same server (e.g. `file://` or `http://localhost:3001/...`).
- **`parseProxyResult()` extracted** (`internal/proxy/handlers/generic.go`):
  deduplicated JSON/string parsing logic shared by the HTTP and `file://`
  paths.

### Makefile & workflow

- **New targets**: `make tidy` (`go mod tidy`), `make up` / `make down` /
  `make logs` (docker compose wrappers).

### Documentation

- **`README.md` overhaul**: added "Zero-code customization" feature table,
  updated deploy instructions to use host bind mount, documented
  `file://` scheme, `HOMEPAGE_HSTS`, `HOMEPAGE_TRUSTED_PROXIES`,
  rate-limit columns on API tables, and clarified that scripts.yaml
  hot-reload is now wired.

---

## v1.2.0 (2026-05-17)

Complete 52-item improvement checklist across 6 phases. All existing tests pass,
zero race conditions, zero lint violations.

### Phase 1 — Critical Fixes (5 items)

- **Real `customapi` widget** (`internal/widgets/customapi.go`,
  `internal/proxy/handlers/generic.go`): replaced 11-line stub with full
  implementation — `GetValue` field-path traversal, `FormatValue` with number/
  date/bytes formats, display-mode dispatch (`text`, `dynamic-list`, `graph`,
  `list`, `tile`).
- **Wire `MergeServices` + `DockerDiscoverer`** (`internal/handlers/services.go`,
  `cmd/myserver/main.go`): the `discovery` package is now imported and used;
  discovered containers are merged with config services, cached, and served
  atomically.
- **Real `ConfigFile` handler** (`internal/handlers/config.go`): whitelist-based
  file serving with `..` and absolute-path rejection, MIME type detection,
  `text/plain` fallback.
- **Remove stub handlers** (`internal/handlers/api.go`): `KubernetesStats`,
  `KubernetesStatus`, `Releases`, `SearchSuggestion` removed from the router
  until real implementations exist.
- **Real Proxmox stats handler** (`internal/handlers/proxmox.go`): fetches live
  data via Proxmox API using `cfg.URL`, `cfg.Token`, and `cfg.Secret`.

### Phase 2 — Security Hardening (7 items)

- **Rate limiting** (`internal/middleware/ratelimit.go`,
  `internal/handlers/api.go`): token-bucket per-IP limits — proxy 10/s,
  scripts 1/s, ping/siteMonitor 5/s, hash 1/s. Uses `golang.org/x/time/rate`.
- **Content Security Policy** (`internal/middleware/security_headers.go`):
  `default-src 'self'; script-src 'self' unpkg.com cdn.jsdelivr.net; ...`
- **Security headers middleware** (`internal/middleware/security_headers.go`):
  `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`,
  `X-Content-Type-Options: nosniff` applied globally.
- **HSTS opt-in** (`internal/middleware/security_headers.go`): enabled via
  `HOMEPAGE_HSTS=true`.
- **Validate SiteMonitor / Ping URLs**
  (`internal/handlers/monitor.go`, `internal/handlers/ping.go`): reject
  arbitrary `?url=` / `?host=` unless they match a configured service.
- **Script file permission checks** (`internal/scripts/executor.go`): reject
  scripts with world-writable bits (`0o002`).
- **Trusted proxy list** (`internal/middleware/trusted_proxy.go`): configurable
  trusted proxies for `X-Forwarded-For` parsing, configurable via
  `HOMEPAGE_TRUSTED_PROXIES`.

### Phase 3 — Performance (10 items)

- **In-memory config cache** (`internal/config/cache.go`): `atomic.Value`
  wrappers for `LoadServices`, `LoadWidgets`, `LoadBookmarks`, `LoadSettings`,
  `LoadDocker`, `LoadProxmox`. Public/private split (`loadXxx` / `LoadXxx`).
- **Cached `ConfigHash`** (`internal/config/config.go`): caches the computed
  hash; invalidated on watcher reload.
- **Proxy response cache** (`internal/handlers/proxy.go`,
  `internal/proxy/cache.go`): SHA-256 keyed cache with 60s TTL,
  `InvalidateProxyCache()` called on config reload.
- **Gzip compression** (`internal/handlers/api.go`): `chi/middleware.Compress`
  for `text/html`, `text/css`, `application/javascript`, `application/json`.
- **DNS SSRF cache** (`internal/proxy/proxy.go`): 5-minute TTL map for hostname
  lookup results, reducing resolver round-trips.
- **Docker client pooling** (`internal/handlers/docker.go`): pooled
  `*client.Client` keyed by server name, invalidated on config change.
- **Preconnect / preload hints** (`internal/templates/head.templ`): added
  `<link rel="preconnect">` and `<link rel="preload">` for critical assets.
- **Hash polling pause on tab hidden** (`web/static/js/app.js`): pauses the
  `setInterval` when `document.hidden` is `true` to reduce idle load.
- **Scripts.yaml hot-reload** (`cmd/myserver/main.go`, `internal/scripts/`):
  config watcher triggers `registerScripts()` which atomically replaces the
  registry via `Manager.ReplaceAll`.
- **Declarative `builtinWidgets` slice**
  (`internal/widgets/registry.go`): 52-widget `[]WidgetDef` table replaces
  individual `RegisterXxx` calls. Aliases registered separately.

### Phase 4 — Code Quality (15 items)

- **Renamed `ApiKey` → `APIKey`** (`internal/config/services.go`): consistent
  Go naming convention.
- **Moved `ClientIPFromRequest` / `IsOriginAllowed` to `middleware`**
  (`internal/middleware/`): extracted from `handlers/`.
- **Removed `CachedRequest` dead code** (`internal/handlers/`): deleted unused
  wrapper.
- **Removed `package.json` / `package-lock.json`**: project uses Tailwind
  standalone CLI only.
- **Unified response helpers** (`internal/handlers/helpers.go`): `respondJSON`,
  `respondHTML`, `respond` with configurable status.
- **Proxy handler tests** (`internal/handlers/generic_test.go`): test coverage
  for `GenericProxyHandler` with `httptest.Server` + `SetTestSkipSSRF`.
- **Adversarial env tests** (`internal/scripts/executor_test.go`): tests for
  env denylist, path traversal, `.sh` execution.
- **Simplified `isBlockedIP`** (`internal/proxy/proxy.go`): cleaner private-IP
  checks with `AllowPrivateHosts` toggle.
- **Pre-allocated `baseEnv`** (`internal/scripts/executor.go`): extracted as
  package-level variable.
- **Sorted `Registry.List()`** (`internal/widgets/registry.go`): deterministic
  output.
- **Removed `JSONString`** (`internal/templates/helpers.go`): unused helper.
- **Hardcoded `_blank`** in templates: removed `linkTarget` helper, used
  `target="_blank"` directly.
- **Fixed `docker-entrypoint.sh`** (`docker-entrypoint.sh`): last line uses
  `exec su-exec myserver /app/myserver "$@"` for proper PID-1 signal
  forwarding.

### Phase 5 — Architecture & Refactoring (7 items)

- **`GenericProxyHandler` queries widget registry**
  (`internal/proxy/handlers/generic.go`, `internal/widgets/registry.go`):
  `resolveWidgetAPI` looks up `APITemplate()` and `Mappings()` from
  `widgets.DefaultRegistry` instead of hardcoding URLs per type.
- **Split `templates/helpers.go`** into 4 files:
  `urls.go` (URL builders), `icons.go` (icon resolution), `styles.go`
  (CSS helpers), `layout.go` (layout lookups).
- **Move formatting utilities** from `i18n.go` to `format.go`:
  `FormatBytes`, `FormatDuration`, `FormatLatency`, `FormatPercent`,
  `FormatStatusCode`, `FormatTemp`.
- **Refactor `cmd/myserver/main.go`**: extracted `initLogger`, `initConfig`,
  `initDocker`, `initScripts`, `startWatcher`, `startServer`,
  `waitForShutdown` — `main()` reduced from 174 lines to 12.
- **Extract middleware setup from `api.go`**: `setupMiddleware` and `setupRoutes`
  separate concerns.
- **`Doer` interface in `proxy.Proxy`** (`internal/proxy/proxy.go`): tests can
  inject mock HTTP clients via `Params.Doer`.
- **`withDockerClient` helper** (`internal/handlers/docker.go`): eliminates
  duplicated `getDockerClient` + error handling in `DockerStats` and
  `DockerStatus`.

### Phase 6 — Workflow & Tooling (8 items)

- **GitHub Actions CI** (`.github/workflows/ci.yml`): runs `go test -race`,
  `gofmt`, `go vet`, `make build` on push and PR.
- **`make tidy`** (`Makefile`): runs `go mod tidy`.
- **Docker Compose targets** (`Makefile`): `make up`, `make down`, `make logs`.
- **Pre-commit hook** (`.githooks/pre-commit`): blocks commits with unformatted
  code or `go vet` errors.
- **Benchmark tests** (`internal/config/config_bench_test.go`,
  `internal/proxy/proxy_bench_test.go`): benchmarks for `ConfigHash`,
  `LoadServices`, `SanitizeURL`, `Proxy`, cached SSRF checks.
- **Dev docker-compose** (`docker-compose.dev.yml`): mounts local
  `config/`, `scripts/`, and Docker socket for local development.
- **Improved `.gitignore`**: added IDE/OS artifacts (`.idea/`, `.vscode/`,
  `.DS_Store`, `*.swp`, etc.).
- **GitHub release workflow** (`.github/workflows/release.yml`): builds
  `linux/amd64` and `linux/arm64` binaries on tag push, creates GitHub
  release with auto-generated notes.

---

## v1.1.0 (2026-04-13)

### Fixes

- **Docker socket access at startup** (`docker-entrypoint.sh`): the
  entrypoint detected the socket GID and added the `myserver` user
  to the corresponding group, but then ran `su-exec myserver:myserver`.
  The `user:group` form forces uid+gid and **discards supplementary
  groups**, so the server ran without the `docker-sock` group and all
  `/api/docker/*` endpoints returned 502. Changed to `su-exec myserver`
  (full user switch with supplementary groups).

- **"running" frozen on non-existent containers**
  (`internal/handlers/docker.go`): `DockerStatus` and `DockerStats`
  returned 404 when the container didn't exist, and HTMX by default
  doesn't swap on non-2xx responses, so the last rendered `running`
  value was stuck forever. Now they return 200 with `status: "notfound"`
  (JSON) or equivalent HTML (HTMX), and the template adds a `notfound`
  case → red dot + label "not found".

- **CPU stats always 0.00%** (`internal/handlers/docker.go`): the
  anonymous struct parsing the Docker API response used
  `json:"system_usage"` when the engine reports `system_cpu_usage`.
  It never decoded, so `systemDelta = 0` and the guard
  `if systemDelta > 0` always failed. Tag fixed; now the percentage
  reflects the actual container load.

- **Memory stats "frozen"** (`internal/handlers/docker.go`): the
  handler returned `memory_stats.usage` directly, which includes
  page-cache (mostly static). Now matches `docker stats`: subtracts
  `inactive_file` (cgroup v2) or `total_inactive_file` (cgroup v1)
  to return the actual working set.

### Features

- **Stats for all cards** (`internal/templates/service_card.templ`):
  dropped the `svc.ShowStats` gate — any card with
  `container` + `server` now shows the CPU/MEM/RX/TX row. Before
  only QAirweave had it because you had to explicitly opt-in with
  `showStats: true` in `services.yaml`.

- **customapi widget — `display: dynamic-list` server-side rendered**
  (`internal/handlers/proxy.go`, `internal/templates/widget.templ`,
  `internal/config/services.go`): the proxy does content-negotiation.
  If the request comes from HTMX and the widget has `display: dynamic-list`,
  it returns rendered HTML via `templates.DynamicListHTML` using the
  `mappings` (items/name/label/target) to extract rows. Clients
  without `HX-Request` continue receiving JSON. Unblocks the "Demos
  Production" card which was stuck at "Loading..." because the
  `htmx:beforeSwap` guard rejects non-HTML responses. The links of
  each item use `pointer-events-auto` to avoid falling into the
  whole-card anchor.

- **Diagnostic script** (`scripts/test-docker-endpoints.sh`):
  parses `services.yaml` from the container volume and hits
  `/api/docker/status` and `/api/docker/stats` for each declared
  service. Includes a regression check that requests a made-up
  container and expects `200` + `status: notfound`. Exits with `0`
  if all ok, `1` if any card fails, `2` if there's an environment
  issue.

### Files touched

- `docker-entrypoint.sh` — su-exec fix
- `internal/handlers/docker.go` — JSON tag, memory without cache,
  notfound as 200
- `internal/handlers/proxy.go` — content-negotiation for dynamic-list,
  `extractDynamicListItems` + `resolveTemplate` + `stringifyJSON`
- `internal/config/services.go` — new field `WidgetConfig.Display`
- `internal/templates/service_card.templ` — remove `ShowStats` gate
- `internal/templates/widget.templ` — new `DynamicListHTML` and
  `notfound` case in `DockerStatusHTML`
- `internal/templates/types.go` — `DynamicListItem` type
- `scripts/test-docker-endpoints.sh` — new
- `../deploy_config.sh` — rename `TriliumNext Moni` → `Notas`
  (description `TriliumNext`, url `notes.example.com`), drop the
  `TriliumNext` card without container

## v1.0.0 (2026-04-13)

Initial release — port of Homepage (Next.js/React) to Go + Templ +
HTMX + Tailwind CSS.

### Features

- 100% compatible with the original Homepage YAML files (`services.yaml`,
  `widgets.yaml`, `bookmarks.yaml`, `settings.yaml`, `docker.yaml`,
  `kubernetes.yaml`, `proxmox.yaml`).
- Runtime ~50-80 MiB RAM, reproducible multi-stage build (no npm,
  no `node_modules`).
- Server-side rendered dashboard with [Templ](https://templ.guide),
  interactivity with [HTMX](https://htmx.org) and styles with Tailwind
  CSS v3 standalone.
- Configuration hot-reload via `fsnotify` (`/app/config`).
- Widgets: `customapi`, `docker` stats/status, `glances`, `resources`,
  `speedtest`, `photoprism`, `vikunja`, plus 150+ stubs from the
  Homepage registry.
- Info widgets: `datetime`, `greeting`, `search`, `weather`,
  `openmeteo`, `stocks`, `kubernetes`, `longhorn`.
- i18n EN/ES via `internal/templates/i18n.go`.
- Opt-in scripts feature (`HOMEPAGE_SCRIPTS_ENABLED=true`): `.sh`
  execution with timeouts, concurrency limits, aggressive env
  scrubbing, denylist (`LD_PRELOAD`, `BASH_ENV`, ...), server-enforced
  `requireConfirm`, path-traversal-safe (`EvalSymlinks` + prefix check).
- HTTP proxy with SSRF guard (blocks cloud-metadata + RFC1918 by
  default), shared transport pool, `scrubError()`.
- Middlewares: port-aware `HostValidation`, same-origin `CORS` only on
  `/api/*`, `Recovery`, `Logging` with zap.
- Recursive credential sanitization on `/api/widgets` and `/api/services`
  (`IsSensitiveKey` case-insensitive substring in maps and slices,
  strips basic-auth from URLs).
- Deploy via any Compose-capable PaaS (git-connected, auto-deploy on push).
  Runtime `alpine:3.21` with `su-exec`, `bash`, `docker-cli`, `wget`,
  `tini`, `tzdata`. User `myserver:1000` non-root.

### Env vars

- `HOMEPAGE_ALLOWED_HOSTS=dashboard.example.com,localhost:3000`
- `HOMEPAGE_SCRIPTS_ENABLED=true` (opt-in)
- `HOMEPAGE_ALLOW_PRIVATE_HOSTS=true` (default true for self-hosted)
- `TZ=Etc/UTC`