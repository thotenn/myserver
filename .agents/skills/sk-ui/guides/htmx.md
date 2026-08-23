# HTMX — live panels and the fragments behind them

HTMX 2.0.4, loaded from unpkg in `head.templ` along with the `json-enc`
extension. No other client-side machinery exists.

## The standard polled panel

Every live panel in the app is this shape:

```templ
<div hx-get={ someURL(args) }
     hx-trigger="load, every 15s [document.visibilityState === 'visible']"
     hx-swap="innerHTML"
     hx-target="this"
     class="flex items-center gap-1 min-w-0">
    <span class="status-dot status-unknown"></span>
    <span class="text-theme-400">…</span>
</div>
```

Five things are load-bearing:

| Part | Why |
|---|---|
| `hx-trigger="load, …"` | The first fetch happens on render, so the server response stays fast and independent of upstreams |
| `[document.visibilityState === 'visible']` | A background tab must cost nothing. **Never omit it on a repeating trigger** |
| `hx-target="this"` + `hx-swap="innerHTML"` | The shell survives the swap, so its triggers keep firing |
| A placeholder inside | Holds the card's final height; without it the page jumps on first swap |
| `min-w-0` | The swapped fragment is usually wider than the shell; see `guides/responsive.md` |

Pick the interval from how fast the data actually moves: docker stats 5s,
docker status 15s, ping and site monitors 60s, service widgets 30s, weather
300s.

## The fragment

A swap target is a plain `templ` component with **no wrapper of its own** — the
shell is already in the DOM. `PingHTML`, `SiteMonitorHTML`, `DockerStatusHTML`,
`DockerStatsHTML` and `DynamicListHTML` in `widget.templ` are the examples.

Because the fragment lands inside the card's `pointer-events-none` content
wrapper, **anything clickable in it needs `pointer-events-auto`** or the
card-wide stretched link swallows the click. `DynamicListHTML`'s `<ul>` carries
it for exactly this reason.

## Adding an endpoint that serves a fragment

1. **A URL builder in `urls.go`.** Never concatenate: group and service names
   come from user YAML and contain `&`, `=` and spaces. Use `url.QueryEscape`
   for query values and `url.PathEscape` for path segments.
2. **A handler that content-negotiates.** `respond(w, r, htmlComponent, jsonPayload)`
   returns HTML to HTMX and JSON to everything else. Both branches are real —
   the JSON side is a documented API. See `templates/handler.go.txt`.
3. **Register the route** in `setupRoutes` (`internal/handlers/api.go`), inside
   the `/api` group so it gets CORS, host validation and rate limiting. Anything
   that reaches an upstream or costs CPU gets an explicit `rateLimit(rps, burst)`.
4. **Errors return a rendered error state, not a 500 body.** A dead upstream
   must degrade one panel. `writeUpstreamError` keeps internal detail out of the
   response; the detail goes to the log.
5. **Add the path to the auth public-path allowlist only if it must be public.**
   `internal/middleware/auth.go` gates by allowlist, so a new route is protected
   by default. That is deliberate: `/api/services` plus `/api/widgets` plus the
   proxy can rebuild the whole dashboard from outside.

## Swapping something larger than a shell

The script card replaces its **own** body, so it uses a stable outer id:

```templ
<div class="service-card relative" id={ "script-card-" + svc.Script }>
    @ScriptCardBody(svc, lang)
</div>
```

```templ
<button hx-post={ "/api/scripts/" + svc.Script }
        hx-target={ "#script-card-" + svc.Script }
        hx-swap="innerHTML"
        hx-indicator={ "#script-spinner-" + svc.Script }>
```

The result component re-renders the execute button, so the card is usable again
after a run. `hx-indicator` points at a `.htmx-indicator` span, which
`input.css` keeps at `opacity-0` until a request is in flight.

## Confirmation is enforced on the server

`hx-confirm` is a convenience, not a control — it lives in the browser. The
server requires the `X-Homepage-Confirm: yes` header and answers **428** without
it, which is why the template sends both `hx-confirm` and `hx-headers`. Never
ship a destructive action guarded only by `hx-confirm`.

## Redirects

When a handler must redirect an HTMX request, `Location` does not work — the
browser never sees it. Answer `200` with an `HX-Redirect` header, as
`AuthLogout` does, and keep the plain `http.Redirect` for non-HTMX callers.

## What HTMX is not used for

Client-only behaviour stays in `app.js`: the clock, the greeting, the search
dropdown, icon error handling, the config-hash poll. The dividing line is
whether the server has anything to say. If the answer is data, it is a fragment;
if it is presentation, it is JS.
