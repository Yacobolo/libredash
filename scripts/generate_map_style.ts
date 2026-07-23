import { mkdir } from 'node:fs/promises'
import { layers, namedFlavor } from '@protomaps/basemaps'
import type { LayerSpecification, StyleSpecification } from 'maplibre-gl'

const outputDir = 'static/map-assets/leapview-streets'
await mkdir(outputDir, { recursive: true })

const roleFor = (layer: LayerSpecification): string | undefined => {
  if (layer.type === 'background') return 'background'
  if (layer.type === 'symbol') return 'label'
  if (layer.id === 'earth' || layer.id.startsWith('landuse_')) return 'land'
  if (layer.id === 'water' || layer.id.startsWith('water_')) return 'water'
  if (layer.id.startsWith('boundaries')) return 'boundary'
  if (layer.id.startsWith('roads_')) return 'road'
  if (layer.id === 'buildings') return 'building'
  return undefined
}

const styleLayers = layers('protomaps', namedFlavor('light'), { lang: 'en' }).map((layer) => {
  const role = roleFor(layer)
  return role ? { ...layer, metadata: { ...(layer.metadata ?? {}), 'leapview:role': role } } : layer
}) as LayerSpecification[]

const style: StyleSpecification = {
  version: 8,
  name: 'LeapView Streets',
  glyphs: 'https://invalid.local/glyphs/{fontstack}/{range}.pbf',
  sprite: 'https://invalid.local/sprites/leapview',
  sources: {
    protomaps: {
      type: 'vector',
      url: 'pmtiles://__LEAPVIEW_ARCHIVE__',
      attribution: '© OpenStreetMap contributors',
    },
  },
  layers: styleLayers,
}

await Bun.write(`${outputDir}/style.json`, `${JSON.stringify(style)}\n`)
