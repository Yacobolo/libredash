const aperture = '<svg class="aperture" viewBox="0 0 24 24" aria-hidden="true"><circle class="ring" cx="12" cy="12" r="10"></circle><path class="blade" d="m14.31 8 5.74 9.94"></path><path class="blade" d="M9.69 8h11.48"></path><path class="blade" d="m7.38 12 5.74-9.94"></path><path class="blade" d="M9.69 16 3.95 6.06"></path><path class="blade" d="M14.31 16H2.83"></path><path class="blade" d="m16.62 12-5.74 9.94"></path></svg>'
const trace = '<svg class="aperture" viewBox="0 0 24 24" aria-hidden="true"><circle class="ring" cx="12" cy="12" r="10"></circle><circle class="trace" cx="12" cy="12" r="10"></circle><path class="blade" d="m14.31 8 5.74 9.94"></path><path class="blade" d="M9.69 8h11.48"></path><path class="blade" d="m7.38 12 5.74-9.94"></path><path class="blade" d="M9.69 16 3.95 6.06"></path><path class="blade" d="M14.31 16H2.83"></path><path class="blade" d="m16.62 12-5.74 9.94"></path></svg>'
const progress = '<svg class="aperture" viewBox="0 0 24 24" aria-hidden="true"><circle class="progress-track" cx="12" cy="12" r="10"></circle><circle class="progress-value" cx="12" cy="12" r="10"></circle></svg>'

document.querySelectorAll('[data-sizes]').forEach((row) => {
  const variant = row.dataset.sizes
  const icon = variant === 'trace' ? trace : variant === 'progress' ? progress : aperture
  row.innerHTML = [['xs', '16'], ['sm', '24'], ['md', '36']]
    .map(([size, label]) => `<span class="size-sample"><span class="spinner spinner--${variant}" data-size="${size}" aria-hidden="true">${icon}</span><span class="size-label">${label}</span></span>`)
    .join('')
})

const motionButton = document.querySelector('#motion-toggle')
motionButton.addEventListener('click', () => {
  const paused = document.body.classList.toggle('paused')
  motionButton.setAttribute('aria-pressed', String(paused))
  motionButton.querySelector('.tool-label-wide').textContent = paused ? 'Resume motion' : 'Pause motion'
  motionButton.querySelector('.tool-dot').style.background = paused ? 'var(--faint)' : 'var(--success)'
})

const themeButton = document.querySelector('#theme-toggle')
themeButton.addEventListener('click', () => {
  const root = document.documentElement
  const current = root.dataset.theme || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
  root.dataset.theme = current === 'dark' ? 'light' : 'dark'
})
