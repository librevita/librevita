// Test loader for node --test: transforms .ts and .tsx on the fly with
// the same esbuild options the production bundle uses (JSX -> h factory
// from jsx.ts), so the TSX modules can be exercised directly. Runs via
// `node --import ./internal/ui/test-loader.ts --test ...`.
//
// module.registerHooks() is used instead of exported load/resolve
// hooks: the test runner loads the graph synchronously, where exported
// (thread-side) hooks are never consulted. The runtime also accepts a
// `loadSync` key, which the published types do not describe; the extra
// key is asserted in the cast below.

import { registerHooks } from 'node:module';
import type {
  LoadFnOutput,
  LoadHookContext,
  LoadHookSync,
  RegisterHooksOptions,
} from 'node:module';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { transformSync } from 'esbuild';

const SOURCE = /\.ts$/;

// The SOURCE test runs on the URL itself so non-file schemes (node:
// builtins, data:, etc.) never reach fileURLToPath.
const hook: LoadHookSync = (url, context, nextLoad) => {
  if (SOURCE.test(url)) {
    const path = fileURLToPath(url);
    const { code } = transformSync(readFileSync(path, 'utf8'), {
      loader: path.endsWith('.tsx') ? 'tsx' : 'ts',
      format: 'esm',
      jsx: 'transform',
      jsxFactory: 'h',
    });
    const output: LoadFnOutput = { format: 'module', source: code, shortCircuit: true };
    return output;
  }
  return nextLoad(url, context);
};

registerHooks({
  load: hook,
  loadSync: hook,
} as RegisterHooksOptions & { loadSync: LoadHookSync });
