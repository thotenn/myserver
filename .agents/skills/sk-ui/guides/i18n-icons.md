# Text and icons

## Every user-visible string goes through `T`

`T(lang, "key")` reads `internal/templates/i18n.go`, which holds two hardcoded
maps, `en` and `es`. There is no translation file to load and no fallback chain
beyond English.

**Adding a string means adding it to both maps.** A key present in only one
renders as the key itself in the other language, which ships as visible
gibberish.

Key names are dotted by area, and the existing prefixes are the vocabulary:
`resources.`, `weather.`, `search.`, `quicklaunch.`, `widgets.`, `docker.`,
`scripts.`, `monitor.`, `greeting.`, `time.`, `auth.`. Reuse a prefix rather
than inventing one.

Any component that renders text takes `lang string` and passes it down.
`PageData.Language` comes from `settings.language`.

### Strings that JavaScript needs

`app.js` cannot call `T`, and Templ will not interpolate inside `<script>`. Pass
them as `data-*` attributes on the element and read them there — that is exactly
what `GreetingWidget` does with its four times of day. Give the JS side a
sensible hardcoded default too, so a missing attribute degrades quietly.

### Locale, not language

Date and number formatting happens in the browser via `Intl`, fed by
`datetimeLocale(lang)` (`layout.go`) → `data-locale` on `#datetime-widget`. It
maps `es` → `es-ES` and everything else to `en-US`. Extend that function when
adding a language, or dates keep formatting in English.

### Values are formatted in Go

`format.go` owns the units, and they are not translated: `FormatBytes` (B/KB/MB/
GB/TB, base 1024), `FormatPercent` (one decimal), `FormatDuration` (`3d`, `4h`,
`12m`, `30s`), `FormatLatency` (`ms` under a second, else `1.2s`),
`FormatStatusCode` (`ERR` for a non-positive code), `FormatTemp` (`°C`). Use
them rather than formatting inline, so every card reads the same.

## Icons

`iconURL(name)` in `icons.go` resolves a config string to a URL. Everything is a
remote CDN — no icons ship with the binary:

| Input | Resolves to |
|---|---|
| `https://…` / `http://…` | verbatim |
| `mdi-foo`, `mdi:foo` | Material Design Icons SVG on jsdelivr |
| `si-foo`, `si:foo` | Simple Icons, coloured, on cdn.simpleicons.org |
| `sh:foo` | simple-icons SVG on jsdelivr |
| `foo`, `foo.png`, `foo.svg`, `foo.webp` | homarr-labs/dashboard-icons; the extension picks the directory, PNG by default |

Those three hosts are in the CSP's `img-src`/`script-src` allowances. **Adding a
new icon source means widening the CSP** in `internal/middleware/security_headers.go` —
do not do it casually.

Rendering an icon:

```templ
<img src={ iconURL(svc.Icon) } alt="" loading="lazy"
     class="w-10 h-10 rounded flex-shrink-0 object-contain service-icon"/>
```

- `alt=""` because the icon is decorative — the name is right next to it.
- `loading="lazy"`: a dashboard has dozens.
- `flex-shrink-0` so the icon keeps its size while the text column shrinks.
- **One of `service-icon`, `bookmark-icon` or `script-icon` is required.**
  `setupIconErrors()` in `app.js` hides broken images by selecting those three
  classes — the CSP-safe replacement for an inline `onerror`. Without the class,
  a dead CDN leaves a broken-image glyph.

`resolveBookmarkIcon` adds a small name-based default for common bookmarks
(GitHub, Cloudflare, Vercel…) when the operator set none.

## Inline SVG

Used for UI chrome that must not depend on a CDN: the theme toggle, the search
icon, spinners, the script result marks. Keep them `fill="none"
stroke="currentColor"` so they inherit the theme colour, and size them with
`w-* h-*`.
