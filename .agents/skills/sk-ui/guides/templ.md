# Templ — writing components

## The generated files are committed

`.templ` → `*_templ.go` via `make templ`. **Both go in the same commit.** A
`.templ` edit without regenerating changes nothing at runtime, and the diff then
lies about what the app does.

The generator version is pinned twice and must stay in lockstep: `go.mod`
(`a-h/templ v0.3.1001`) and the Dockerfile's `TEMPL_VERSION` arg. A newer
generator emits calls the pinned runtime does not have, and the Go build fails.

## Comments

| Where | Use |
|---|---|
| Above a `templ` block | Go comments (`//`) — this is where explanation belongs |
| Inside a `templ` block | `<!-- … -->`, but see the trap below |

**An HTML comment cannot contain `--`.** `<!-- the --service-cols property -->`
is a parse error, not a warning. Since CSS custom properties and CLI flags both
start with `--`, this bites often. Put the prose in the Go comment above the
component.

## Attributes

```templ
<a href={ "/x?v=" + data.AssetVersion }>          // correct
<a href="/x?v={ data.AssetVersion }">             // emits a literal {
```

Conditional attributes go inline in the element:

```templ
<button
    hx-post={ "/api/scripts/" + svc.Script }
    if svc.RequireConfirm {
        hx-headers='{"X-Homepage-Confirm": "yes"}'
        hx-confirm={ T(lang, "scripts.confirm") + " " + svc.Name + "?" }
    }
>
```

A class list mixing a constant and a variable takes a slice:

```templ
<div class={ "h-full rounded-full", barColor }></div>
```

## `<script>` and `<style>` are literal

Templ does not interpolate inside them — `{ data.Foo }` ships as those
characters. There are exactly two supported ways to get server data into the
browser:

1. **`data-*` attributes**, read in `app.js`. `DatetimeWidget` passes
   `data-locale`; `GreetingWidget` passes four translated strings.
2. **A fetch** — i.e. make it an HTMX fragment instead.

## Inline styles

Only three helpers produce one, all in `styles.go`, and all return a **complete
declaration**:

| Helper | Returns | Type |
|---|---|---|
| `barWidth(pct)` | `width: 42.0%;` | `string` |
| `gridColumns(n)` | `--service-cols: repeat(4, minmax(0, 1fr));` | `templ.SafeCSS` |
| `backgroundStyle(settings, hash)` | the full `background-image: …` block | `templ.SafeCSS` |

Two rules:

- **A responsive property never goes in an inline style.** It beats every media
  query. Emit a custom property and let `input.css` decide the breakpoints.
- **A custom property requires `templ.SafeCSS`.** The CSS sanitiser drops `--*`
  from plain strings, silently. `SafeCSS` skips sanitisation but **not** HTML
  escaping, so build such a string only from data you control.

`backgroundStyle` carries a third trap in its own comment: the URL must be
emitted **unquoted**, because attribute escaping runs after sanitisation and
turns `"` into `&#34;`, which the browser then reads as literal text inside
`url(...)`.

## Security while rendering

- `templ.SafeURL(…)` is required for a user-supplied `href`. Already used for
  `svc.Href`.
- Text content is escaped automatically. Do not hand-build HTML strings to work
  around it.
- Never render a secret. `WidgetConfig` tags `Key`, `Password`, `APIKey`,
  `Token`, `Secret` and `Headers` as `json:"-"` on purpose, and the sanitisation
  layer strips them from API responses. Templates render from the loaded config
  and must not reach for those fields.

## Adding a component

1. Put it in the `.templ` file that owns its area (see the map in `SKILL.md`).
   A new area gets a new file, `package templates`.
2. Take `lang string` if it renders any text, and route it through
   `T(lang, "…")`.
3. Take the config struct directly (`config.Service`, `config.ServiceGroup`) —
   templates consume them without an intermediate view model. Add a field to
   `PageData` only when the value cannot be derived from what is already there.
4. `make templ`, then build.

## Testing a component

`internal/templates` renders to a `strings.Builder` and asserts on the HTML:

```go
var sb strings.Builder
err := ServiceGroup(group, layout, "es", false).Render(context.Background(), &sb)
```

Assert on the **property that must hold**, not on exact markup. The existing
test checks that the column count arrives as a custom property and that no
inline `grid-template-columns` came back — it survives a class rename and still
catches the regression it was written for.
