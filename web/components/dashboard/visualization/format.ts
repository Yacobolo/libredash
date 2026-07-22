import type { VisualizationFormat } from '../../../generated/visualization'

type LocaleData = { decimal: string; group: string; currencySpace: string; currencies: Record<string, string> }
const locales: Record<string, LocaleData> = {
  'en-US': { decimal: '.', group: ',', currencySpace: '', currencies: { USD: '$', BRL: 'R$', EUR: '€' } },
  'pt-BR': { decimal: ',', group: '.', currencySpace: '\u00a0', currencies: { USD: 'US$', BRL: 'R$', EUR: '€' } },
}

export function formatValue(locale: string, format: VisualizationFormat, value: unknown): string {
  const data = locales[locale]
  if (!data) throw new Error(`unsupported visualization locale ${JSON.stringify(locale)}`)
  if (value === null || value === undefined) return '—'
  switch (format.kind) {
    case 'number': return number(data, value, format.minimumFractionDigits ?? 0, format.maximumFractionDigits ?? 3)
    case 'currency': {
      const symbol = data.currencies[format.currency]
      if (!symbol) throw new Error(`unsupported visualization currency ${JSON.stringify(format.currency)}`)
      return symbol + data.currencySpace + number(data, value, format.minimumFractionDigits ?? 2, format.maximumFractionDigits ?? 2)
    }
    case 'percent': return number(data, numeric(value) * 100, format.minimumFractionDigits ?? 0, format.maximumFractionDigits ?? 1, '%')
    case 'compact': {
      const numericValue = numeric(value), absolute = Math.abs(numericValue)
      const [scale, suffix] = absolute >= 1e9 ? [1e9, 'B'] : absolute >= 1e6 ? [1e6, 'M'] : absolute >= 1e3 ? [1e3, 'K'] : [1, '']
      return number(data, numericValue / scale, 0, format.maximumFractionDigits ?? 1, suffix)
    }
    case 'duration': return duration(data, numeric(value), format.unit)
    case 'temporal': {
      if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z$/.test(value)) throw new Error('temporal visualization value must be RFC 3339 UTC')
      return format.timeStyle && !format.dateStyle ? value.slice(11, 19) : value.slice(0, 10)
    }
  }
}

function number(locale: LocaleData, raw: unknown, minimum: number, maximum: number, suffix = ''): string {
  const value = numeric(raw)
  if (!Number.isInteger(minimum) || !Number.isInteger(maximum) || minimum < 0 || maximum < minimum || maximum > 12) throw new Error(`invalid fraction digit range ${minimum}..${maximum}`)
  const factor = 10 ** maximum
  const rounded = Math.round(Math.abs(value) * factor) / factor
  let [integer, fraction = ''] = rounded.toFixed(maximum).split('.')
  while (fraction.length > minimum && fraction.endsWith('0')) fraction = fraction.slice(0, -1)
  integer = group(integer, locale.group)
  if (value < 0) integer = `-${integer}`
  return integer + (fraction ? locale.decimal + fraction : '') + suffix
}

function numeric(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) throw new Error('visualization value must be a finite number')
  return value
}

function group(value: string, separator: string): string {
  const first = value.length % 3 || 3
  let output = value.slice(0, first)
  for (let index = first; index < value.length; index += 3) output += separator + value.slice(index, index + 3)
  return output
}

function duration(locale: LocaleData, value: number, unit: string): string {
  if (unit === 'days') return number(locale, value, 0, 1, 'd')
  let seconds = Math.round(value)
  if (unit === 'milliseconds') seconds = Math.round(value / 1000)
  else if (unit === 'minutes') seconds *= 60
  else if (unit === 'hours') seconds *= 3600
  else if (unit !== 'seconds') throw new Error(`unsupported duration unit ${JSON.stringify(unit)}`)
  const hours = Math.floor(seconds / 3600), minutes = Math.floor(seconds % 3600 / 60), secs = seconds % 60
  const parts: string[] = []
  if (hours) parts.push(`${hours}h`); if (minutes) parts.push(`${minutes}m`); if (secs || !parts.length) parts.push(`${secs}s`)
  return parts.join(' ')
}
