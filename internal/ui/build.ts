// LibreVita frontend build, driven by Node.
// Manifest: package.json + package-lock.json (no version pinning in
// the Taskfile). Produces the content-addressed assets in
// internal/ui/static (app-<hash>.css, app-<hash>.js and the versioned
// HTMX runtime) plus the Inter webfonts (woff2+woff, in
// internal/ui/static/fonts) and internal/ui/assets.go with the asset
// paths, which the templates render and the Go binary embeds via
// //go:embed.

import * as esbuild from 'esbuild';
import { createHash } from 'node:crypto';
import { mkdir, readFile, readdir, unlink, writeFile } from 'node:fs/promises';
import { readFileSync } from 'node:fs';
import postcss, { type Plugin } from 'postcss';
import tailwindcss from 'tailwindcss';
import postcssSortMediaQueries from 'postcss-sort-media-queries';
import postcssCombineMediaQuery from 'postcss-combine-media-query';
import autoprefixer from 'autoprefixer';
import cssnano from 'cssnano';

const cssOut = 'internal/ui/static/css';
const jsOut = 'internal/ui/static/js';
const fontsOut = 'internal/ui/static/fonts';
await mkdir(cssOut, { recursive: true });
await mkdir(jsOut, { recursive: true });
await mkdir(fontsOut, { recursive: true });

// npm run assets -- --dev: unminified bundle with an inline source map
// for debugging on the old engines.
const devMode = process.argv.includes('--dev');

// Browser floor: Firefox 52 ESR / Goanna. esbuild's firefox52 feature matrix
// rejects for-of and destructuring (its feature matrix is conservative,
// even though Firefox 52 supports both natively); firefox58 accepts them
// while still lowering `?.`/`??`/class fields, so the output stays
// compatible with the legacy floor. The generated files are verified
// afterwards so a tool or dependency upgrade can never silently break
// the floor.
const legacyFloorForbidden = /\?\.|\?\?=|\?\?|\bcatch\s*\{/;

// Regex features esbuild cannot lower: lookbehind (?<= ?<!), named
// groups (?<name>) and unicode property escapes (\p{...} \P{...}) are
// valid ES2017+ syntax, so the parse-level checks never see them and
// the firefox58 target passes them through verbatim. Firefox 52 /
// Goanna reject all three at runtime, so grep the artifacts directly.
const legacyFloorHardRegex = /\(\?<|\\p\{|\\P\{/;

// The firefox58 matrix still allows optional catch binding (catch {}),
// which is a syntax error on Firefox 52 / Goanna. Force esbuild to keep
// a binding so the floor can parse the file.
const legacyFloorSupported = { 'optional-catch-binding': false };

function assertLegacyFloorCode(code: string, what: string) {
  const offenders = code.match(legacyFloorForbidden) ?? [];
  if (offenders.length > 0) {
    throw new Error(
      `${what} contains syntax the legacy floor cannot parse ` +
        `(${offenders.length} matches of ?./??): lower it or drop the dependency`,
    );
  }
  const regexOffenders = code.match(legacyFloorHardRegex) ?? [];
  if (regexOffenders.length > 0) {
    throw new Error(
      `${what} contains regex syntax the legacy floor cannot parse ` +
        `(${regexOffenders.length} matches of lookbehind/named groups/unicode property escapes)`,
    );
  }
}

function assertLegacyFloor(file: string) {
  assertLegacyFloorCode(readFileSync(file, 'utf8'), file);
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
      } else if (/#[0-9a-f]{4}\b/i.test(value)) {
        // The shorthand #RGBA (Tailwind's transparent outlines and
        // ring colors minify to it) is Firefox 49+, so the legacy floor
        // drops the whole declaration; expand it to rgba() with the
        // standard digit doubling.
        decl.value = value.replace(
          /#([0-9a-f])([0-9a-f])([0-9a-f])([0-9a-f])\b/gi,
          (_m, r: string, g: string, b: string, a: string) =>
            `rgba(${parseInt(r + r, 16)}, ${parseInt(g + g, 16)}, ${parseInt(b + b, 16)}, ${(parseInt(a + a, 16) / 255).toFixed(3)})`,
        );
      } else if (decl.prop === 'inset' && /^[^ ]+$/.test(value.trim())) {
        const v = value.trim();
        for (const prop of ['top', 'right', 'bottom', 'left']) {
          decl.cloneBefore({ prop, value: v });
        }
      } else if (value.includes('--tw-space-') || value.includes('--tw-divide-')) {
        // Tailwind's space-x/space-y and divide-x emit
        // calc(<len>*(1 - var(--tw-<kind>-[xy]-reverse))) (margins on
        // the siblings, border widths for divide), but multiplication
        // in calc() is Firefox 117+ and the XP floor drops the whole
        // declaration, so the spacing silently disappears. The reverse
        // variable is always defined to 0 in the same rule (the app
        // never sets space-*-reverse), so the forms collapse to the
        // plain length and 0, keeping the modern output byte-identical
        // in effect.
        decl.value = value
          .replace(/calc\(([^)]*)\*\(1 - var\(--tw-(?:space|divide)-[xy]-reverse\)\)\)/g, '$1')
          .replace(/calc\(([^)]*)\*var\(--tw-(?:space|divide)-[xy]-reverse\)\)/g, '0');
      }
    });
  },
  Rule(rule) {
    rule.selectors = rule.selectors
      .map((selector) => {
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
        // :host is shadow-DOM only (Firefox 63+); Tailwind's preflight
        // pairs it with html, and an unsupported selector in a list
        // drops the whole rule (spec behavior), so the font-family,
        // line-height and tab-size resets never apply on the legacy floor.
        // No shadow roots exist in the app, so the selector can go.
        out = out.replace(/:host(\([^)]*\))?/g, '');
        return out;
      })
      .filter((selector) => selector.trim() !== '');
  },
};

// The CSS pipeline mirrors the old postcss.config.ts, which the bundle
// step used to run separately: Tailwind as a plugin, autoprefixer for
// the legacy floor, and cssnano minification (the production flag was
// hardcoded in the npm script).
// The Inter webfonts: static latin weights used by the app (the theme
// sets font-sans to Inter), copied from @fontsource/inter with
// content-addressed names, plus the @font-face rules appended to the
// main CSS. Both woff2 (Firefox 39+/modern) and woff (Safari 4.1-era
// WebKit) are shipped; engines without webfont support fall back to the
// system stack. Appended after the postcss pipeline, so the legacy
// fallbacks never touch them.
const FONT_WEIGHTS = [400, 500, 600];
const fontSrcDir = 'node_modules/@fontsource/inter/files';

async function buildFonts(): Promise<string> {
  for (const entry of await readdir(fontsOut)) {
    if (/^inter-.*\.woff2?$/.test(entry)) {
      await unlink(`${fontsOut}/${entry}`);
    }
  }
  const faces: string[] = [];
  for (const weight of FONT_WEIGHTS) {
    const srcs: string[] = [];
    for (const ext of ['woff2', 'woff']) {
      const raw = await readFile(`${fontSrcDir}/inter-latin-${weight}-normal.${ext}`);
      const hash = createHash('sha256').update(raw).digest('hex').slice(0, 10);
      const name = `inter-${weight}-${hash}.${ext}`;
      await writeFile(`${fontsOut}/${name}`, raw);
      srcs.push(`url(/static/fonts/${name}) format('${ext}')`);
    }
    faces.push(
      `@font-face{font-family:'Inter';font-style:normal;font-weight:${weight};font-display:swap;src:${srcs.join(',')}}`,
    );
  }
  return faces.join('\n');
}

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
  return writeHashed(cssOut, 'app', '.css', result.css + '\n' + (await buildFonts()));
}

// The single application bundle: the theme bootstrap (which must run
// before first paint) and the ui manifest (main.ts) in one file, loaded
// blocking in the head after the htmx runtime. In dev mode
// (npm run assets -- --dev) the output is unminified with an external
// source map, so old-engine devtools can map stacks back to the TS
// sources.
async function buildJs(): Promise<{ name: string; hash: string; integrity: string }> {
  const result = await esbuild.build({
    entryPoints: ['internal/ui/ts/main.ts'],
    bundle: true,
    platform: 'browser',
    format: 'iife',
    target: 'firefox58',
    minify: !devMode,
    sourcemap: devMode ? 'inline' : false,
    supported: legacyFloorSupported,
    // Lets the bundle gate debug diagnostics (reportHtmxErrors) on the
    // build mode; esbuild tree-shakes the dead branch in production.
    define: { __LV_DEV__: devMode ? 'true' : 'false' },
    write: false,
    // TSX compiles to calls of the local factory h() (jsx.ts). Explicit
    // here (esbuild would read jsxFactory from tsconfig, but only when it
    // discovers it via cwd) and never 'preserve': raw JSX in the output
    // would be invalid JavaScript that assertLegacyFloor cannot detect.
    jsx: 'transform',
    jsxFactory: 'h',
  });
  const output = result.outputFiles[0];
  const rawJs = await patchAlpineStyleAttribute(await lowerLegacyBundle(output.text));
  const jsContent = devMode ? `/* DEV_MODE_ASSET: DO NOT COMMIT */\n${rawJs}` : rawJs;
  const asset = await writeHashed(
    jsOut,
    'app',
    '.js',
    jsContent,
  );
  assertLegacyFloor(`${jsOut}/${asset.name}`);
  return asset;
}

// Alpine's x-transition applies inline styles through
// setAttribute("style", ...) (the Wl helper), which the strict CSP
// (style-src 'self') blocks: every transition logs a violation and the
// effect breaks. The CSSOM form (style.cssText = ...) is not subject to
// style-src, so the two call sites are rewritten. The count is asserted
// so an Alpine upgrade that changes the helper's shape fails the build
// instead of spamming the console silently.
async function patchAlpineStyleAttribute(code: string): Promise<string> {
  const callSites = (code.match(/setAttribute\("style",/g) ?? []).length;
  if (callSites !== 2) {
    throw new Error(
      `alpine style patch: expected 2 setAttribute("style") call sites, found ${callSites}`,
    );
  }
  // Replace the whole call (including its closing paren) so the
  // assignment stays syntactically valid: setAttribute("style",X) ->
  // style.cssText=X.
  const patched = code.replace(/setAttribute\("style",([^)]+)\)/g, 'style.cssText=$1');
  // The patch runs after every parser in the pipeline, so validate the
  // syntax here — a stray token would otherwise ship silently (the
  // floor assertions are regex-based and the minifier never re-parses).
  await esbuild.transform(patched, { minify: false });
  return patched;
}

// Babel preset-env lowers whatever the bundled dependencies (Alpine's
// CSP build, htmx) ship that the floor cannot parse — async/await above
// all (esbuild cannot transform it), but also optional catch binding,
// class fields and any other ES2016+ feature in the dependency trees.
// The targets come from the project's browserslist (Firefox >= 45), so
// the same config file drives Babel and autoprefixer. The step runs on
// every build: the first-party code is already esbuild-lowered and
// passes through nearly untouched, and a future dependency can never
// slip syntax past the hand-rolled checks again (async itself was such
// a blind spot). The output is re-minified with esbuild afterwards
// (Babel re-prints unminified; esbuild's minifier is target-aware, so
// it cannot introduce modern syntax), and assertLegacyFloor still gates
// the final artifact.
async function lowerLegacyBundle(code: string): Promise<string> {
  const babel = await import('@babel/core');
  const out = await babel.transformAsync(code, {
    // preset-env reads the browserslist ("Firefox >= 45") from
    // package.json automatically; modules are already bundled by
    // esbuild into an iife. (Babel 8 enables the bugfix plugins
    // always, so no bugfixes option is set.)
    presets: [['@babel/preset-env', { modules: false }]],
    comments: false,
    sourceMaps: false,
    babelrc: false,
    configFile: false,
  });
  if (!out?.code) {
    throw new Error('babel: transform produced no output');
  }
  if (devMode) {
    return out.code;
  }
  const min = await esbuild.transform(out.code, {
    minify: true,
    target: 'firefox58',
    // Babel's regenerator helpers use optional catch binding
    // (catch {}); the firefox58 matrix would keep it, so the same
    // override as the main build forces the lowering.
    supported: legacyFloorSupported,
  });
  return min.code;
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
    target: 'firefox58',
    minify: !devMode,
    supported: legacyFloorSupported,
    write: false,
  });
  const script = result.outputFiles[0].text;
  if (script.includes('`')) {
    throw new Error('theme bootstrap contains a backtick; the Go raw string cannot embed it');
  }
  assertLegacyFloorCode(script, 'theme bootstrap');
  return { script, hash: 'sha256-' + createHash('sha256').update(script).digest('base64') };
}

const theme = await buildTheme();

// The paths, SRI hashes and the inline theme script the templates
// render. Generated (not versioned): the Go build picks it up after the
// frontend runs.
const assetsGoHeader = devMode
  ? '// Code generated by internal/ui/build.ts (--dev); DO NOT COMMIT (DEVELOPMENT BUILD).\n// DEV_MODE_BUILD: DO NOT COMMIT. Built with unminified debug assets and inline sourcemap.'
  : '// Code generated by internal/ui/build.ts; DO NOT EDIT.';

const assetsGo = [
  assetsGoHeader,
  '',
  'package ui',
  '',
  '// Content-addressed paths of the compiled frontend and their SRI',
  '// hashes: the hash changes when the content changes, so these files',
  '// can be cached immutably and the templates render them with the',
  '// integrity attribute (Subresource Integrity).',
  'const (',
  `\tAppCSS          = "/static/css/${css.name}"`,
  `\tAppCSSIntegrity = "${css.integrity}"`,
  `\tAppJS           = "/static/js/${js.name}"`,
  `\tAppJSIntegrity  = "${js.integrity}"`,
  `\tThemeScript     = \`${theme.script}\``,
  `\tThemeScriptHash = "${theme.hash}"`,
  ')',
  '',
].join('\n');
await writeFile('internal/ui/assets.go', assetsGo);

await esbuild.stop();
