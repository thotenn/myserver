# sk-clients — Running several dashboards from one MyServer

Expert skill for the **dashboards themselves**: how many there are, who each one
is for, which config directory feeds it, which URL it answers on, and who is
allowed in. Creating a new dashboard, changing an existing one — the root one
included — and retiring one.

> **Companion skills.** `add-widget/` is about the *content* of one dashboard
> (cards, widgets, bookmarks, icons) — go there for YAML syntax. `sk-ui/` is
> about the dashboard's own Go/Templ/Tailwind code. This skill sits above both:
> it decides what a dashboard IS, and it is the only one that touches URL
> prefixes, per-dashboard auth and dashboard isolation.
>
> **Companion files**
> - `guides/deploy.md` — the instance, the reverse proxy, the healthcheck.
> - `guides/auth.md` — per-dashboard login, and the traps that only appear
>   when two dashboards share a hostname.
> - `templates/dashboard/` — a complete dashboard, ready to copy.
> - `scripts/new-dashboard.sh` — scaffolds one (`make dashboard SLUG=acme`).

---

## The model, in one table

**A dashboard is a config directory.** Nothing more: no database, no admin UI,
no per-client container. Create the directory and the dashboard exists.

| | Comes from | Notes |
|---|---|---|
| Content | its own config directory | The root dashboard reads `HOMEPAGE_CONFIG_DIR`; a client reads `<that>/dashboards/<slug>/`. Never share one between two dashboards. |
| URL | the directory name | The root dashboard is at `/`, a client at `/<slug>`. `HOMEPAGE_BASE_PATH` moves **all** of them under a prefix: `/team` and `/team/acme`. |
| Who gets in | `<its dir>/auth.yaml` | Absent ⇒ public. At least one address ⇒ login required. Per dashboard. |
| Features | process env (`HOMEPAGE_SCRIPTS_ENABLED`, …) | Process-wide, and therefore **root-only**: a client dashboard never gets them, whatever the env says. |

**One process serves every dashboard.** It resolves the first path segment
against the directories under `dashboards/` and serves that one — so a new client
needs no container, no reverse-proxy rule and no new redirect URI at Google.

```
                                        ┌── /        ─► config/
reverse proxy ──► one myserver ─────────┼── /acme/*  ─► config/dashboards/acme/
   (one hostname)                       └── /globex/*─► config/dashboards/globex/
```

Isolation is therefore a property of the code, not of a process boundary, and it
is held up by three things: a handler can only read the dashboard the URL
resolved to (the global config accessors were deleted — see `CLAUDE.md`
§Dashboards), a client dashboard's router does not even register the dangerous
routes, and every cache is keyed per dashboard. `internal/handlers/tenants_test.go`
is what pins it.

### Two roles, and they are not symmetrical

| | Root dashboard | Client dashboard |
|---|---|---|
| Audience | you, the operator | one client, plus you |
| URL | usually the host root, no base path | `/<slug>` |
| Written by | you | you — the client never creates anything |
| Auth | your call | an allowlist with you and the client on it |
| Scripts, Docker, widget credentials | fine | **never** — see [What a client dashboard must not have](#what-a-client-dashboard-must-not-have) |
| Content | anything | links, reminders, read-only status |

---

## Layout convention

Client dashboards live under the root config directory, one per slug:

```
config/                        the root dashboard
  settings.yaml
  services.yaml
  …
  dashboards/
    acme/                      served at /acme
      settings.yaml
      services.yaml
      widgets.yaml
      bookmarks.yaml
      auth.yaml                who may see this dashboard (absent = public)
      data/                    JSON read by file:// widgets
      custom.css               optional, per dashboard
```

`dashboards/` is not a filing convention any more — it is the registry. A
directory there IS a dashboard, and its name IS the URL. Three names are
reserved (`api`, `auth`, `static`), because they would shadow the root
dashboard's own routes; the charset is `[A-Za-z0-9._~-]`, one segment, no
slashes. A directory that breaks either rule is logged and skipped, never
served.

A single bind mount therefore carries every dashboard, and a new client is a
directory.

---

## Create a client dashboard

The slug is the URL. Keep it to `[a-z0-9._~-]`, no slashes unless you really
want `/clients/acme` (nested prefixes work; each extra level is one more thing
the proxy has to match).

1. **Scaffold it.**

   ```bash
   make dashboard SLUG=acme          # or: bash .agents/skills/sk-clients/scripts/new-dashboard.sh acme
   ```

   Copies `templates/dashboard/` to `config/dashboards/acme/` and refuses to
   touch an existing directory.

2. **Write the content.** `settings.yaml` (title, colour, language, layout),
   `services.yaml` (the links), `widgets.yaml` (top bar). Card and widget syntax
   is `add-widget/`'s job; the rules specific to a client dashboard are
   [below](#content-rules-for-a-client-dashboard).

3. **Decide who gets in.** The scaffold leaves `auth.yaml.example`, which the
   loader ignores — so a freshly scaffolded dashboard is **public**, and the
   process says so at startup with a warning. To require login: fill it in,
   export its `{{HOMEPAGE_VAR_*}}` variables, then rename it to `auth.yaml`.
   Point `google.redirectURL` at the **root dashboard's** callback and there is
   nothing to add in the Google console — the callback is shared, and the
   dashboard the login belongs to travels inside the signed OAuth state. Read
   `guides/auth.md` before deviating from that.

4. **That is it — there is no step 4.** The running process notices the new
   directory and starts serving `/acme` without a restart. No container, no
   proxy rule, no new redirect URI.

5. **Verify** with the [checklist](#verify-after-every-change).

## Change an existing dashboard

Editing content is just editing YAML in that dashboard's directory — the process
picks it up without a restart, and **only that dashboard reloads**: its own
config hash changes, so nobody else's browser is told to refresh. What does
**not** hot-reload:

| Change | Needs |
|---|---|
| Any `.yaml`, `custom.css`, `custom.js` at the top of a dashboard's directory | nothing, saved is applied |
| A whole new dashboard (`dashboards/<slug>/`) | nothing — the directory is noticed and served |
| Removing a dashboard's directory | nothing — it stops being served |
| `data/*.json` (file:// sources) | nothing, but up to ~60s: the watcher ignores subdirectories and the `.json` extension, and the proxy caches a response for 60s |
| `auth.yaml` | nothing — the policy is read per request, so removing an address evicts that person on their next click |
| `settings.scripts.scriptDirs`, `maxTimeout`, `defaultTimeout`, `maxConcurrent` | a restart (read once at startup, root only) |
| `HOMEPAGE_BASE_PATH`, any env var | a restart |

**Moving a client dashboard to a different URL** is renaming its directory. Old
links break; there is no redirect built in. Nothing else changes — not the proxy,
not the Google console.

**Moving every dashboard under a prefix** is `HOMEPAGE_BASE_PATH`, and it
deserves its own warning: with `HOMEPAGE_BASE_PATH=/home`, the host root stops
answering (404) and every client moves to `/home/<slug>`, so the proxy needs a
redirect `/` → `/home` if the bare hostname should keep working, and any
`google.redirectURL` has to be updated in the Google console. Leaving it unset
costs nothing and is the tested path.

## Retire a client dashboard

Delete `config/dashboards/<slug>/`. The process stops serving that prefix and
logs the removal; every other dashboard keeps its session and its cache.

Deleting only the `auth.yaml` from a gated dashboard is NOT the way to retire
one: a vanished policy is indistinguishable from a failed mount, so that subtree
locks down with 503 rather than reverting to public. Correct fail-closed
behaviour, alarming way to find out.

Removing the client's address from `auth.yaml` and leaving the dashboard up is
the softer version: they lose access on their next request.

---

## What a client dashboard must not have

Each of these is a concrete way for one client to reach something that is not
theirs. They are all avoided by configuration, not by code — which is why they
are listed here rather than trusted to a review.

Most of these the code now refuses on its own — the routes a client dashboard
would need for them are not registered at all. They stay on the list because
writing the YAML anyway produces a card that silently does nothing, and because
the reasoning is what should stop you, not the 404.

| Never | Why |
|---|---|
| A `widget:` carrying `key` / `token` / `password` / `username` | The widget proxy fetches the upstream **with those credentials** and returns the body to whoever loaded the card. `/api/services/proxy` does not exist for a client dashboard, so the card renders empty — but a credential of yours has no business sitting in a client's directory in the first place. |
| `docker.yaml`, `showStats`, or a `type: docker` service | Container names and stats describe the host. Discovery is merged into the root dashboard only, and `/api/docker/*` is not registered for a client. |
| An info widget that reports the host (`resources`) | `/api/widgets/resources` is not registered for a client; the card renders empty. |
| A `script:` service | `/api/scripts/*` is root-only and gated on `HOMEPAGE_SCRIPTS_ENABLED`, which a client dashboard ignores by construction. |
| The same config directory as another dashboard | Two dashboards, one `services.yaml`: everyone sees everything. |
| A real password, API key or token in any field | Nothing in a client dashboard needs a secret. Point at where the secret lives instead. |
| A custom `session.cookieName` shared with another dashboard | The default already carries the slug (`myserver_session_acme`); overriding it with a name another dashboard uses re-creates the ambiguity it exists to avoid. See `guides/auth.md`. |

What IS safe on a client dashboard: links (`href`), text (`description`), and
the status indicators `ping:` and `siteMonitor:`. Those two resolve the service
inside that dashboard's own config and hit the service's own URL — no widget
credential is involved, which is exactly what separates them from
`/api/services/proxy`.

---

## Content rules for a client dashboard

Beyond `add-widget/`'s syntax, four things behave differently from what the
YAML suggests. All four have cost someone an afternoon.

### Reminders go in `description`, not in `labels`

`labels:` is parsed and reaches `/api/services`, but **no template renders it** —
it never appears on a card. Anything the client has to read goes in
`description`, or in a widget:

```yaml
- Applications:
    - Orders panel:
        href: https://orders.example.com
        description: user acme.admin · password in the team vault
        icon: mdi:cart-outline
        siteMonitor: https://orders.example.com
```

### The secret never enters MyServer

The card says **who** the user is and **where** the password is kept. That way a
leak of the whole config directory leaks a filing system, not access.

### Only `display: dynamic-list` renders

A `customapi` widget with `display: text`, `list`, `tile` or `graph` returns JSON,
and the frontend refuses to swap a non-HTML response into a card: it renders
**empty**. Use `dynamic-list` — a name plus a right-aligned badge covers most of
what the other modes were for:

```yaml
- Access:
    description: user, and where each password is kept
    icon: mdi:key-outline
    widget:
      type: customapi
      url: file://data/access.json          # relative to THIS dashboard's dir
      display: dynamic-list
      mappings:
        items: access
        name:  app
        label: where
```

### `data/*.json` is readable by the server, not by the browser

`/api/config/{path}` only serves `custom.css`, `custom.js` and image files, so a
JSON file under `data/` cannot be fetched directly — it reaches the page only
through a widget, already shaped by `mappings`. Useful: notes you would rather
not publish verbatim can live there.

### The theme is a starting value, not a per-dashboard setting

`settings.yaml: theme` and `color` decide the first render. The visitor's stored
choice wins after that, and `localStorage` belongs to the **hostname**, not to
the path — so a visitor who toggles dark mode on one dashboard toggles it on
every dashboard of that host. Use `color:` to tell dashboards apart and expect
the light/dark choice to be shared.

---

## Isolation — what holds, and what you have to hold up

**Holds by itself.** One process serves them all, so this is now a code
property rather than a process boundary — and it is tested as one
(`internal/handlers/tenants_test.go`):

- A handler can only read the dashboard the URL resolved to. The package-level
  config accessors were **deleted**, so reading "the" config does not compile.
- A client dashboard's router does not register the widget proxy, the scripts,
  the docker/proxmox endpoints, the info-widget data endpoints, `/api/reload` or
  `/api/validate`. They are not forbidden for a client — they do not exist.
- Every cache is keyed per dashboard, `/api/services` included, so one
  dashboard's response can never be served to another.
- `ping` and `siteMonitor` resolve against the requesting dashboard's own
  services and refuse anything it does not list.
- `/api/config/{path}` reads that dashboard's own directory, traversal guarded.
- The session cookie's name carries the slug, its `Path` is the prefix, and its
  signing key is per dashboard — so a cookie from one dashboard does not
  authenticate on another even after the holder renames it. If the signing keys
  are accidentally identical, the allowlist is re-checked per request against the
  dashboard being served, and that still holds the line.
- The short-lived OAuth state cookie has to stay at `Path=/` (`__Host-` requires
  it), so it carries the prefix in its **name** and the dashboard slug inside a
  **signed** payload. The signature is what makes the slug trustworthy: it
  decides which allowlist judges the login, and the cookie lives in the caller's
  own browser.
- A dashboard whose policy cannot be read locks down **its own subtree** with
  503; the others keep serving.

**You have to hold up.** One config directory per dashboard, an allowlist on
every client dashboard that should not be public (a missing `auth.yaml` means
public, and the startup log warns about it), and keeping credentials out of
client YAML. And, if you add a route: putting it in `setupClientRoutes` is a
security decision, not a convenience.

---

## Verify after every change

```bash
# 1. The dashboard answers under its prefix.
curl -so /dev/null -w '%{http_code}\n' https://example.com/acme                  # 200
curl -so /dev/null -w '%{http_code}\n' https://example.com/acme/api/healthcheck  # 200

# 2. Every URL on the page carries the prefix. Nothing may point at the root.
curl -s https://example.com/acme | grep -oE '(href|src|hx-get)="/[^"]*"' | sort -u

# 3. It serves ITS services and nobody else's.
curl -s https://example.com/acme/api/services

# 4. The host surface is absent for it. None of these may answer 200: a public
#    dashboard gives 404 (the route is not registered), a gated one gives
#    401/302 because the auth gate sits above the router.
for p in api/services/proxy api/docker/stats/x/y api/widgets/resources \
         api/validate api/scripts; do
  curl -so /dev/null -w "$p %{http_code}\n" "https://example.com/acme/$p"
done

# 5. No credential reaches the browser.
curl -s https://example.com/acme/api/services | grep -iE 'key|token|password|secret'

# 6. The root dashboard still answers, and the config parses.
curl -s https://example.com/api/validate
```

Then, in a browser: the page is styled (a 200 on `main.css` with an unstyled
page means the prefix is wrong somewhere), the status badges resolve, and — if
the dashboard is gated — an anonymous visit lands on **that dashboard's** login,
and signing in there does not sign you into any other.

---

## Never

- **Set `HOMEPAGE_BASE_PATH` to "add" a dashboard.** It moves *every* dashboard
  under a prefix; the previous URLs start answering 404. A dashboard is a
  directory under `dashboards/`.
- **Let the reverse proxy strip the prefix.** The process expects to receive
  `/acme/...` — that first segment is how it knows which dashboard you want.
  Stripped, the request is served by the ROOT dashboard.
- **Point two dashboards at one config directory.**
- **Name a dashboard `api`, `auth` or `static`** — reserved, and skipped with a
  log line rather than served.
- **Write a secret into any dashboard's YAML** — a reminder of where it lives is
  the whole point.
- **Override `session.cookieName` with a name another dashboard uses.**
- **Add a route to `setupClientRoutes` without asking what it exposes.** That
  list is the client dashboards' entire attack surface.

## Caveats — surprising behaviour worth knowing

- **No base path and no `dashboards/` directory ⇒ byte-identical to a build
  without the feature**, cookie names, headers and redirect targets included,
  with the gate both on and off. Verified against the previous binary, not only
  by tests. A single-dashboard install is the tested path, not a compromise.
- **A client dashboard is served the moment its directory exists**, and stops
  being served the moment it is deleted. No restart either way.
- **Adding or removing a client does not disturb the others**: the dashboards
  that survive a rescan keep their identity, so nobody is signed out and no cache
  is dropped by someone else's arrival.
- **The bare prefix works without a trailing slash.** `/acme` and `/acme/` both
  serve the dashboard; no redirect between them.
- **An invalid `HOMEPAGE_BASE_PATH` is fatal at startup.** It must start with
  `/`, must not end with one, and each segment is limited to `A-Za-z0-9._~-`.
  A silent fallback to the root would emit URLs that resolve nowhere.
- **The watcher watches the top of each dashboard's directory** plus
  `dashboards/` itself, and only `.yaml`, `.yml`, `.css`, `.js`. Deeper
  subdirectories (`data/`) and other extensions are not watched at all.
- **`custom.css` sits behind the gate**, so the login page never carries operator
  CSS. Add `/api/config/custom.css` to `auth.yaml: publicPaths` if you want your
  styling on the login screen of a gated dashboard.
- **A broken `auth.yaml` keeps the previous policy; one that vanishes locks the
  dashboard down with 503.** Only a well-formed, empty allowlist opens a
  dashboard up. Full table in `docs/context/authentication.md`.
- **`/api/validate` is not a strict YAML parser.** It accepts `key:{flow}` and
  silently produces the wrong shape. If a file parses but the dashboard is
  empty, run a strict linter.

## Where to look next

| Question | File |
|---|---|
| Card, widget, bookmark, icon syntax | `.agents/skills/add-widget/SKILL.md` |
| The instance, the proxy, the healthcheck | `guides/deploy.md` |
| Login, allowlists, Google, cookie names | `guides/auth.md` |
| Full auth schema and failure table | `docs/context/authentication.md` |
| Every config key | `docs/context/configuration.md` |
| Dashboard internals (for code changes) | `CLAUDE.md` §Dashboards |
| What a client dashboard may reach | `internal/handlers/api.go: setupClientRoutes` |
| The isolation contract, as tests | `internal/handlers/tenants_test.go` |
