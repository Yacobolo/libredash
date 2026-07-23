import Ajv2020 from 'ajv/dist/2020'
import addFormats from 'ajv-formats'
import standaloneCode from 'ajv/dist/standalone'
import { mkdir } from 'node:fs/promises'
import { dirname } from 'node:path'

const schemaPath = 'api/gen/visualization.schema.json'
const contractPath = 'api/visualization/main.tsp'
const outputPath = 'web/generated/visualization/validate.ts'
const versionOutputPath = 'web/generated/visualization/schema-version.ts'
const goVersionOutputPath = 'internal/visualization/ir/schema_version.gen.go'
await Promise.all([
  mkdir(dirname(outputPath), { recursive: true }),
  mkdir(dirname(goVersionOutputPath), { recursive: true }),
])
const document = await Bun.file(schemaPath).json()
const contract = await Bun.file(contractPath).text()
const envelope = contract.match(/model\s+VisualizationEnvelope\s*\{[\s\S]*?`schemaVersion`:\s*(\d+);/)
if (!envelope) throw new Error(`could not resolve VisualizationEnvelope.schemaVersion from ${contractPath}`)
const schemaVersion = Number(envelope[1])
if (!Number.isSafeInteger(schemaVersion) || schemaVersion < 1) throw new Error(`invalid visualization schema version ${JSON.stringify(envelope[1])}`)
document.$defs.VisualizationEnvelope.properties.schemaVersion.const = schemaVersion
await Bun.write(schemaPath, `${JSON.stringify(document, null, 2)}\n`)
const schema = { ...document, $ref: '#/$defs/VisualizationEnvelope' }
const ajv = new Ajv2020({ allErrors: true, code: { source: true, esm: true }, strict: true })
addFormats(ajv)
ajv.addKeyword('x-apigen-contracts')
ajv.addKeyword('x-leapview-contract-role')
const validate = ajv.compile(schema)
const source = standaloneCode(ajv, validate)
await Bun.write(outputPath, `// Code generated from api/visualization/main.tsp. DO NOT EDIT.\n// @ts-nocheck\n${source}\n`)
await Bun.write(versionOutputPath, `// Code generated from api/visualization/main.tsp. DO NOT EDIT.\nexport const currentVisualizationSchemaVersion = ${schemaVersion} as const\n`)
await Bun.write(goVersionOutputPath, `// Code generated from api/visualization/main.tsp. DO NOT EDIT.\n\npackage ir\n\nconst CurrentSchemaVersion int32 = ${schemaVersion}\n`)
