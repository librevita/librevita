// Loads the HTMX runtime into the bundle and publishes it as the
// global `htmx`. The 1.9 dist is UMD: under esbuild it resolves
// through the CommonJS branch, which never assigns the global, so the
// api object is set explicitly here through window. This module must
// be imported before the SSE extension and before the application code
// (core.ts reads the global and configures it).

import htmx from 'htmx.org/dist/htmx.min.js';

(window as unknown as { htmx: unknown }).htmx = htmx;
