# Responsive — the layout system, and the four ways it broke

The dashboard runs from a 280px phone to a 2560px monitor with no separate
mobile view. This guide is written around real failures; each rule below fixed
one.

## The breakpoint ladder

Tailwind v3 defaults, used as-is:

| Prefix | From | Cards per row |
|---|---|---|
| — | 0 | 1 |
| `sm:` | 640px | 2 |
| `md:` | 768px | 3 |
| `lg:` | 1024px | the group's configured `columns:` (default 4) |

Those column counts live in **`.service-grid` in `input.css`**, not in classes on
the element. The per-group count from `layout.<group>.columns` arrives as the
inline custom property `--service-cols` and is read only in the `lg` block:

```css
.service-grid { @apply grid gap-3; grid-template-columns: minmax(0, 1fr); }
@media (min-width: 640px)  { .service-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (min-width: 768px)  { .service-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (min-width: 1024px) { .service-grid { grid-template-columns: var(--service-cols, repeat(4, minmax(0, 1fr))); } }
```

The fallback in the `var()` is what a group with no `columns:` entry gets.

## Failure 1 — an inline style beats every media query

The count used to be emitted as `style="grid-template-columns: repeat(4, …)"`
on an element that also carried `grid-cols-1 sm:grid-cols-2 md:grid-cols-3
lg:grid-cols-4`. **Those classes never applied.** A 390px phone got four ~85px
columns; names vanished, the status line ran past the card border, and the four
stat values overlapped into `CPMLEMRXTX`.

**Rule: a value that varies by viewport never travels in a `style` attribute.**
Pass it as a custom property and let the stylesheet decide where it applies.
`TestServiceGroup_ColumnsTravelAsCustomProperty` fails if this returns.

## Failure 2 — one rigid element stretches the whole document

With the grid fixed the cards still did not fill the screen. The measurement:
`document.scrollWidth > document.clientWidth` — **the page scrolled sideways**.

That matters more than it sounds. A full-page screenshot captures the *document*
width, so `width: 100%` elements render narrower than the image around them, and
the report you get is "the cards don't fill the screen" — nowhere near the cause.

The culprit was the header's right-hand cluster (datetime, greeting, weather,
buttons): `flex-shrink-0` with no `flex-wrap`. It could neither shrink nor wrap,
so it set a floor of ~550px on the document width.

**Rule: any horizontal cluster of unknown width must be able to wrap, shrink, or
both, below `sm`.** The fix is `flex-wrap` + `min-w-0` under `sm`, restoring
`sm:flex-nowrap sm:flex-shrink-0` above it.

**Corollary: horizontal overflow is the first thing to measure**, before
believing any description of a layout bug.

## Failure 3 — `min-width: auto` makes items overlap instead of clip

Flex and grid children default to `min-width: auto`: they refuse to shrink below
their content. That is why the four stat values overlapped rather than clipping.

**Rule: every flex/grid child holding text that might not fit gets `min-w-0`.**
It is on the status row, on the three HTMX status shells, and on all four stat
cells.

## Failure 4 — the stats row is sized by the card, not by the screen

At 768px cards are 237px and the values ellipsised. But card width does not
follow viewport width: at 1024px a 4-column group gives 239px cards and a
2-column group gives ~500px. **No media query can distinguish them.**

**Rule: anything whose layout depends on the card uses a container query.**

```css
.service-card  { container-type: inline-size; }
@container (max-width: 240px) {
  .service-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); row-gap: 0.375rem; }
}
```

Two subtleties:

- **The threshold is against the content box.** A 288px card with `p-3` queries
  as 264. Pick the number after subtracting padding.
- `container-type: inline-size` implies layout containment. It was verified not
  to change the card's geometry or break the stretched link — if you add it
  elsewhere, verify the same.

## Text: wrap, never truncate

Card name, description and status use `wrap-anywhere` and the card grows in Y.
An ellipsis hides information the reader came for, and card heights are already
free to differ — the grid row takes the tallest.

The stats row keeps `truncate` as a last-resort guard, but the container query
above means it is never reached (measured: zero clipped values from 280px to
2560px).

The one deliberate exception is `DynamicListHTML`: a scrollable list of rows
inside a card, where per-row truncation keeps the list readable.

## The invariants to check after any layout change

1. `document.scrollWidth === document.clientWidth` at every width.
2. No element has `scrollWidth > clientWidth` unless it is a deliberate
   scroll container.
3. Cards fill the available width at `xs`/`sm`.
4. Desktop is unchanged — the same column count and the same one-line rows.

`guides/verification.md` has the how. `templates/devtools-audit.js` checks 1 and
2 from the browser console with no tooling.
