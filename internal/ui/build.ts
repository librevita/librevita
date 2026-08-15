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
import postcss, { type Plugin } from 'postcss';
import { transformAsync } from '@babel/core';
import tailwindcss from 'tailwindcss';
import postcssSortMediaQueries from 'postcss-sort-media-queries';
import postcssCombineMediaQuery from 'postcss-combine-media-query';
import autoprefixer from 'autoprefixer';
import cssnano from 'cssnano';

const cssOut = 'internal/ui/static/css';
const jsOut = 'internal/ui/static/js';
await mkdir(cssOut, { recursive: true });
await mkdir(jsOut, { recursive: true });

// npm run assets -- --dev: unminified bundle with an inline source map
// for debugging on the old engines.
const devMode = process.argv.includes('--dev');

// WebKit 533 (Safari 4) does not implement the XMLHttpRequest Level 2
// `response` attribute for plain text responses (it arrived in Safari
// 5): there `xhr.response` reads as undefined and the htmx swap dies
// with "content is not an object". htmx 1.9.12 reads only xhr.response
// (never responseText), so patch those reads at build time to fall back
// to responseText. The readable and the minified dist use different
// variable names for the XHR (xhr, this, f). Applied while bundling, so
// a bare npm ci can never wipe the fix.
const patchHtmxXhrResponse: esbuild.Plugin = {
  name: 'patch-htmx-xhr-response',
  setup(build) {
    build.onLoad({ filter: /[\\/]node_modules[\\/]htmx\.org[\\/]dist[\\/]htmx\.(min\.)?js$/ }, async (args) => {
      const contents = await readFile(args.path, 'utf8');
      return {
        contents: contents.replace(/(\b(?:xhr|this|f))\.response(?![A-Za-z0-9])/g, '$1.response !== undefined ? $1.response : $1.responseText'),
        loader: 'js',
      };
    });
  },
};

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

function assertXpFloorCode(code: string, what: string) {
  const offenders = code.match(xpForbidden) ?? [];
  if (offenders.length > 0) {
    throw new Error(
      `${what} contains syntax the XP floor cannot parse ` +
        `(${offenders.length} matches of ?./??): lower it or drop the dependency`,
    );
  }
  const regexOffenders = code.match(xpHardRegex) ?? [];
  if (regexOffenders.length > 0) {
    throw new Error(
      `${what} contains regex syntax the XP floor cannot parse ` +
        `(${regexOffenders.length} matches of lookbehind/named groups/unicode property escapes)`,
    );
  }
}

function assertXpFloor(file: string) {
  assertXpFloorCode(readFileSync(file, 'utf8'), file);
}

// Legacy fixes for the Firefox 45-era engines (TenFourFox/AquaFox):
// - `inset` is Firefox 66+, so single-value uses are expanded to the
//   four longhands;
// - `:is(.dark *)` is Firefox 78+; `.dark *` is exactly equivalent
//   (same matching, same specificity), so dark-mode rules apply.
// (Colors need no fallback: Tailwind already emits a hex declaration
// before every modern rgb()/var() one.) All transforms are no-ops for
// modern browsers.
// Legacy fixes for the Firefox 45-era engines (TenFourFox/AquaFox).
// Runs after cssnano, which would discard or re-merge them:
// - the modern color syntax (`rgb(R G B / var(--tw-x,1))`) makes old
//   Gecko drop the WHOLE rule (not just the declaration), so every
//   such value is rewritten to the classic `rgba(R, G, B, var(...))`
//   form, which parses and resolves fine; a comma-rgb fallback is
//   kept before it for engines without var() support;
// - `inset` is Firefox 66+, so single-value uses are expanded to the
//   four longhands (cssnano would re-merge them into the shorthand);
// - `:is(.dark *)` is Firefox 78+; `.dark *` is exactly equivalent
//   (same matching, same specificity), so dark-mode rules apply.
// All transforms are no-ops for modern browsers. The color rewrites
// run in OnceExit: cssnano converts colors back to (8-digit) hex in
// its own OnceExit phase, which would otherwise undo them.
const legacyFallbacks: Plugin = {
  postcssPlugin: 'legacy-fallbacks',
  OnceExit(root) {
    root.walkDecls((decl) => {
      const value = decl.value;
      if (/^rgb\(\d+ \d+ \d+\s*\/\s*/.test(value)) {
        decl.cloneBefore({
          value: value.replace(/^rgb\((\d+) (\d+) (\d+)\s*\/\s*.*\)$/, 'rgb($1, $2, $3)'),
        });
        decl.value = value.replace(/^rgb\((\d+) (\d+) (\d+)\s*\/\s*(.+)\)$/, 'rgba($1, $2, $3, $4)');
      } else if (/#[0-9a-f]{8}/i.test(value)) {
        // cssnano folds alpha-modifier colors into 8-digit hex (also
        // inside box-shadow custom properties), which is Firefox 49+;
        // expand every occurrence to the classic rgba() form.
        decl.value = value.replace(
          /#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})/gi,
          (_m, r: string, g: string, b: string, a: string) =>
            `rgba(${parseInt(r, 16)}, ${parseInt(g, 16)}, ${parseInt(b, 16)}, ${(parseInt(a, 16) / 255).toFixed(3)})`,
        );
      } else if (decl.prop === 'inset' && /^[^ ]+$/.test(value.trim())) {
        const v = value.trim();
        for (const prop of ['top', 'right', 'bottom', 'left']) {
          decl.cloneBefore({ prop, value: v });
        }
      }
    });
  },
  Rule(rule) {
    rule.selectors = rule.selectors.map((selector) => {
      let out = selector;
      // `X:is(.dark *)` means "X that is a descendant of .dark", which
      // is exactly `.dark X`; the bare form means any descendant, i.e.
      // `.dark *`. (The naive `X.dark *` rewrite is wrong: it would
      // target descendants of an X.dark element instead.)
      if (out.includes(':is(.dark *)')) {
        const rest = out.replace(/:is\(\.dark \*\)/g, '');
        out = rest ? '.dark ' + rest : '.dark *';
      }
      // :where() is Firefox 78+. Tailwind's preflight uses it for the
      // form-element resets; an unsupported selector in a list drops
      // the whole rule (spec behavior), which leaves buttons and
      // inputs with the browser's default background. Rewriting
      // :where(S) to S keeps the matching (raising specificity to the
      // natural selector weight, still below the utilities).
      out = out.replace(/:where\(:not\(([^()]*)\)\)/g, ':not($1)');
      out = out.replace(/:where\(([^()]*)\)/g, '$1');
      return out;
    });
  },
};

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
    // Runs after cssnano so the minifier does not discard or re-merge
    // the legacy fallbacks.
    legacyFallbacks,
  ]).process(input, { from: 'internal/ui/input.css' });
  return writeHashed(cssOut, 'app', '.css', result.css);
}

// The single application bundle: the theme bootstrap (which must run
// before first paint) and the ui manifest (main.ts) in one file, loaded
// blocking in the head after the htmx runtime. In dev mode
// (npm run assets -- --dev) the output is unminified with an external
// source map, so old-engine devtools can map stacks back to the TS
// sources.
//
// PoC: the output goes through Babel (@babel/preset-env, safari 4)
// after esbuild, lowering the ES2015 syntax that esbuild refuses to
// lower (let/const, arrows, template literals, for-of), so the bundle
// parses on engines below ES2015 (WebKit 533 of Safari 4). The
// reserved-words plugin quotes reserved-word property names and object
// keys (the ES3-era parser of WebKit 533 rejects them), and the
// iterableIsArray assumption turns for-of into index loops, avoiding
// the Symbol.iterator machinery entirely.
async function buildJs(): Promise<{ name: string; hash: string; integrity: string }> {
  const result = await esbuild.build({
    entryPoints: ['internal/ui/ts/main.ts'],
    bundle: true,
    platform: 'browser',
    format: 'iife',
    target: 'es2015',
    minify: false,
    sourcemap: devMode ? 'inline' : false,
    supported: xpSupported,
    // Lets the bundle gate debug diagnostics (reportHtmxErrors) on the
    // build mode; esbuild tree-shakes the dead branch in production.
    define: { __LV_DEV__: devMode ? 'true' : 'false' },
    // Dev mode bundles the readable htmx dist (htmx.js) instead of the
    // minified one, so errors on the old engines point at real
    // functions and line numbers.
    alias: devMode ? { 'htmx.org/dist/htmx.min.js': 'htmx.org/dist/htmx.js' } : undefined,
    plugins: [patchHtmxXhrResponse],
    write: false,
    // TSX compiles to calls of the local factory h() (jsx.ts). Explicit
    // here (esbuild would read jsxFactory from tsconfig, but only when it
    // discovers it via cwd) and never 'preserve': raw JSX in the output
    // would be invalid JavaScript that assertXpFloor cannot detect.
    jsx: 'transform',
    jsxFactory: 'h',
  });
  const output = result.outputFiles[0];
  let text = output.text;
  const babel = await transformAsync(text, {
    assumptions: { iterableIsArray: true },
    presets: [['@babel/preset-env', { targets: { safari: '4' } }]],
    plugins: ['@babel/plugin-transform-reserved-words'],
  });
  text = babel ? babel.code ?? text : text;
  if (!devMode) {
    // Minify with the es5 target: the default esnext target would
    // re-introduce ??/?., which the XP floor gate rejects.
    const min = await esbuild.transform(text, { minify: true, target: 'es5' });
    text = min.code;
  }
  const asset = await writeHashed(jsOut, 'app', '.js', text);
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

// The theme bootstrap, bundled separately from the application: it must
// run before first paint, and the whole client is a single deferred
// script. It is rendered inline in the head with a content hash in the
// CSP (`script-src ... 'sha256-...'`), so the strict policy needs no
// unsafe-inline and no per-request nonce for a static script.
async function buildTheme(): Promise<{ script: string; hash: string }> {
  const result = await esbuild.build({
    entryPoints: ['internal/ui/ts/theme.ts'],
    bundle: true,
    platform: 'browser',
    format: 'iife',
    target: 'es2015',
    minify: false,
    supported: xpSupported,
    write: false,
  });
  let script = result.outputFiles[0].text;
  const babel = await transformAsync(script, {
    assumptions: { iterableIsArray: true },
    presets: [['@babel/preset-env', { targets: { safari: '4' } }]],
    plugins: ['@babel/plugin-transform-reserved-words'],
  });
  script = babel ? babel.code ?? script : script;
  if (!devMode) {
    const min = await esbuild.transform(script, { minify: true, target: 'es5' });
    script = min.code;
  }
  if (script.includes('`')) {
    throw new Error('theme bootstrap contains a backtick; the Go raw string cannot embed it');
  }
  assertXpFloorCode(script, 'theme bootstrap');
  return { script, hash: 'sha256-' + createHash('sha256').update(script).digest('base64') };
}

const theme = await buildTheme();

// The paths, SRI hashes and the inline theme script the templates
// render. Generated (not versioned): the Go build picks it up after the
// frontend runs.
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
  `\tThemeScript = \`${theme.script}\``,
  `\tThemeScriptHash = "${theme.hash}"`,
  ')',
  '',
].join('\n');
await writeFile('internal/ui/assets.go', assetsGo);

await esbuild.stop();
