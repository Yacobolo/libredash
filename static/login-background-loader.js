(() => {
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

  document.addEventListener('libredash-login-background-init', () => {
    schedule(() => {
      const el = document.querySelector('[data-login-background]')
      if (!el) return
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
    })
  })
})()
