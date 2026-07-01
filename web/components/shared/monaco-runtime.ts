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

type MonacoTheme = 'libredash-light' | 'libredash-dark'

let runtimePromise: Promise<typeof monaco> | null = null
let highlighter: HighlighterCore | null = null
let themeListenerRegistered = false

type LibreDashThemeColors = {
  background: string
  foreground: string
  lineNumber: string
  activeLineNumber: string
  selection: string
  inactiveSelection: string
  cursor: string
  lineHighlight: string
}

export function loadMonacoRuntime(): Promise<typeof monaco> {
  runtimePromise ??= initializeMonaco()
  return runtimePromise
}

async function initializeMonaco(): Promise<typeof monaco> {
  registerWorker()
  registerLanguages()
  highlighter = await createHighlighterCore({
    themes: [
      libreDashTheme(githubLight, 'libredash-light', 'light'),
      libreDashTheme(githubDark, 'libredash-dark', 'dark'),
    ],
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
  if (document.documentElement.style.colorScheme === 'dark') return 'libredash-dark'
  return 'libredash-light'
}

function libreDashTheme<T extends typeof githubLight | typeof githubDark>(theme: T, name: MonacoTheme, mode: 'light' | 'dark'): T {
  return withLibreDashColors(theme, {
    name,
    ...libreDashThemeColors(mode),
  })
}

function withLibreDashColors<T extends typeof githubLight | typeof githubDark>(theme: T, colors: {
  name: MonacoTheme
  background: string
  foreground: string
  lineNumber: string
  activeLineNumber: string
  selection: string
  inactiveSelection: string
  cursor: string
  lineHighlight: string
}): T {
  return {
    ...theme,
    name: colors.name,
    colors: {
      ...theme.colors,
      'editor.background': colors.background,
      'editor.foreground': colors.foreground,
      'editorGutter.background': colors.background,
      'editorLineNumber.foreground': colors.lineNumber,
      'editorLineNumber.activeForeground': colors.activeLineNumber,
      'editorCursor.foreground': colors.cursor,
      'editor.selectionBackground': colors.selection,
      'editor.inactiveSelectionBackground': colors.inactiveSelection,
      'editor.lineHighlightBackground': colors.lineHighlight,
      'editorLineNumber.dimmedForeground': colors.lineNumber,
      'input.background': colors.background,
      'input.foreground': colors.foreground,
      'dropdown.background': colors.background,
      'dropdown.foreground': colors.foreground,
      'editorWidget.background': colors.background,
      'editorWidget.foreground': colors.foreground,
    },
  } as T
}

function libreDashThemeColors(mode: 'light' | 'dark'): LibreDashThemeColors {
  const scope = document.createElement('div')
  const probe = document.createElement('div')
  scope.dataset.colorMode = mode
  scope.dataset.lightTheme = 'light'
  scope.dataset.darkTheme = 'dark'
  scope.style.position = 'absolute'
  scope.style.inset = '0'
  scope.style.visibility = 'hidden'
  scope.style.pointerEvents = 'none'
  scope.append(probe)
  ;(document.body || document.documentElement).append(scope)
  try {
    return {
      background: cssColorToken(probe, '--ld-bg-panel', 'backgroundColor'),
      foreground: cssColorToken(probe, '--ld-fg-default', 'color'),
      lineNumber: cssColorToken(probe, '--ld-icon-muted', 'color'),
      activeLineNumber: cssColorToken(probe, '--ld-fg-accent', 'color'),
      selection: cssColorToken(probe, '--ld-bg-accent-muted', 'backgroundColor'),
      inactiveSelection: cssColorToken(probe, '--ld-bg-panel-muted', 'backgroundColor'),
      cursor: cssColorToken(probe, '--ld-fg-default', 'color'),
      lineHighlight: cssColorToken(probe, '--ld-bg-panel-muted', 'backgroundColor'),
    }
  } finally {
    scope.remove()
  }
}

function cssColorToken(element: HTMLElement, token: string, property: 'color' | 'backgroundColor'): string {
  if (!getComputedStyle(element).getPropertyValue(token).trim()) {
    throw new Error(`${token} is not defined`)
  }
  element.style[property] = `var(${token})`
  const color = getComputedStyle(element)[property]
  element.style[property] = ''
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
