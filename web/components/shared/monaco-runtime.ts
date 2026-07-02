import * as monaco from 'monaco-editor-core/esm/vs/editor/editor.api'
import { shikiToMonaco } from '@shikijs/monaco'
import { createHighlighterCore, type HighlighterCore } from 'shiki/core'
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript'
import markdown from '@shikijs/langs/md'
import yaml from '@shikijs/langs/yaml'
import json from '@shikijs/langs/json'
import sql from '@shikijs/langs/sql'
import githubDark from '@shikijs/themes/github-dark'
import githubLight from '@shikijs/themes/github-light'

type MonacoTheme = 'github-light' | 'github-dark'

let runtimePromise: Promise<typeof monaco> | null = null
let highlighter: HighlighterCore | null = null
let themeListenerRegistered = false

export function loadMonacoRuntime(): Promise<typeof monaco> {
  runtimePromise ??= initializeMonaco()
  return runtimePromise
}

async function initializeMonaco(): Promise<typeof monaco> {
  registerWorker()
  registerLanguages()
  highlighter = await createHighlighterCore({
    themes: [githubLight, githubDarkWithPanelBackground()],
    langs: [markdown, yaml, json, sql],
    engine: createJavaScriptRegexEngine(),
  })
  shikiToMonaco(highlighter, monaco, {
    tokenizeMaxLineLength: 20000,
    tokenizeTimeLimit: 500,
  })
  applyTheme()
  registerThemeListener()
  return monaco
}

function registerWorker(): void {
  globalThis.MonacoEnvironment = {
    getWorker() {
      return new Worker('/static/monaco-editor-worker.js', { type: 'module' })
    },
  }
}

function registerLanguages(): void {
  for (const id of ['markdown', 'yaml', 'json', 'sql', 'text']) {
    if (!monaco.languages.getLanguages().some((language) => language.id === id)) {
      monaco.languages.register({ id })
    }
  }
}

function registerThemeListener(): void {
  if (themeListenerRegistered) return
  themeListenerRegistered = true
  document.addEventListener('libredash-theme-applied', applyTheme)
}

function applyTheme(): void {
  const theme = currentTheme()
  highlighter?.setTheme(theme)
  monaco.editor.setTheme(theme)
}

function currentTheme(): MonacoTheme {
  if (document.documentElement.style.colorScheme === 'dark') return 'github-dark'
  return 'github-light'
}

function githubDarkWithPanelBackground(): typeof githubDark {
  const background = darkPanelBackground()
  return {
    ...githubDark,
    colors: {
      ...githubDark.colors,
      'editor.background': background,
      'editorGutter.background': background,
    },
  }
}

function darkPanelBackground(): string {
  const scope = document.createElement('div')
  const probe = document.createElement('div')
  scope.dataset.colorMode = 'dark'
  scope.dataset.lightTheme = 'light'
  scope.dataset.darkTheme = 'dark'
  scope.style.position = 'absolute'
  scope.style.inset = '0'
  scope.style.visibility = 'hidden'
  scope.style.pointerEvents = 'none'
  scope.append(probe)
  ;(document.body || document.documentElement).append(scope)
  try {
    return cssColorToken(probe, '--ld-bg-panel')
  } finally {
    scope.remove()
  }
}

function cssColorToken(element: HTMLElement, token: string): string {
  if (!getComputedStyle(element).getPropertyValue(token).trim()) {
    throw new Error(`${token} is not defined`)
  }
  element.style.backgroundColor = `var(${token})`
  const color = getComputedStyle(element).backgroundColor
  element.style.backgroundColor = ''
  return colorToHex(color, token)
}

function colorToHex(color: string, token: string): string {
  const channels = color.match(/^rgba?\((\d+),\s*(\d+),\s*(\d+)(?:,\s*([\d.]+))?\)$/)
  if (!channels) throw new Error(`${token} did not resolve to an RGB color`)
  const [, red, green, blue, alpha] = channels
  return [
    Number(red),
    Number(green),
    Number(blue),
    alpha === undefined ? null : Math.round(Number(alpha) * 255),
  ]
    .filter((value): value is number => value !== null)
    .map((value) => value.toString(16).padStart(2, '0'))
    .join('')
    .replace(/^/, '#')
}
