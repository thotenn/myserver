# Styling — Tailwind v3, theme tokens and component classes

## The one source file

Edit **`web/tailwind/input.css`**. Never `web/static/css/main.css` — that is
build output. It happens to be committed (so a clone can run without the
Tailwind CLI), which makes it tempting to edit; don't. The next `make tailwind`
overwrites it, and in the meantime the two disagree.

Recompile with `make tailwind`, and commit the regenerated `main.css` with the
change. **Markup whose classes are not in the compiled CSS ships broken.**

## Why v3 and not v4

`input.css` uses `@layer base` with `@apply`, which v4 removed without the
`@reference` directive. The Dockerfile pins `v3.4.17`. Migrating means
rewriting `input.css` per the v4 upgrade guide — not a drive-by change.

## The theme token system

This is **not** Tailwind's colour palette. It is hand-written:

```
themes.css     .theme-blue { --color-50: 239 246 255; … --color-900: 30 58 138; }
                ↑ 23 palettes, each 9 raw RGB triplets

input.css      .bg-theme-500 { background-color: rgb(var(--color-500)); }
@layer base     .text-theme-500 { color: rgb(var(--color-500)); }
                .border-theme-200 { border-color: rgb(var(--color-200)); }
```

`<html class="{dark} theme-{color}">` selects the palette. `darkMode: "class"`
means `dark:` variants key off that same class.

Consequences you must respect:

- **Only the shades that exist in `input.css` exist.** `bg-theme-*` and
  `text-theme-*` cover 50–900; `border-theme-*` covers only 100, 200, 300 and
  700. Need another? Add the rule to `input.css` — writing the class alone
  produces nothing.
- **Dark variants are written by hand too**, as `.dark\:text-theme-400:is(.dark *)`.
  A new `dark:*-theme-*` combination needs its rule.
- The safelist in `tailwind.config.js` covers `(bg|text|border)-theme-{shade}`
  and their `dark:` forms so runtime-assembled names survive purging.

**Use the theme tokens for anything structural** — surfaces, text, borders.
Reach for a literal Tailwind colour only for semantics that must not change with
the palette: `text-green-600` for running, `text-red-500` for failed,
`ring-blue-500` for focus. Those are safelisted as
`(bg|text)-(blue|green|red|yellow)-(400|500|600|700)`.

## Component classes

Defined in `@layer components` in `input.css`, used across the templates:

| Class | What it is |
|---|---|
| `.service-card` | The card surface. Also `container-type: inline-size` — it is the container for card-level container queries |
| `.service-grid` | The responsive card grid; owns the breakpoints and reads `--service-cols` |
| `.service-stats` | The CPU/MEM/RX/TX row; folds 4→2 columns by container query |
| `.bookmark-item` | A bookmark chip |
| `.widget-label` / `.widget-value` | The label/value pair inside widgets |
| `.btn-primary` / `.btn-execute` | Buttons; `btn-execute` is full width, for script cards |
| `.status-dot` + `.status-running` / `.status-stopped` / `.status-unknown` | The state dot |
| `.usage-bar` / `.usage-bar-fill` | Progress bars |
| `.htmx-indicator` | Hidden until an HTMX request is in flight |
| `.widget-error` | Error state for a widget container |
| `.card-blur` | Opt-in translucent cards, toggled by a setting |
| `.wrap-anywhere` | `overflow-wrap: anywhere` — Tailwind v3 has no utility for it |

Add a new one when the same class list appears in three places, or when it needs
a media/container query — a utility string in the markup cannot express either.
See `templates/component.css`.

## `.wrap-anywhere`, and why `break-words` is not enough

`break-words` (`overflow-wrap: break-word`) breaks the line but **does not lower
the element's intrinsic min-content width**. A spaceless name or a URL therefore
still spills out of its card and scrolls the whole page — measured at 157px of
overflow in a 280px viewport. `overflow-wrap: anywhere` also shrinks min-content,
which is what actually contains the text. `.wrap-anywhere` declares
`break-word` first as a fallback, then `anywhere`.

## Purging: if a class does not render, it was not scanned

`content` in `tailwind.config.js`:

```
./internal/templates/*.templ
./internal/templates/*_templ.go
./web/tailwind/classes.html
./web/static/js/*.js
```

A class name assembled at runtime — concatenated in Go, or set from JS — is
invisible to that scan. Either write it literally somewhere scanned, or add a
`safelist` **RegExp**. The config is `.js` and not `.json` for exactly this
reason: v3's JSON loader cannot parse `{pattern: "…"}` and crashes inside
`setupContextUtils.js`.

`@apply`-ed classes are always emitted, scan or no scan — which is another
reason to move a runtime-variable class list into a component class.

## Operator CSS

`config/custom.css` is injected as `<link href="/api/config/custom.css">` after
the app's own stylesheets, so an operator can override anything. Two things
follow: do not rely on source order within your own CSS to beat them, and treat
your class names as a **public interface** — renaming `.service-card` breaks
every user's custom CSS.
