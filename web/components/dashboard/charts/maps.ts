type Feature = {
  type: 'Feature'
  properties: { name: string }
  geometry: { type: 'Polygon'; coordinates: number[][][] }
}

const stateBoxes: Array<[string, number, number, number, number]> = [
  ['RR', -64, 1, -58, 5],
  ['AP', -53, -1, -49, 3],
  ['AM', -73, -9, -58, 1],
  ['PA', -58, -9, -46, 1],
  ['AC', -73, -11, -66, -7],
  ['RO', -66, -13, -60, -9],
  ['MT', -60, -18, -50, -9],
  ['TO', -50, -14, -46, -7],
  ['MA', -46, -8, -42, -2],
  ['PI', -42, -10, -39, -3],
  ['CE', -39, -7, -36, -3],
  ['RN', -36, -7, -34, -5],
  ['PB', -38, -8, -34, -6],
  ['PE', -41, -10, -34, -7],
  ['AL', -38, -11, -35, -9],
  ['SE', -38, -12, -36, -10],
  ['BA', -46, -18, -37, -10],
  ['GO', -52, -19, -46, -14],
  ['DF', -48.5, -16.5, -47, -15],
  ['MS', -58, -24, -51, -18],
  ['MG', -51, -22, -40, -15],
  ['ES', -41, -21, -39, -18],
  ['RJ', -44, -23.5, -40, -21],
  ['SP', -53, -25, -44, -20],
  ['PR', -54, -27, -48, -23],
  ['SC', -54, -29, -48, -26],
  ['RS', -58, -34, -49, -29],
]

function boxFeature([name, minX, minY, maxX, maxY]: [string, number, number, number, number]): Feature {
  return {
    type: 'Feature',
    properties: { name },
    geometry: {
      type: 'Polygon',
      coordinates: [
        [
          [minX, minY],
          [maxX, minY],
          [maxX, maxY],
          [minX, maxY],
          [minX, minY],
        ],
      ],
    },
  }
}

export const brazilStatesGeoJSON = {
  type: 'FeatureCollection',
  features: stateBoxes.map(boxFeature),
}
