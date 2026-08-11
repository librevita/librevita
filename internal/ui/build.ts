// LibreVita frontend build, driven by Node.
// Manifest: package.json + package-lock.json (no version pinning in
// Earthly). Produces /out/app.css, /out/ui.js, /out/theme.js, and
// /out/vendor/*.

import * as esbuild from 'esbuild';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { readFileSync } from 'node:fs';

// Output root; Earthly provides /out, local runs may override with OUT.
const out = process.env.OUT ?? '/out';

// Browser floor: Firefox 52 ESR / Goanna. esbuild's firefox52 feature matrix
// rejects for-of and destructuring (its feature matrix is conservative,
// even though Firefox 52 supports both natively); firefox58 accepts them
// while still lowering `?.`/`??`/class fields, so the output stays
// compatible with the XP floor. The generated files are verified
// afterwards so a tool or dependency upgrade can never silently break
// the floor.
const xpForbidden = /\?\.|\?\?=|\?\?|\bcatch\s*\{/;

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
});
assertXpFloor(`${out}/theme.js`);

// Runtime assets from the pinned npm packages. HTMX 1.9 is
// IE11-compatible and is copied verbatim.
await mkdir(`${out}/vendor`, { recursive: true });

async function copyAsset(specifier: string, dest: string) {
  const url = import.meta.resolve(specifier);
  const data = await readFile(new URL(url));
  await writeFile(dest, data);
  assertXpFloor(dest);
}

await copyAsset('htmx.org/dist/htmx.min.js', `${out}/vendor/htmx-1.9.12.min.js`);
await copyAsset('htmx.org/dist/ext/sse.js', `${out}/vendor/htmx-sse-1.9.12.js`);

await esbuild.stop();
