// LibreVita frontend build, driven by Node.
// Manifest: package.json + package-lock.json (no version pinning in
// the Taskfile). Produces the content-addressed assets in
// internal/ui/static (app-<hash>.css, app-<hash>.js and the versioned
// HTMX runtime) plus internal/ui/assets.go with their paths, which the
// templates render and the Go binary embeds via //go:embed.

import * as esbuild from 'esbuild';
import { createHash } from 'node:crypto';
import { mkdir, readFile, readdir, unlink, writeFile } from 'node:fs/promises';
import { readFileSync } from 'node:fs';
import postcss from 'postcss';
import tailwindcss from 'tailwindcss';
import postcssSortMediaQueries from 'postcss-sort-media-queries';
import postcssCombineMediaQuery from 'postcss-combine-media-query';
import autoprefixer from 'autoprefixer';
import cssnano from 'cssnano';

const cssOut = 'internal/ui/static/css';
const jsOut = 'internal/ui/static/js';
await mkdir(cssOut, { recursive: true });
await mkdir(jsOut, { recursive: true });

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

// The CSS pipeline mirrors the old postcss.config.ts, which the bundle
// step used to run separately: Tailwind as a plugin, autoprefixer for
// the XP floor, and cssnano minification (the production flag was
// hardcoded in the npm script).
async function buildCss(): Promise<{ name: string; hash: string; integrity: string }> {
  const input = await readFile('internal/ui/input.css', 'utf8');
  const result = await postcss([
    tailwindcss(),
    postcssSortMediaQueries(),
    postcssCombineMediaQuery(),
    autoprefixer(),
    cssnano(),
  ]).process(input, { from: 'internal/ui/input.css' });
  return writeHashed(cssOut, 'app', '.css', result.css);
}

// The single application bundle: the theme bootstrap (which must run
// before first paint) and the ui manifest (main.ts) in one file, loaded
// blocking in the head after the htmx runtime.
async function buildJs(): Promise<{ name: string; hash: string; integrity: string }> {
  const result = await esbuild.build({
    entryPoints: ['internal/ui/ts/main.ts'],
    bundle: true,
    platform: 'browser',
    format: 'iife',
    target: 'firefox58',
    minify: true,
    supported: xpSupported,
    write: false,
    // TSX compiles to calls of the local factory h() (jsx.ts). Explicit
    // here (esbuild would read jsxFactory from tsconfig, but only when it
    // discovers it via cwd) and never 'preserve': raw JSX in the output
    // would be invalid JavaScript that assertXpFloor cannot detect.
    jsx: 'transform',
    jsxFactory: 'h',
  });
  const output = result.outputFiles[0];
  const asset = await writeHashed(jsOut, 'app', '.js', output.text);
  assertXpFloor(`${jsOut}/${asset.name}`);
  return asset;
}

// Content-addressed names: the hash is the fingerprint of the content,
// so a change in the sources produces a new file and the old one is
// cleaned up; unchanged content keeps the same name and the assets can
// be cached immutably. The integrity value is the full sha256-base64 of
// the content, served as the SRI hash on the script/link tags. The
// legacy stable names (app.css, ui.js, theme.js and the copied HTMX
// files) are removed so they never linger in the embed.
const LEGACY_STABLE = new Map<string, string[]>([
  [cssOut, ['app.css']],
  [jsOut, ['ui.js', 'theme.js', 'htmx-1.9.12.min.js', 'htmx-sse-1.9.12.js']],
]);

// Hashed names produced by earlier bundle names (ui-<hash>.js), so a
// rename never leaves stale embedded files behind.
const LEGACY_HASHED = new Map<string, RegExp>([
  [jsOut, /^ui-[0-9a-f]{10}\.js$/],
]);

async function writeHashed(
  dir: string,
  prefix: string,
  ext: string,
  content: string,
): Promise<{ name: string; hash: string; integrity: string }> {
  const hash = createHash('sha256').update(content).digest('hex').slice(0, 10);
  for (const entry of await readdir(dir)) {
    if (
      (entry.startsWith(prefix + '-') && entry.endsWith(ext)) ||
      (LEGACY_HASHED.get(dir)?.test(entry) ?? false)
    ) {
      await unlink(`${dir}/${entry}`);
    }
  }
  const stable = LEGACY_STABLE.get(dir) ?? [];
  for (const entry of stable) {
    try {
      await unlink(`${dir}/${entry}`);
    } catch {
      // Already gone.
    }
  }
  const name = `${prefix}-${hash}${ext}`;
  await writeFile(`${dir}/${name}`, content);
  const integrity = 'sha256-' + createHash('sha256').update(content).digest('base64');
  return { name, hash, integrity };
}

// Runtime assets from the pinned npm packages: the HTMX runtime and
// its SSE extension are imported by main.ts (htmx-runtime.ts) and end
// up inside the single app-<hash>.js bundle.

const [css, js] = await Promise.all([buildCss(), buildJs()]);

// The paths and SRI hashes the templates render. Generated (not
// versioned): the Go build picks it up after the frontend runs.
const assetsGo = [
  '// Code generated by internal/ui/build.ts; DO NOT EDIT.',
  '',
  'package ui',
  '',
  '// Content-addressed paths of the compiled frontend and their SRI',
  '// hashes: the hash changes when the content changes, so these files',
  '// can be cached immutably and the templates render them with the',
  '// integrity attribute (Subresource Integrity).',
  'const (',
  `\tAppCSS = "/static/css/${css.name}"`,
  `\tAppCSSIntegrity = "${css.integrity}"`,
  `\tAppJS = "/static/js/${js.name}"`,
  `\tAppJSIntegrity = "${js.integrity}"`,
  ')',
  '',
].join('\n');
await writeFile('internal/ui/assets.go', assetsGo);

await esbuild.stop();
