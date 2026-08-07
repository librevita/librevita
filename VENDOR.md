# Vendored assets and dependencies

All assets are served locally through `go:embed`; there is no CDN at runtime. Both the frontend and the backend pins
come from lockfiles, so no npm version or Go checksum is hard-coded anywhere else: `npm ci` installs from
`package-lock.json` (integrity hashes) and Go resolves from `go.sum`. Everything under `internal/ui/static` is generated
at build time and is not versioned.

## Backend

Go module `librevita.org`, built with `CGO_ENABLED=0` into a static binary on a `scratch` image; migrations live in
`db/migrations` and run through goose at startup.

| Package                                    | Version        | License      | Purpose                                       |
| ------------------------------------------ | -------------- | ------------ | --------------------------------------------- |
| `github.com/a-h/templ`                     | v0.3.1020      | MIT          | Type-safe server-rendered templates (SSR)     |
| `github.com/labstack/echo/v4`              | v4.15.4        | MIT          | HTTP router and middleware                    |
| `aidanwoods.dev/go-paseto`                 | v1.6.0         | MIT          | PASETO v4.local session tokens                |
| `golang.org/x/crypto`                      | v0.54.0        | BSD-3-Clause | Argon2id password hashing (PHC format)        |
| `github.com/google/cel-go`                 | v0.28.0        | Apache-2.0   | Dynamic policy engine (RBAC rules at runtime) |
| `github.com/pressly/goose/v3`              | v3.27.3        | MIT          | SQL migrations (embedded)                     |
| `modernc.org/sqlite`                       | v1.56.0        | BSD-3-Clause | Pure-Go SQLite driver (no CGO)                |
| `github.com/rqlite/gorqlite`               | `50d445fd0ab9` | MIT          | Optional rqlite backend client (HTTP)         |
| `github.com/breml/rootcerts`               | v0.3.7         | BSD-2-Clause | Trusted roots for rqlite TLS connections      |
| `go.uber.org/fx`                           | v1.24.0        | MIT          | Dependency injection container                |
| `go.uber.org/zap`                          | v1.28.0        | MIT          | Structured logging (UTC)                      |
| `go.uber.org/zap/exp`                      | v0.3.0         | MIT          | Audit trail logging                           |
| `gopkg.in/natefinch/lumberjack.v2`         | v2.2.1         | MIT          | Log rotation for file output                  |
| `github.com/knadh/koanf/v2`                | v2.3.6         | MIT          | Config loading core                           |
| `github.com/knadh/koanf/providers/env`     | v1.1.0         | MIT          | Environment variable provider                 |
| `github.com/knadh/koanf/providers/file`    | v1.2.1         | MIT          | Config file provider                          |
| `github.com/knadh/koanf/providers/posflag` | v1.0.2         | MIT          | pflag provider                                |
| `github.com/knadh/koanf/parsers/json`      | v1.0.1         | MIT          | JSON parser                                   |
| `github.com/knadh/koanf/parsers/yaml`      | v1.1.1         | MIT          | YAML parser                                   |
| `github.com/spf13/pflag`                   | v1.0.6         | BSD-3-Clause | CLI flag parsing                              |
| `github.com/joho/godotenv`                 | v1.5.1         | MIT          | `.env` support for local config               |
| `github.com/google/uuid`                   | v1.6.0         | BSD-3-Clause | Session and record identifiers                |
| `golang.org/x/term`                        | v0.45.0        | BSD-3-Clause | Terminal input for the setup wizard           |

### Build-time Go tools

Installed in the Earthfile `+tools` target and never linked into the binary; used only to generate committed sources.

| Package                             | Version   | License | Purpose                                              |
| ----------------------------------- | --------- | ------- | ---------------------------------------------------- |
| `github.com/a-h/templ/cmd/templ`    | v0.3.1020 | MIT     | Code generation for SSR templates (`templ generate`) |
| `github.com/sqlc-dev/sqlc/cmd/sqlc` | v1.31.1   | MIT     | SQL query code generation (`sqlc generate`)          |

## Frontend

Build chain: `package.json`/`package-lock.json` (Node) → `npm run build` (`tsc --noEmit`, PostCSS, esbuild) inside the
Earthfile `+frontend` target (`node:26.7-alpine3.24`), with the artifacts copied back into the Go embed tree.

| Package                 | Version | License | Purpose                                 |
| ----------------------- | ------- | ------- | --------------------------------------- |
| `htmx.org`              | 1.9.12  | 0BSD    | Hypermedia runtime (`dist/htmx.min.js`) |
| `htmx.org` (ext/sse.js) | 1.9.12  | 0BSD    | SSE extension (`dist/ext/sse.js`)       |

`htmx.org` is the only runtime JavaScript: `js/vendor/htmx-1.9.12.min.js` and `js/vendor/htmx-sse-1.9.12.js`, copied
verbatim from `node_modules/htmx.org@1.9.12` by `internal/ui/build.ts`. Everything else under `internal/ui/static` is
first-party: `theme.js` and `ui.js` are esbuild bundles of `internal/ui/src/*.ts`, and `app.css` is the PostCSS pipeline
over `internal/ui/input.css` (see below) — none of it is vendored.

### Build-time devDependencies

| Package                       | Version | License    | Purpose                                                       |
| ----------------------------- | ------- | ---------- | ------------------------------------------------------------- |
| `typescript`                  | 5.9.3   | Apache-2.0 | Type checking (`tsc --noEmit`)                                |
| `esbuild`                     | 0.28.1  | MIT        | Bundler for the TS modules (XP floor target)                  |
| `tailwindcss`                 | 3.4.17  | MIT        | CSS framework; compiled into `app.css` at build               |
| `postcss`                     | 8.5.26  | MIT        | CSS pipeline core                                             |
| `postcss-cli`                 | 11.0.1  | MIT        | PostCSS runner for `npm run css`                              |
| `postcss-import`              | 16.1.1  | MIT        | Inlines `@import` statements                                  |
| `postcss-sort-media-queries`  | 6.7.1   | MIT        | Mobile-first ordering of `@media` blocks                      |
| `postcss-combine-media-query` | 2.1.0   | MIT        | Merges identical adjacent `@media` blocks                     |
| `autoprefixer`                | 10.5.4  | MIT        | Vendor prefixes (driven by `browserslist`)                    |
| `cssnano`                     | 8.0.4   | MIT        | Minification in production builds                             |
| `htmx-types`                  | 1.0.1   | ISC        | Type declarations for htmx (`@types/htmx.org` does not exist) |
| `linkedom`                    | 0.18.13 | ISC        | DOM implementation for the `node:test` unit tests             |

The bundles are compiled with `target=firefox58` and the output is verified after the build by `assertXpFloor`: no
`?.`/`??` and no optional catch binding (`catch {}`, a syntax error on Firefox 52 and Goanna) may reach the output.
esbuild is forced to lower optional catch binding (`supported: { 'optional-catch-binding': false }`) because its
firefox58 feature matrix would otherwise allow it. The build fails on any modern-only syntax, which is the automated
half of the XP floor.

PostCSS pipeline (in order): `postcss-import` (stylesheet can be split into modules), `tailwindcss`,
`postcss-sort-media-queries` (mobile-first order), `postcss-combine-media-query` (one merged block per breakpoint),
`autoprefixer` (driven by the `browserslist` `Firefox >= 52` entry) and `cssnano` in production builds. The compiled
output is minified and has a single `@media (min-width: …)` block per Tailwind breakpoint.

Dark mode uses Tailwind's class strategy (`darkMode: 'class'`). The `dark` class is toggled on the `<html>` element by
`theme.js` (system follow) and by the theme-pref module on the profile page (light, system, or dark). The compiled dark
variant is `:is(.dark *)`, which only matches descendants of the element carrying the `dark` class, so surface
backgrounds live on `<body>` and below — never on the `<html>` element itself.

The strict Content-Security-Policy (`script-src 'self'`, no `unsafe-eval`/`unsafe-inline`) is enforced at runtime;
interaction is progressive enhancement on top of server-rendered templ markup, so pages work even with scripting off.

Unit tests run with `node:test` + linkedom (`npm test`).

## Design references

The UI follows the design language of the MIT-licensed
[flowbite-admin-dashboard](https://github.com/themesberg/flowbite-admin-dashboard): the shell layout (topbar, sidebar,
stat cards, tables), the sign-in page, and the dark mode palette. No code is copied; markup is rendered server-side with
templ and interaction stays with htmx and the first-party TypeScript modules as progressive enhancement.

