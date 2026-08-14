// LibreVita frontend build, driven by Node.
// Manifest: package.json + package-lock.json (no version pinning in
// the Taskfile). Produces internal/ui/static/js/ui.js, theme.js and
// the versioned HTMX runtime files, which the Go binary embeds via
// //go:embed.

import * as esbuild from 'esbuild';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { readFileSync } from 'node:fs';

// Output root; defaults to the embed tree, overridable with OUT.
const out = process.env.OUT ?? 'internal/ui/static/js';
await mkdir(out, { recursive: true });

// Browser floor: Firefox 52 ESR / Goanna. esbuild's firefox52 feature matrix
// rejects for-of and destructuring (its feature matrix is conservative,
// even though Firefox 52 supports both natively); firefox58 accepts them
// while still lowering `?.`/`??`/class fields, so the output stays
// compatible with the XP floor. The generated files are verified
// afterwards so a tool or dependency upgrade can never silently break
// the floor.
const xpForbidden = /\?\.|\?\?=|\?\?|\bcatch\s*\{/;

// Regex features esbuild cannot lower: lookbehind (?<= ?<!), named
// groups (?<name>) and unicode property escapes (\p{...} \P{...}) are
// valid ES2017+ syntax, so the parse-level checks never see them and
// the firefox58 target passes them through verbatim. Firefox 52 /
// Goanna reject all three at runtime, so grep the artifacts directly.
const xpHardRegex = /\(\?<|\\p\{|\\P\{/;

// The firefox58 matrix still allows optional catch binding (catch {}),
// which is a syntax error on Firefox 52 / Goanna. Force esbuild to keep
// a binding so the floor can parse the file.
const xpSupported = { 'optional-catch-binding': false };

function assertXpFloor(file: string) {
  const code = readFileSync(file, 'utf8');
  const offenders = code.match(xpForbidden) ?? [];
  if (offenders.length > 0) {
    throw new Error(
      `${file} contains syntax the XP floor cannot parse ` +
        `(${offenders.length} matches of ?./??): lower it or drop the dependency`,
    );
  }
  const regexOffenders = code.match(xpHardRegex) ?? [];
  if (regexOffenders.length > 0) {
    throw new Error(
      `${file} contains regex syntax the XP floor cannot parse ` +
        `(${regexOffenders.length} matches of lookbehind/named groups/unicode property escapes)`,
    );
  }
}

await esbuild.build({
  entryPoints: ['internal/ui/ts/main.ts'],
  outfile: `${out}/ui.js`,
  bundle: true,
  platform: 'browser',
  format: 'iife',
  target: 'firefox58',
  minify: true,
  supported: xpSupported,
  // TSX compiles to calls of the local factory h() (jsx.ts). Explicit
  // here (esbuild would read jsxFactory from tsconfig, but only when it
  // discovers it via cwd) and never 'preserve': raw JSX in the output
  // would be invalid JavaScript that assertXpFloor cannot detect.
  jsx: 'transform',
  jsxFactory: 'h',
});
assertXpFloor(`${out}/ui.js`);

// Theme bootstrap for the head: blocking so the dark class exists before
// first paint. Kept separate from ui.js (which is deferred).
await esbuild.build({
  entryPoints: ['internal/ui/ts/theme.ts'],
  outfile: `${out}/theme.js`,
  bundle: true,
  platform: 'browser',
  format: 'iife',
  target: 'firefox58',
  minify: true,
  supported: xpSupported,
  jsx: 'transform',
  jsxFactory: 'h',
});
assertXpFloor(`${out}/theme.js`);

// Runtime assets from the pinned npm packages. HTMX 1.9 is
// IE11-compatible and is copied verbatim. They land flat in the js
// output directory: base.templ and module.go reference the versioned
// names directly under /static/js/.
async function copyAsset(specifier: string, dest: string) {
  const url = import.meta.resolve(specifier);
  const data = await readFile(new URL(url));
  await writeFile(dest, data);
  assertXpFloor(dest);
}

await copyAsset('htmx.org/dist/htmx.min.js', `${out}/htmx-1.9.12.min.js`);
await copyAsset('htmx.org/dist/ext/sse.js', `${out}/htmx-sse-1.9.12.js`);

await esbuild.stop();
