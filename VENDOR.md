# Frontend assets

All assets are served locally through `go:embed`; there is no CDN at
runtime. The frontend build is driven by Deno: `deno.json` declares the
dependencies and tasks, and `deno.lock` pins the npm packages with
integrity hashes, so no npm version or checksum is hard-coded in the
Earthfile. Everything in `internal/ui/static` is generated at build time
and is not versioned.

| Asset | Origin | Purpose |
| --- | --- | --- |
| `htmx-1.9.12.min.js` | `npm:htmx.org@1.9.12` (`dist/htmx.min.js`), copied verbatim | Hypermedia runtime |
| `htmx-sse-1.9.12.js` | `npm:htmx.org@1.9.12` (`dist/ext/sse.js`), copied verbatim | SSE extension |
| `alpine-csp-3.15.12.min.js` | `npm:@alpinejs/csp@3.15.12` (`dist/cdn.min.js`), lowered to the XP floor by esbuild | Alpine CSP build |
| `theme.js` | esbuild bundle of `internal/ui/src/theme.ts` | Blocking head script: applies the `dark` class from the stored preference or `prefers-color-scheme` before first paint |
| `ui.js` | esbuild bundle of `internal/ui/src/ui.ts` | Bootstrap: htmx config, CSRF header, Alpine components |

`ui.js` and `theme.js` are bundled with `target=firefox58`, and the
output is verified after the build: no `?.`/`??` and no optional catch
binding (`catch {}`, a syntax error on Firefox 52 and Goanna) may reach
the output. esbuild is forced to lower optional catch binding
(`supported: { 'optional-catch-binding': false }`) because its firefox58
feature matrix would otherwise allow it.

The Alpine CSP cdn build auto-starts when it loads; `ui.js` is loaded
before it and registers components on `alpine:init`. The CSP build
disables `x-html` and keeps the strict Content-Security-Policy intact.

Dark mode uses Tailwind's class strategy (`darkMode: 'class'`). The
`dark` class is toggled on the `<html>` element by `theme.js` (system
follow) and by the `themePref` component on the profile page (light,
system, or dark, persisted in `localStorage['librevita-theme']`). The
compiled dark variant is `:is(.dark *)`, which only matches descendants
of the element carrying the `dark` class, so surface backgrounds live on
`<body>` and below — never on the `<html>` element itself.

## Design references

The UI follows the design language of the MIT-licensed
[flowbite-admin-dashboard](https://github.com/themesberg/flowbite-admin-dashboard):
the shell layout (topbar, sidebar, stat cards, tables), the sign-in
page, and the dark mode palette. No code is copied; markup is rendered
server-side with templ and interaction stays with htmx/Alpine as
progressive enhancement.
