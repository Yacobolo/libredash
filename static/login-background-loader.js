(() => {
  const maxAttempts = 120

  const schedule = (task) => {
    const run = () => {
      if ('requestIdleCallback' in window) {
        requestIdleCallback(task, { timeout: 1600 })
        return
      }
      setTimeout(task, 600)
    }
    if (document.readyState === 'complete') {
      run()
      return
    }
    window.addEventListener('load', run, { once: true })
  }

  const findBackground = () => {
    const direct = document.querySelector('[data-login-background]')
    if (direct) return direct
    return document.querySelector('ld-login-page')?.shadowRoot?.querySelector('[data-login-background]') ?? null
  }

  const load = (attempt = 0) => {
    const el = findBackground()
    if (!el) {
      if (attempt < maxAttempts) window.requestAnimationFrame(() => load(attempt + 1))
      return
    }
    const state = el.dataset.backgroundState
    if (state === 'loading' || state === 'loaded') return
    const src = el.dataset.moduleSrc
    if (!src) return
    el.dataset.backgroundState = 'loading'
    import(src)
      .then(() => {
        el.dataset.backgroundState = 'loaded'
      })
      .catch((error) => {
        el.dataset.backgroundState = 'error'
        console.error('LibreDash login background failed to load', error)
      })
  }

  const start = () => schedule(() => load())
  start()
  document.addEventListener('libredash-login-background-init', start)
})()
