import type { FeatureCollection } from 'geojson'
import { addProtocol, type StyleSpecification } from 'maplibre-gl'
import { Protocol } from 'pmtiles'
import type { VisualizationGeometryAsset, VisualizationMapStyleAsset } from '../../../../../generated/visualization'

const geometryCache = new Map<string, Promise<FeatureCollection>>()
const mapStyleCache = new Map<string, Promise<StyleSpecification>>()
let pmtilesRegistered = false

export function registerPMTilesProtocol(): void {
  if (pmtilesRegistered) return
  const protocol = new Protocol()
  addProtocol('pmtiles', protocol.tile)
  pmtilesRegistered = true
}

export function blankMapStyle(background: string): StyleSpecification {
  return { version: 8, sources: {}, layers: [{ id: '__ld-background', type: 'background', metadata: { 'libredash:role': 'background' }, paint: { 'background-color': background } }] }
}

export async function loadMapStyleAsset(asset: VisualizationMapStyleAsset, baseURL: string): Promise<StyleSpecification> {
  const styleURL = sameOriginGeometryURL(asset.styleUrl, baseURL)
  const archiveURL = sameOriginGeometryURL(asset.archiveUrl, baseURL)
  const glyphsURL = sameOriginGeometryURL(asset.glyphsUrl, baseURL)
  const spriteURL = sameOriginGeometryURL(asset.spriteUrl, baseURL)
  const key = `${styleURL.href}\0${asset.styleDigest}\0${archiveURL.href}\0${asset.archiveDigest}`
  let pending = mapStyleCache.get(key)
  if (!pending) {
    pending = (async () => {
      const response = await fetch(styleURL, { credentials: 'same-origin', redirect: 'error' })
      if (!response.ok) throw new Error(`map style asset ${JSON.stringify(asset.id)} returned ${response.status}`)
      const bytes = new Uint8Array(await response.arrayBuffer())
      await verifyGeometryDigest(bytes, asset.styleDigest)
      const style = JSON.parse(new TextDecoder().decode(bytes)) as StyleSpecification
      if (style.version !== 8 || !style.sources || !Array.isArray(style.layers)) throw new Error(`map style asset ${JSON.stringify(asset.id)} is not a MapLibre style`)
      for (const source of Object.values(style.sources) as Array<{ url?: string }>) {
        if (source.url === 'pmtiles://__LIBREDASH_ARCHIVE__') source.url = `pmtiles://${archiveURL.href}`
      }
      style.glyphs = glyphsURL.href
        .replace(/%7Bfontstack%7D/gi, '{fontstack}')
        .replace(/%7Brange%7D/gi, '{range}')
      style.sprite = spriteURL.href
      return style
    })()
    mapStyleCache.set(key, pending)
    void pending.catch(() => { if (mapStyleCache.get(key) === pending) mapStyleCache.delete(key) })
  }
  return structuredClone(await pending)
}

export async function loadGeometryAsset(asset: VisualizationGeometryAsset, baseURL: string): Promise<FeatureCollection> {
  const url = sameOriginGeometryURL(asset.url, baseURL)
  const key = `${url.href}\0${asset.digest}`
  let pending = geometryCache.get(key)
  if (!pending) {
    pending = (async () => {
      const response = await fetch(url, { credentials: 'same-origin', redirect: 'error' })
      if (!response.ok) throw new Error(`geometry asset ${JSON.stringify(asset.id)} returned ${response.status}`)
      const bytes = new Uint8Array(await response.arrayBuffer())
      await verifyGeometryDigest(bytes, asset.digest)
      const value = JSON.parse(new TextDecoder().decode(bytes)) as Partial<FeatureCollection>
      if (value.type !== 'FeatureCollection' || !Array.isArray(value.features)) throw new Error(`geometry asset ${JSON.stringify(asset.id)} is not a GeoJSON FeatureCollection`)
      return value as FeatureCollection
    })()
    geometryCache.set(key, pending)
    void pending.catch(() => { if (geometryCache.get(key) === pending) geometryCache.delete(key) })
  }
  return pending
}

export function sameOriginGeometryURL(value: string, base: string): URL {
  const url = new URL(value, base)
  if (url.origin !== new URL(base).origin) throw new Error('geometry asset must be same-origin')
  return url
}

export async function verifyGeometryDigest(bytes: Uint8Array, declared: string): Promise<void> {
  if (!/^sha256:[0-9a-f]{64}$/.test(declared)) throw new Error('geometry asset digest must be canonical SHA-256')
  const input = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', input))
  const actual = `sha256:${Array.from(digest, (value) => value.toString(16).padStart(2, '0')).join('')}`
  if (actual !== declared) throw new Error(`geometry asset digest mismatch: got ${actual}`)
}
