// Theme bootstrap, bundled separately by build.ts and rendered inline
// in the head, blocking, so the dark class is applied before first
// paint. The strict CSP allows exactly this script through its content
// hash (`script-src ... 'sha256-...'`, see ui.ThemeScriptHash); no
// unsafe-inline and no per-request nonce is needed for a static script.
//
// The preference comes from the server: the authenticated shell renders
// data-ui-theme (system|light|dark) on <html>, per user. Anonymous pages
// have no attribute and follow the system scheme. Browsers without
// prefers-color-scheme (the XP floor) stay light.

function readTheme(): string {
  return document.documentElement.getAttribute('data-ui-theme') ?? 'system';
}

function prefersDark(): boolean {
  if (typeof window.matchMedia !== 'function') {
    return false;
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function applyTheme(): void {
  const theme = readTheme();
  document.documentElement.classList.toggle(
    'dark',
    theme === 'dark' || (theme === 'system' && prefersDark()),
  );
}

applyTheme();

if (typeof window.matchMedia === 'function') {
  const mq = window.matchMedia('(prefers-color-scheme: dark)');
  const onSystemChange = (): void => {
    if (readTheme() === 'system') {
      applyTheme();
    }
  };
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', onSystemChange);
  } else if (typeof mq.addListener === 'function') {
    mq.addListener(onSystemChange);
  }
}
