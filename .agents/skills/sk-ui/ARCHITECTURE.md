# Architecture — how a pixel gets on screen

## The request path

```
GET /
  └─ handlers.Dashboard()                       internal/handlers/pages.go
       ├─ config.LoadSettings/Services/Bookmarks/Widgets   (from the cache)
       ├─ builds templates.PageData
       └─ templates.Index(data).Render(ctx, w)
            └─ Layout(data)
                 ├─ Head(data)          asset links, preloads, theme init
                 ├─ Header(data)        resource widgets | datetime/weather/…
                 ├─ main > { children } ← Index's own body
                 │    ├─ BookmarkGroup … for each bookmark group
                 │    └─ ServiceGroup  … for each service group
                 │         └─ ServiceCard | ScriptCard   per service
                 ├─ Footer(data)
                 └─ Scripts(data)       theme.js, app.js, custom.js
```

Everything above is **one synchronous render**. What is *not* in that HTML is
anything that needs an upstream call: container stats, ping results, weather,
resource bars, widget payloads. Those arrive as HTMX fragments.

## The two-phase card

A service card renders in two phases, and understanding this explains most of
the markup:

1. **Phase one, server-side, synchronous.** Name, description, icon, and an
   empty shell for anything live. The shell is a `<div hx-get=… hx-trigger="load, every Ns">`
   holding a placeholder, so the card has its final height immediately and the
   page does not jump.
2. **Phase two, HTMX.** On `load` and then on a timer, the shell fetches its own
   fragment and replaces its `innerHTML`. Each shell is independent: a dead
   upstream degrades one panel, never the page.

The polling triggers all carry `[document.visibilityState === 'visible']` so a
background tab costs nothing.

| Panel | Endpoint | Interval |
|---|---|---|
| Docker status | `/api/docker/status/{container}/{server}` | 15s |
| Docker stats | `/api/docker/stats/{container}/{server}` | 5s |
| Ping | `/api/ping?groupName=…&serviceName=…` | 60s |
| Site monitor | `/api/siteMonitor?groupName=…&serviceName=…` | 60s |
| Service widget | `/api/services/proxy?group=…&service=…&endpoint=…` | 30s |
| Resource bars | `/api/widgets/resources?…` | 5s |
| Weather | `/api/widgets/openmeteo?…` | 300s |

## Content negotiation

Every one of those endpoints answers **HTML to HTMX and JSON to everyone else**,
keyed on the `HX-Request` header. `handlers.respond(w, r, htmlComponent, jsonPayload)`
is the helper; `isHTMXRequest(r)` is the predicate. Keep both branches
meaningful — the JSON side is a real API used by other clients, not an
afterthought.

## The stretched-link pattern

A whole service card is clickable without nesting interactive elements:

- an absolutely positioned `<a class="absolute inset-0 z-10">` covers the card;
- all visible content sits in `<div class="relative z-20 pointer-events-none">`,
  so it paints *above* the link while letting clicks fall through to it.

**Anything inside the card that must be clickable on its own needs
`pointer-events-auto`** — that is why `DynamicListHTML`'s `<ul>` carries it.
Forget it and the item link is dead, swallowed by the card-wide anchor.

HTMX polling is unaffected: the triggers are time-based, not click-based.

## Hot reload, and the two hashes

Two independent mechanisms, often confused:

| | `config.CurrentHash()` | `handlers.AssetVersion()` |
|---|---|---|
| Hashes | the user's `config/*.yaml` (+ `custom.css`/`custom.js`) | every file under `web/static` |
| Changes when | the operator edits config | you ship a new build |
| Drives | `<meta name="config-hash">` and the `/api/hash` poll in `app.js`, which reloads the page | the `?v=` on the CSS and JS links |

`app.js` polls `/api/hash` every 10s (paused while hidden) and reloads when it
differs from the meta tag. That is how a YAML edit reaches an open browser with
no restart.

Using the config hash for the asset `?v=` was a real bug: a deploy with a new
stylesheet and unchanged config produced the same URL, and browsers served the
cached CSS — for `max-age=86400` — against the new markup.

## Theming

`<html class="{theme} theme-{color}">` where `{theme}` is `dark` or empty and
`{color}` is one of the 23 palettes in `themes.css`. Each palette sets
`--color-50` … `--color-900` as raw RGB triplets; the `bg-theme-*`,
`text-theme-*` and `border-theme-*` utilities in `input.css` read them through
`rgb(var(--color-N))`.

`theme.js` loads **before** the body and applies the stored preference, which is
what avoids a flash of the wrong theme. Tailwind runs with `darkMode: "class"`,
so `dark:` variants key off that same `<html>` class.

## Auth pages are a separate shell

`login.templ` deliberately does not reuse `Layout`: those pages render before
the visitor is trusted, so they carry no services, no bookmarks, no widget
config, and they skip `/api/config/custom.css` — operator CSS is content, and
content is what the gate exists to protect. `AuthPageData` is correspondingly
minimal.

## What has no UI yet

- `layout.<group>.tab` parses and `TabNavigation` exists, but `index.templ`
  never calls it: groups always render as flat `<h2>` sections. Wire it up or
  drop the field — do not assume tabs work.
- `/api/releases` and `/api/search/searchSuggestion` are commented-out stubs.
