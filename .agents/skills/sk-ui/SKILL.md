# sk-ui — Building the MyServer interface

Expert skill for changing **the dashboard's own UI code**: Templ components, the
Tailwind layer, the HTMX wiring and the Go handlers that render HTML.

> **This is the developer-facing skill.** Its companions are for the *operator*
> who configures a running instance through YAML and never writes Go:
> `add-widget/` for the content of a dashboard, `sk-clients/` for the dashboards
> themselves (several on one hostname, URL prefixes, per-dashboard auth). If a
> request can be satisfied by editing `config/*.yaml`, it belongs there — stop
> and say so. Come here only when the answer is new markup, new styles, or a new
> server-rendered fragment.

---

## The stack, and why each piece is there

| Layer | Technology | Notes |
|---|---|---|
| Markup | **Templ** (`a-h/templ`, pinned `v0.3.1001` in `go.mod`) | `.templ` compiles to `*_templ.go`, which **is committed**. Regenerate with `make templ`; a version drift between the generator and the runtime breaks the Go build. |
| Interactivity | **HTMX 2.0.4** from unpkg | Every dynamic panel is a server-rendered fragment swapped into place. There is no client-side framework and no client-side router. |
| Styles | **Tailwind CSS v3** standalone CLI | No Node.js in the repo. **v3 only** — v4 broke `@layer base` with `@apply`, which `input.css` depends on. Pinned `v3.4.17` in the Dockerfile. |
| Bespoke JS | `web/static/js/app.js`, `theme.js` | Plain IIFEs, no modules, no bundler, ES5-flavoured. |
| Server | Go handlers in `internal/handlers` | Content-negotiate on `HX-Request`: HTML for HTMX, JSON for API clients. |

The consequence worth internalising: **the browser never builds the UI.** Every
piece of visible state is rendered by Go and shipped as HTML. When you need
something to change on screen, the question is "which fragment re-renders?",
not "which component re-runs?".

---

## Where things live

```
internal/templates/
  layout.templ        <html>, <body>, container, header/main/footer frame
  head.templ          <head>: asset links, preloads, early theme init
  index.templ         the dashboard body: bookmarks, then service groups
  header.templ        top bar: resource widgets | datetime/weather/buttons
  footer.templ        the footer
  service_group.templ a group's <section> + the responsive grid
  service_card.templ  a service card + the docker stats placeholder
  script_card.templ   a script card and its post-execution result
  bookmark.templ      bookmark groups and items
  info_widgets.templ  top-bar widgets: search, datetime, greeting,
                      resources, weather, theme toggle
  widget.templ        HTMX swap targets: ping, siteMonitor, docker
                      status/stats, dynamic lists
  login.templ         the auth pages, deliberately a separate shell
  scripts.templ       the <script> tags at the end of <body>
  search.go           the search form's action: provider -> engine URL

  types.go            PageData, AuthPageData, TabGroup, DynamicListItem
  styles.go           inline-style helpers: gridColumns, barWidth, background
  layout.go           layout lookups: layoutForGroup, layoutColumns
  urls.go             every API URL builder, with escaping
  icons.go            icon name → CDN URL
  format.go           FormatBytes / Percent / Duration / Latency / Temp
  i18n.go             T(lang, key) with hardcoded en/es maps

web/tailwind/input.css   the ONLY stylesheet source you edit
web/static/css/main.css  BUILD OUTPUT — never edit by hand (it is committed)
web/static/css/themes.css  the 23 colour themes as CSS variables
web/static/js/app.js     widgets, search, hot reload, icon errors, poll pause
web/static/js/theme.js   the theme toggle
web/static/js/theme-init.js  applies the stored theme before the first paint
```

---

## Hard rules

Each of these has already cost a debugging session. They are not style
preferences.

### 1. Never put a responsive property in an inline `style`

An inline style beats every class and every media query. The column count used
to be emitted as `style="grid-template-columns: …"` next to
`sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4`, and **those classes never did
anything** — phones got the desktop layout and the card text overflowed.

Hand the value to the stylesheet as a **custom property** and let CSS decide
where it applies. `gridColumns()` emits `--service-cols`; `.service-grid` in
`input.css` owns the breakpoints. `TestServiceGroup_ColumnsTravelAsCustomProperty`
fails if this regresses.

### 2. A custom property needs `templ.SafeCSS`

Templ's CSS sanitiser silently drops properties it does not recognise, `--*`
among them. A helper returning a plain `string` loses the value with no error.
Return `templ.SafeCSS` — and only ever build such a string from data you
control (an int, an enum), never from user config.

### 3. `style` helpers return a complete declaration

`width: 42.0%;`, not `42.0%`. See `barWidth` and `gridColumns` in `styles.go`.

### 4. Never interpolate `{ … }` inside `<script>` or `<style>`

Templ emits those blocks literally, so `{ data.Foo }` ships as the characters
`{ data.Foo }`. Pass server data through `data-*` attributes and read it in
`app.js` — that is why `DatetimeWidget` carries `data-locale` and
`GreetingWidget` carries four `data-*` strings.

### 5. The CSP forbids inline handlers — and inline scripts, and eval

`script-src 'self' unpkg.com cdn.jsdelivr.net`. No `onclick`, `onsubmit`,
`onerror`. Attach listeners in `app.js` with `addEventListener` — that is why
`setupIconErrors()` exists instead of an inline `onerror`.

The same directive blocks two things that are easy to write by accident:

- **An inline `<script>` body.** The early theme script was inline in
  `head.templ` and never ran: the flash it prevents was back, and every page
  logged a violation. It is `web/static/js/theme-init.js` now, loaded blocking
  from `<head>`.
- **An `hx-trigger` filter.** `every 30s [document.visibilityState === 'visible']`
  is compiled by htmx with `new Function(...)`, which needs `'unsafe-eval'`.
  Each polled element logged an `EvalError` and lost its filter, so hidden tabs
  polled anyway. Keep `hx-trigger` free of `[...]`; conditions go in `app.js`
  (see the `htmx:beforeRequest` guard).

### 6. Dynamic attributes use `attr={ expr }`

`href={ "/x?v=" + data.AssetVersion }`. The interpolating form
`href="/x?v={ data.AssetVersion }"` emits a literal `{`.

### 7. An HTML comment cannot contain `--`

`<!-- the --service-cols property -->` is a **parse error** in Templ. Put the
explanation in a Go comment above the `templ` block instead. Go comments are
also the safe place for any commentary inside a component.

### 8. `min-w-0` is what lets a flex or grid item shrink

Their default `min-width: auto` is why the CPU/MEM/RX/TX values used to overlap
each other instead of clipping. Any flex/grid child holding text that might not
fit needs it.

### 9. Text wraps, it does not truncate

Card name, description and status use `wrap-anywhere` and the card grows
vertically. An ellipsis hides information the reader came for.

`break-words` is **not** sufficient: it breaks the line but leaves the element's
intrinsic min-content width at the full token, so a spaceless name or URL still
spills out of the card and scrolls the page. `.wrap-anywhere` in `input.css`
exists because Tailwind v3 has no `overflow-wrap: anywhere`.

### 10. Card-sized things need a container query, not a media query

The stats row fits four columns or two depending on the **card** width, and at
1024px a 4-column group gives ~239px cards while a 2-column group gives ~500px
ones. The viewport cannot tell them apart. `.service-card` is
`container-type: inline-size`; `.service-stats` queries it. Container queries
resolve against the **content box**, so a threshold excludes the card padding.

### 11. The `?v=` of a static asset is `handlers.AssetVersion()`

Never `config.CurrentHash()`. The config hash is derived from the user's YAML
alone, so a deploy shipping a new `main.css` produced the identical URL and
browsers kept the cached stylesheet for a day against new markup. It reads as a
broken layout, not as a caching problem. `config.CurrentHash()` still owns the
`config-hash` meta tag and the `/api/hash` reload poll — the two answer
different questions; do not merge them.

### 12. Never build an API URL by string concatenation

Use the builders in `urls.go`. Two reasons, both load-bearing:

- Service and group names come from user YAML and routinely contain `&`, `=`
  and spaces. Every builder escapes with `url.QueryEscape` or `url.PathEscape`.
- The dashboard can be served under a base path (`HOMEPAGE_BASE_PATH=/team`),
  and the builders are what add it. A literal `"/api/…"` in a template works
  perfectly at the root and 404s under a prefix.

Every builder takes `ctx` as its first argument — inside a `templ` component
`ctx` is in scope, so pass it straight through: `pingURL(ctx, groupName,
svc.Name)`. A new endpoint gets a new builder there, prefixed the same way.

### 13. If a class does not render, it was not scanned

Tailwind only emits classes it finds in `content` (see `tailwind.config.js`):
`internal/templates/*.templ`, `internal/templates/*_templ.go`,
`web/tailwind/classes.html`, `web/static/js/*.js`. A class assembled at runtime
in Go or JS must be added to the `safelist` **as a RegExp** — the config is
`.js` and not `.json` precisely because JSON cannot express that.

---

## Workflow for any UI change

1. **Read the guide for the area you are touching** (see below). The traps are
   documented because they are not visible in the code.
2. **Edit the `.templ` and `input.css` sources.** Never `main.css`.
3. **`make templ`** — regenerate. Commit the `*_templ.go` alongside the source.
4. **`make tailwind`** — recompile the stylesheet. It is a committed artifact;
   a change that skips this ships markup whose classes do not exist.
5. **`make lint && make test`.**
6. **Verify it in a browser at real widths** — see `guides/verification.md`.
   Screenshots are evidence; measurements are proof.
7. **Ask the user to confirm** before declaring it done.

`make build` does steps 3–4 plus the Go build, and is what the Docker image
runs.

---

## Guides

| File | Read it when |
|---|---|
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | You need the render path end to end, or you are adding a new page |
| [`guides/templ.md`](guides/templ.md) | Writing or editing any component |
| [`guides/styling.md`](guides/styling.md) | Touching classes, colours, themes or `input.css` |
| [`guides/responsive.md`](guides/responsive.md) | Anything about widths, grids, breakpoints or overflow |
| [`guides/htmx.md`](guides/htmx.md) | Adding a live panel, a swap target or an endpoint that returns HTML |
| [`guides/i18n-icons.md`](guides/i18n-icons.md) | Adding user-visible text or an icon |
| [`guides/verification.md`](guides/verification.md) | Before saying a change works |

## Templates

`templates/` holds a working example of every artifact this UI is made of.
Copy the closest one into `internal/templates/` and adapt it; each carries the
conventions inline.

> The `.templ` files here are **reference, not build input**. `.agents/` is a
> dot-directory, so `templ generate`, `go build`, `gofmt` and Tailwind's content
> scan all skip it — the examples can reference helpers that do not exist yet
> without breaking anything. Verified: a full `make build` after adding them is
> clean.

| File | What it builds |
|---|---|
| `service-card.templ` | A card variant: icon, title, body, status line, footer |
| `section.templ` | A titled section wrapping a responsive grid |
| `info-widget.templ` | A top-bar widget, with its HTMX shell and swap target |
| `htmx-fragment.templ` | A polled panel and the fragment it swaps in |
| `stat-row.templ` | A row of numeric values that survives a narrow card |
| `list-widget.templ` | A scrollable list inside a card |
| `badge-status.templ` | Status dots, pills and health badges |
| `form-input.templ` | Inputs, selects and buttons on the theme tokens |
| `empty-error.templ` | Empty, loading and error states |
| `handler.go.txt` | The Go handler that serves a fragment, negotiated |
| `component.css` | A new component class in `input.css`, done right |
| `widget.js` | A bespoke behaviour in `app.js`, CSP-safe |
| `devtools-audit.js` | Paste into the console: overflow and clipping report |
