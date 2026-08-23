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

**A dashboard is a config directory plus a process.** Nothing more: there is no
dashboard registry, no database, no admin UI.

| | Comes from | Notes |
|---|---|---|
| Content | `HOMEPAGE_CONFIG_DIR` | One directory per dashboard. Never share one between two dashboards. |
| URL | `HOMEPAGE_BASE_PATH` | Unset ⇒ the host root (`/`). `/acme` ⇒ everything lives under `/acme`. |
| Who gets in | `<config dir>/auth.yaml` | Absent ⇒ public. At least one address ⇒ login required. |
| Features | process env (`HOMEPAGE_SCRIPTS_ENABLED`, …) | Per process, therefore per dashboard. |

**One process serves one dashboard.** Several dashboards on one hostname means
several instances behind a reverse proxy, each with its own config directory and
its own base path. That is what makes them isolated: not a code invariant, but a
process boundary — an instance cannot read a directory it was not given.

```
                           ┌── / ─────────────► instance A   config/
reverse proxy ─────────────┤
   (one hostname)          └── /acme/* ───────► instance B   config/dashboards/acme/
```

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

Nesting them there is a filing convention, not a feature: each instance is
pointed at its own directory and knows nothing about the others. `config/` and
`config/dashboards/acme/` are equally valid as `HOMEPAGE_CONFIG_DIR`, and the
loader only ever reads the fixed filenames at the top of the directory it was
given — a `dashboards/` subdirectory is invisible to the root instance.

Keeping them nested does buy one thing: a single bind mount carries every
dashboard, so a new client is a directory, not a volume.

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
   loader ignores — so a freshly scaffolded dashboard is **public**. To require
   login: fill it in, export its `{{HOMEPAGE_VAR_*}}` variables, then rename it
   to `auth.yaml`. (It ships renamed because an `auth.yaml` with unresolved
   placeholders makes the instance refuse to start — correct, but a puzzling
   first run.) Read `guides/auth.md` first: with two dashboards on one hostname
   there are two traps — a shared cookie name and the Google redirect URI — and
   both read as bugs rather than as misconfiguration.

4. **Run an instance for it.** `HOMEPAGE_CONFIG_DIR` at its directory,
   `HOMEPAGE_BASE_PATH=/acme`, and **no** `HOMEPAGE_SCRIPTS_ENABLED`. Full env
   table, compose skeleton and the reverse-proxy rules in `guides/deploy.md`.

5. **Verify** with the [checklist](#verify-after-every-change).

## Change an existing dashboard

Editing content is just editing YAML in that dashboard's directory — the
instance picks it up without a restart. What does **not** hot-reload:

| Change | Needs |
|---|---|
| Any `.yaml`, `custom.css`, `custom.js` at the top of the config dir | nothing, saved is applied |
| `data/*.json` (file:// sources) | nothing, but up to ~60s: the watcher ignores subdirectories and the `.json` extension, and the proxy caches a response for 60s |
| `auth.yaml` | nothing — the policy is read per request, so removing an address evicts that person on their next click |
| `settings.scripts.scriptDirs`, `maxTimeout`, `defaultTimeout`, `maxConcurrent` | a restart (read once at startup) |
| `HOMEPAGE_BASE_PATH`, any env var | a restart |

**Moving a dashboard to a different URL** is changing `HOMEPAGE_BASE_PATH` and
then: the reverse-proxy rule, the healthcheck path, and — if it uses Google —
`google.redirectURL` plus a new authorised redirect URI in the Google console.
Old links break; there is no redirect built in.

**Moving the root dashboard under a prefix** deserves its own warning: with
`HOMEPAGE_BASE_PATH=/home` set on it, the host root stops answering (404), so
the proxy needs a redirect `/` → `/home` if you want the bare hostname to keep
working. Leaving the root dashboard at the root costs nothing and keeps its
Google redirect URI as it is.

## Retire a client dashboard

Stop its instance, remove its proxy rule, then delete `config/dashboards/<slug>/`.
In that order: deleting the directory first leaves an instance serving a config
that no longer exists on disk — with an `auth.yaml` that vanished, the gate
answers 503 for everything, which is the correct fail-closed behaviour and an
alarming way to find out.

Removing the client's address from `auth.yaml` and leaving the dashboard up is
the softer version: they lose access on their next request.

---

## What a client dashboard must not have

Each of these is a concrete way for one client to reach something that is not
theirs. They are all avoided by configuration, not by code — which is why they
are listed here rather than trusted to a review.

| Never | Why |
|---|---|
| `HOMEPAGE_SCRIPTS_ENABLED=true` on a client instance | The scripts endpoints run shell on the host. With the variable unset the routes are not even registered. |
| A `widget:` carrying `key` / `token` / `password` / `username` | The widget proxy fetches the upstream **with those credentials** and returns the body to whoever loaded the card. A client dashboard has no business holding a credential of yours. |
| `docker.yaml`, or the Docker socket mounted into a client instance | Container names, stats, and whatever else the socket exposes. |
| The same config directory as another dashboard | Two dashboards, one `services.yaml`: everyone sees everything. |
| A real password, API key or token in any field | Nothing in a client dashboard needs a secret. Point at where the secret lives instead. |
| The same `session.cookieName` as another dashboard on that hostname | A login loop that looks random. See `guides/auth.md`. |

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

**Holds by itself.** Separate processes with separate config directories: an
instance cannot read another dashboard's YAML, cache, credentials or session
secret. A request outside a dashboard's base path gets a 404 from that instance
rather than a response from the wrong dashboard, and the session cookie is
scoped to the base path, so a client's cookie is not even sent to `/` or to
another prefix. The short-lived OAuth state cookie has to stay at `Path=/`
(`__Host-` requires it), so it carries the base path in its **name** instead —
two logins in flight under different prefixes cannot overwrite each other.

**You have to hold up.** The reverse-proxy rules (a wrong rule sends a client to
your dashboard's login, not to their data), one config directory per dashboard,
a distinct `session.cookieName` per dashboard on a shared hostname, and keeping
scripts and widget credentials out of client instances.

---

## Verify after every change

```bash
# 1. The dashboard answers under its prefix, and nothing else does.
curl -so /dev/null -w '%{http_code}\n' https://example.com/acme
curl -so /dev/null -w '%{http_code}\n' https://example.com/acme/api/healthcheck   # 200
curl -so /dev/null -w '%{http_code}\n' https://example.com/api/services           # not this instance's

# 2. Every URL on the page carries the prefix. Nothing may point at the root.
curl -s https://example.com/acme | grep -oE '(href|src|hx-get)="/[^"]*"' | sort -u

# 3. The config parses. Reports YAML errors with paths scrubbed.
curl -s https://example.com/acme/api/validate

# 4. No credential reaches the browser.
curl -s https://example.com/acme/api/services | grep -iE 'key|token|password|secret'
```

Then, in a browser: the page is styled (a 200 on `main.css` with an unstyled
page means the prefix is wrong somewhere), the status badges resolve, and — if
the dashboard is gated — an anonymous visit lands on **that dashboard's** login,
not on another one's.

---

## Never

- **Set `HOMEPAGE_BASE_PATH` on an instance to "add" a dashboard.** It moves the
  one that instance already serves; the previous URL starts answering 404.
- **Let the reverse proxy strip the prefix.** The instance expects to receive
  `/acme/...`. Stripped, every path it emits is wrong and half the routes 404.
- **Point two dashboards at one config directory.**
- **Enable scripts, Docker, or credential-bearing widgets on a client instance.**
- **Write a secret into any dashboard's YAML** — a reminder of where it lives is
  the whole point.
- **Reuse `session.cookieName` across dashboards on one hostname.**
- **Delete a dashboard's directory before stopping its instance.**

## Caveats — surprising behaviour worth knowing

- **No base path ⇒ byte-identical to a build without the feature**, cookie names
  and redirect targets included. Leaving the root dashboard unprefixed is not a
  compromise, it is the tested path.
- **The bare prefix works without a trailing slash.** `/acme` and `/acme/` both
  serve the dashboard; no redirect between them.
- **An invalid `HOMEPAGE_BASE_PATH` is fatal at startup.** It must start with
  `/`, must not end with one, and each segment is limited to `A-Za-z0-9._~-`.
  A silent fallback to the root would emit URLs that resolve nowhere.
- **The watcher only watches the top of the config directory**, and only
  `.yaml`, `.yml`, `.css`, `.js`. Subdirectories and other extensions are not
  watched at all.
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
| Base path internals (for code changes) | `CLAUDE.md` §Base path |
