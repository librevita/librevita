// Theme bootstrap, loaded as a blocking head script so the dark class is
// applied before first paint (CSP forbids inline scripts).
//
// Preference precedence: stored user choice, then the system scheme.
// Browsers without prefers-color-scheme (the XP floor) stay light. The
// system listener only reacts while the user has no stored choice.

const THEME_KEY = 'librevita-theme';

function readStoredTheme(): string | null {
  try {
    return localStorage.getItem(THEME_KEY);
  } catch (_) {
    return null;
  }
}

function writeStoredTheme(value: string): void {
  try {
    localStorage.setItem(THEME_KEY, value);
  } catch (_) {
    // Private mode: the preference lives for the session only.
  }
}

function prefersDark(): boolean {
  if (typeof window.matchMedia !== 'function') {
    return false;
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function applyTheme(dark: boolean): void {
  document.documentElement.classList.toggle('dark', dark);
}

const stored = readStoredTheme();
applyTheme(stored === 'dark' || (stored === null && prefersDark()));

if (stored === null && typeof window.matchMedia === 'function') {
  const mq = window.matchMedia('(prefers-color-scheme: dark)');
  const onSystemChange = (event: { matches: boolean }): void => {
    if (readStoredTheme() === null) {
      applyTheme(event.matches);
    }
  };
  if (typeof mq.addEventListener === 'function') {
    mq.addEventListener('change', onSystemChange);
  } else if (typeof mq.addListener === 'function') {
    mq.addListener(onSystemChange);
  }
}
