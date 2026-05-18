/** @type {import('tailwindcss').Config} */
//
// Tailwind v3 config. Uses JS (not JSON) because safelist patterns must be
// real RegExp objects — the JSON loader in Tailwind v3 cannot parse
// `{pattern: "string"}` and crashes with:
//
//   TypeError: Cannot read properties of undefined (reading 'includes')
//
// at setupContextUtils.js when handling the `{pattern}` shape.
//
module.exports = {
  content: [
    "./internal/templates/*.templ",
    "./internal/templates/*_templ.go",
    "./web/tailwind/classes.html",
    "./web/static/js/*.js",
  ],
  darkMode: "class",
  safelist: [
    // Theme color utilities (dark/light) — dynamic classes used in Templ
    // templates that Tailwind's content scan might miss.
    { pattern: /^(bg|text|border)-theme-(50|100|200|300|400|500|600|700|800|900)$/ },
    { pattern: /^dark:(bg|text|border)-theme-(50|100|200|300|400|500|600|700|800|900)$/ },
    // Semantic color utilities for status indicators.
    { pattern: /^(bg|text)-(blue|green|red|yellow)-(400|500|600|700)$/ },
    { pattern: /^dark:(bg|text)-(blue|green|red|yellow)-(400|500|600|700)$/ },
    // Opacity utilities used by htmx-indicator transitions.
    { pattern: /^opacity-(0|25|50|75|100)$/ },
  ],
  theme: {
    extend: {},
  },
  plugins: [],
};
