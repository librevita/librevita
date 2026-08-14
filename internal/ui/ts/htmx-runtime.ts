// Loads the HTMX runtime into the bundle and publishes it as the
// global `htmx`. The package ships plain-source dists plus generated
// wrappers; htmx.cjs.js is the CommonJS wrapper (`module.exports =
// htmx`), which esbuild resolves like the UMD in htmx 1.x. The global
// is never assigned by the source itself, so it is published explicitly
// here. This module must be imported before the SSE extension and
// before the application code (core.ts reads the global and configures
// it).

import htmx from 'htmx.org/dist/htmx.cjs.js';

(globalThis as unknown as Record<string, unknown>).htmx = htmx;
