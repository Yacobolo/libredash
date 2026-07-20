type CommandHeaders = Record<string, string>

function csrfToken(): string {
  return document.querySelector<HTMLMetaElement>('meta[name="csrf-token"]')?.content.trim() ?? ''
}

function headers(): CommandHeaders {
  const token = csrfToken()
  return token ? { 'X-CSRF-Token': token } : {}
}

declare global {
  interface Window {
    LeapViewCommand: {
      headers(): CommandHeaders
    }
  }
}

window.LeapViewCommand = { headers }

export {}
