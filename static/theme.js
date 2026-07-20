const storageKey = 'leapview-color-mode';
const root = document.documentElement;
const media = window.matchMedia?.('(prefers-color-scheme: dark)');
const nextModes = { system: 'light', light: 'dark', dark: 'system' };
const modeLabels = { system: 'System theme', light: 'Light theme', dark: 'Dark theme' };
let lastAppliedEvent = 0;

window.addEventListener('unhandledrejection', (event) => {
  const reason = event.reason;
  const message = typeof reason?.message === 'string' ? reason.message : '';
  if (reason?.name === 'AbortError' && message.includes('Transition was skipped')) {
    event.preventDefault();
    return;
  }
  if (reason?.name === 'InvalidStateError' && message.includes('Transition was aborted')) {
    event.preventDefault();
  }
});

function storedMode() {
  const saved = localStorage.getItem(storageKey);
  if (saved === 'system' || saved === 'light' || saved === 'dark') return saved;
  return 'system';
}

function setMode(mode, options = {}) {
  const next = mode === 'light' || mode === 'dark' ? mode : 'system';
  const resolved = next === 'system' ? (media?.matches ? 'dark' : 'light') : next;
  root.dataset.colorMode = next === 'system' ? 'auto' : next;
  root.dataset.lightTheme = 'light';
  root.dataset.darkTheme = 'dark';
  root.style.colorScheme = resolved;
  localStorage.setItem(storageKey, next);
  for (const button of document.querySelectorAll('[data-theme-value]')) {
    button.setAttribute('aria-pressed', String(button.dataset.themeValue === next));
  }
  for (const toggle of document.querySelectorAll('[data-theme-toggle]')) {
    const nextMode = nextModes[next] || 'system';
    const label = `${modeLabels[next] || 'System theme'}. Switch to ${modeLabels[nextMode] || 'system theme'}.`;
    toggle.dataset.themeMode = next;
    toggle.setAttribute('aria-label', label);
    toggle.setAttribute('title', label);
    for (const icon of toggle.querySelectorAll('[data-theme-icon]')) {
      const active = icon.dataset.themeIcon === next;
      icon.hidden = !active;
      icon.classList.toggle('hidden', !active);
    }
  }

  if (options.notify !== false) {
    const eventID = ++lastAppliedEvent;
    requestAnimationFrame(() => {
      if (eventID !== lastAppliedEvent) return;
      document.dispatchEvent(new CustomEvent('leapview-theme-applied', { detail: { mode: next, resolvedMode: resolved } }));
    });
  }
}

document.addEventListener('click', (event) => {
  const button = event.target.closest?.('[data-theme-value]');
  if (button) {
    setMode(button.dataset.themeValue);
    return;
  }

  const toggle = event.target.closest?.('[data-theme-toggle]');
  if (toggle) setMode(nextModes[storedMode()] || 'system');
});

document.addEventListener('leapview-theme-change', (event) => {
  setMode(event.detail?.mode);
});

media?.addEventListener?.('change', () => {
  if (storedMode() === 'system') setMode('system');
});

setMode(storedMode(), { notify: false });
