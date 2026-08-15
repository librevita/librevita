# Vendored assets and dependencies

All assets are served locally through `go:embed`; there is no CDN at runtime. Both the frontend and the backend pins
come from lockfiles, so no npm version or Go checksum is hard-coded anywhere else: `npm ci` installs from
`package-lock.json` (integrity hashes) and Go resolves from `go.sum`. Everything under `internal/ui/static` is generated
at build time and is not versioned.

## Backend

Go module `librevita.org`, built with `CGO_ENABLED=0` into a static binary on a `scratch` image; migrations live in
`db/migrations` and run through goose at startup.

| Package                                    | Version   | License      | Purpose                                             |
| ------------------------------------------ | --------- | ------------ | --------------------------------------------------- |
| `github.com/a-h/templ`                     | v0.3.1020 | MIT          | Type-safe server-rendered templates (SSR)           |
| `github.com/labstack/echo/v4`              | v4.15.4   | MIT          | HTTP router and middleware                          |
| `aidanwoods.dev/go-paseto`                 | v1.6.0    | MIT          | PASETO v4.local session tokens                      |
| `golang.org/x/crypto`                      | v0.54.0   | BSD-3-Clause | Argon2id password hashing (PHC format)              |
| `github.com/google/cel-go`                 | v0.28.0   | Apache-2.0   | Dynamic policy engine (RBAC rules at runtime)       |
| `github.com/pressly/goose/v3`              | v3.27.3   | MIT          | SQL migrations (embedded)                           |
| `modernc.org/sqlite`                       | v1.56.0   | BSD-3-Clause | Pure-Go SQLite driver (no CGO)                      |
| `github.com/canonical/go-dqlite/v3`        | v3.0.4    | Apache-2.0   | Optional dqlite backend: pure-Go wire protocol      |
| `github.com/minio/minio-go/v7`             | v7.2.1    | Apache-2.0   | S3-compatible API client for file storage           |
| `github.com/breml/rootcerts`               | v0.3.7    | BSD-2-Clause | Trusted roots for S3-compatible API TLS connections |
| `go.uber.org/fx`                           | v1.24.0   | MIT          | Dependency injection container                      |
| `go.uber.org/zap`                          | v1.28.0   | MIT          | Structured logging (UTC)                            |
| `go.uber.org/zap/exp`                      | v0.3.0    | MIT          | Audit trail logging                                 |
| `gopkg.in/natefinch/lumberjack.v2`         | v2.2.1    | MIT          | Log rotation for file output                        |
| `github.com/knadh/koanf/v2`                | v2.3.6    | MIT          | Config loading core                                 |
| `github.com/knadh/koanf/providers/env`     | v1.1.0    | MIT          | Environment variable provider                       |
| `github.com/knadh/koanf/providers/file`    | v1.2.1    | MIT          | Config file provider                                |
| `github.com/knadh/koanf/providers/posflag` | v1.0.2    | MIT          | pflag provider                                      |
| `github.com/knadh/koanf/parsers/json`      | v1.0.1    | MIT          | JSON parser                                         |
| `github.com/knadh/koanf/parsers/yaml`      | v1.1.1    | MIT          | YAML parser                                         |
| `github.com/spf13/pflag`                   | v1.0.10   | BSD-3-Clause | CLI flag parsing                                    |
| `github.com/joho/godotenv`                 | v1.5.1    | MIT          | `.env` support for local config                     |
| `github.com/google/uuid`                   | v1.6.0    | BSD-3-Clause | Session and record identifiers                      |
| `github.com/disintegration/imaging`        | v1.6.2    | MIT          | Image resize and re-encode (avatar pipeline)        |
| `golang.org/x/term`                        | v0.45.0   | BSD-3-Clause | Terminal width detection for the console logger     |

### Build-time Go tools

Installed by the Taskfile `tools` task and never
linked into the binary; used only to generate committed sources or to analyze the code.

| Package                                                  | Version   | License      | Purpose                                                       |
| -------------------------------------------------------- | --------- | ------------ | ------------------------------------------------------------- |
| `github.com/a-h/templ/cmd/templ`                         | v0.3.1020 | MIT          | Code generation for SSR templates (`templ generate`)          |
| `github.com/sqlc-dev/sqlc/cmd/sqlc`                      | v1.31.1   | MIT          | SQL query code generation (`sqlc generate`)                   |
| `github.com/golangci/golangci-lint/v2/cmd/golangci-lint` | v2.12.2   | GPL-3.0      | Static analysis (`golangci-lint run`, config `.golangci.yml`) |
| `golang.org/x/vuln/cmd/govulncheck`                      | v1.1.4    | BSD-3-Clause | Known-vulnerability scan of modules and the binary            |

## Frontend

Build chain: `package.json`/`package-lock.json` (Node) → `npm run assets` (`tsc --noEmit` + PostCSS + esbuild via
`internal/ui/build.ts`) through the Taskfile `frontend` task (Node 24 (LTS), see `.nvmrc`). The compiled CSS and a single JS
bundle — carrying the HTMX runtime, its SSE extension and the application code — are written into
the Go embed tree under content-addressed names (`app-<hash>.css`, `app-<hash>.js`); the bundle loads with `defer`.
The theme bootstrap is bundled separately and rendered inline in the head (ui.ThemeScript): the strict CSP allows
exactly that static script through its content hash (ui.ThemeScriptHash in `script-src`), so no `unsafe-inline` and
no per-request nonce is needed.

| Package        | Version | License | Purpose                                 |
| -------------- | ------- | ------- | --------------------------------------- |
| `htmx.org`              | 1.9.12  | 0BSD    | Hypermedia runtime (`dist/htmx.min.js`) |
| `htmx.org` (ext/sse.js) | 1.9.12  | 0BSD    | SSE extension (`dist/ext/sse.js`)       |

`htmx.org` is the only third-party runtime JavaScript. It is not copied: `internal/ui/build.ts` bundles its
`dist/htmx.min.js` (the 1.x UMD) and its SSE extension (`dist/ext/sse.js`) into the single application script
(`app-<hash>.js`) along with the first-party TypeScript (the ui manifest). `htmx-runtime.ts` publishes the api object
as the global `htmx` before the extension loads; the SSE extension is an IIFE calling `htmx.defineExtension('sse', ...)`.
The stylesheet (`app-<hash>.css`) is the PostCSS pipeline over `internal/ui/input.css`. Everything under
`internal/ui/static` is generated at build time, content addressed, and served with Subresource Integrity hashes (see
`internal/ui/assets.go`).

The SSE extension is configured (bundled and registered) for the upcoming agenda live updates. Elements opt in with
`hx-ext="sse"` plus `sse-connect="/endpoint"` (EventSource URL) and `sse-swap="<event-name>"` on the receiving element;
the server sends named events whose data is swapped into the element.

HTMX 1.9.12 is the terminal line for this application: it is ES5-era (IE11-compatible) and runs on the PowerPC-era
engines (TenFourFox/AquaFox, Firefox 45-based) with a single compatibility shim (`Document.evaluate` arguments in
compat.ts). HTMX 2.x was evaluated and rejected as a runtime: its modern APIs required a full compatibility layer
(XPath evaluator, getRootNode, ShadowRoot, append, Object.*, selector flags), and the 4.x line requires
`fetch`/`ReadableStream`/`AbortController`/`crypto.randomUUID`, which break the floor outright. The 1.9 line is frozen
since April 2024; the application never uses `hx-on` (eval) and hardens htmx with `allowEval=false` and
`allowScriptTags=false`.

### Build-time devDependencies

| Package                       | Version | License    | Purpose                                                |
| ----------------------------- | ------- | ---------- | ------------------------------------------------------ |
| `typescript`                  | 5.9.3   | Apache-2.0 | Type checking (`tsc --noEmit`)                         |
| `esbuild`                     | 0.28.1  | MIT        | Bundler for the TS modules (legacy floor target)            |
| `tailwindcss`                 | 3.4.17  | MIT        | CSS framework; compiled into `app-<hash>.css` at build |
| `postcss`                     | 8.5.26  | MIT        | CSS pipeline core (run in-process by `build.ts`)       |
| `postcss-sort-media-queries`  | 6.7.1   | MIT        | Mobile-first ordering of `@media` blocks               |
| `postcss-combine-media-query` | 2.1.0   | MIT        | Merges identical adjacent `@media` blocks              |
| `autoprefixer`                | 10.5.4  | MIT        | Vendor prefixes (driven by `browserslist`)             |
| `cssnano`                     | 8.0.4   | MIT        | Minification in production builds                      |
| `linkedom`                    | 0.18.13 | ISC        | DOM implementation for the `node:test` unit tests      |

The bundles are compiled with `target=firefox58` and the output is verified after the build by `assertLegacyFloor`: no
`?.`/`??` and no optional catch binding (`catch {}`, a syntax error on Firefox 52 and Goanna) may reach the output.
esbuild is forced to lower optional catch binding (`supported: { 'optional-catch-binding': false }`) because its
firefox58 feature matrix would otherwise allow it. The build fails on any modern-only syntax, which is the automated
half of the legacy floor.

PostCSS pipeline (in order): `tailwindcss`,
`postcss-sort-media-queries` (mobile-first order), `postcss-combine-media-query` (one merged block per breakpoint),
`autoprefixer` (driven by the `browserslist` `Firefox >= 52` entry) and `cssnano` (minification). The compiled
output is minified and has a single `@media (min-width: …)` block per Tailwind breakpoint.

Dark mode uses Tailwind's class strategy (`darkMode: 'class'`). The `dark` class is toggled on the `<html>` element by
the inline theme bootstrap in the head (system follow) and by the theme-pref module on the profile page
(light, system, or dark). The compiled dark
variant is `:is(.dark *)`, rewritten to `.dark <sel>` for the old engines (same matching, same specificity) and only matching descendants of the element carrying the `dark` class, so surface
backgrounds live on `<body>` and below — never on the `<html>` element itself.

The strict Content-Security-Policy (`script-src 'self'`, no `unsafe-eval`/`unsafe-inline`) is enforced at runtime;
interaction is progressive enhancement on top of server-rendered templ markup, so pages work even with scripting off.

Unit tests run with `node:test` + linkedom (`npm test`); the test-loader transpiles the `.ts`/`.tsx` sources with the
same esbuild options as the bundle.

## Design references

The UI follows the design language of the MIT-licensed
[flowbite-admin-dashboard](https://github.com/themesberg/flowbite-admin-dashboard): the shell layout (topbar, sidebar,
stat cards, tables), the sign-in page, and the dark mode palette. No code is copied; markup is rendered server-side with
templ and interaction stays with htmx and the first-party TypeScript modules as progressive enhancement.
