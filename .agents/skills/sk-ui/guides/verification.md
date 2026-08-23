# Verifying a UI change

A screenshot is evidence. A measurement is proof. Most of the layout bugs in
this project's history looked fine in the screenshot that was taken to check
them — including one where the *page* was 700px wide inside a 320px viewport,
which no screenshot can show you.

## The mechanical checks

```bash
make templ       # regenerate — a .templ edit does nothing until you do
make tailwind    # recompile — markup whose classes are missing ships broken
make lint        # gofmt -l + go vet
make test        # go test ./...
make build       # all of the above plus the binary
```

`make templ` and `make tailwind` are not optional for a UI change: both outputs
are committed, and skipping either produces a diff that does not match what the
app renders.

## Running it locally

```bash
go build -o /tmp/myserver ./cmd/myserver/
HOMEPAGE_CONFIG_DIR=$PWD/config /tmp/myserver
```

`bootstrap-demo-config.sh` writes a `config/` covering every artifact type —
groups with 2, 3 and 4 columns, docker cards, ping and siteMonitor badges,
script cards, `customapi` widgets, bookmarks, top-bar widgets. Use it rather
than inventing a config: it exercises paths a hand-written one misses.

Note that `config/` is git-ignored, so it is safe to edit while testing.

## Checking the layout

**In the browser, at real widths.** Resize to 320, 390, 768, 1024 and 1920 —
those are the interesting ones, because they are where the column count and the
container queries change.

At each width, paste `templates/devtools-audit.js` into the console. It reports:

- whether the document scrolls horizontally, and **which element causes it** —
  the deepest offender, not the ancestors merely carrying its width;
- every ellipsised text node;
- the computed grid columns and card width.

Expected output at every width: no horizontal overflow, and zero clipped text.

If you have browser automation available outside the repo, drive that same audit
across a list of widths instead of resizing by hand. Keep the tooling out of
this repository: it is a Go project with no Node dependency tree, deliberately.

## What to check, specifically

| Check | Why it is on the list |
|---|---|
| `document.scrollWidth === document.clientWidth` at every width | One rigid element stretches the whole document and makes every `100%` element look narrow |
| No unintended `scrollWidth > clientWidth` on any element | Ellipsised text is information the reader lost |
| Cards fill the width at 320px | The failure mode that started all of this |
| The card grid column count at ≥1024px is unchanged | Desktop must not move |
| Stats row: 4 columns on wide cards, 2×2 on narrow ones | The container query threshold is in content-box pixels |
| A card's stretched link still receives the click | Layout containment and `pointer-events` both break it quietly |
| Both themes and both colour modes | `dark:` variants of theme utilities are hand-written and easy to miss |
| A hard reload picks up the new CSS | If it needs one, `AssetVersion` is not wired for that link |

## Regression tests worth writing

Not every UI change deserves a test, but these two shapes have earned their
place in `internal/templates` and `internal/handlers`:

- **A structural invariant.** "The column count arrives as a custom property and
  there is no inline `grid-template-columns`" survives a class rename and still
  catches the bug it was written for.
- **A cache-busting property.** "Editing a built asset changes `AssetVersion`"
  is three lines and prevents a class of bug that presents as a broken layout.

Assert on the property, never on a snapshot of the markup.

## Before saying it works

The user's QA is the gate. "The tests pass" is not "the feature works" — tests
verify the code, not the rendering. Show what you measured, say what you did not
check, and wait for confirmation.
