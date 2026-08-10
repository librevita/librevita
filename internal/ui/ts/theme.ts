// Theme bootstrap, loaded as a blocking head script so the dark class is
// applied before first paint (CSP forbids inline scripts).
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
