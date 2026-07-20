import Ajv2020 from 'ajv/dist/2020'
import addFormats from 'ajv-formats'
import standaloneCode from 'ajv/dist/standalone'
import { mkdir } from 'node:fs/promises'
import { dirname } from 'node:path'

const schemaPath = 'api/gen/visualization.schema.json'
const outputPath = 'web/generated/visualization/validate.ts'
await mkdir(dirname(outputPath), { recursive: true })
const document = await Bun.file(schemaPath).json()
document.$defs.VisualizationEnvelope.properties.schemaVersion.const = 1
await Bun.write(schemaPath, `${JSON.stringify(document, null, 2)}\n`)
const schema = { ...document, $ref: '#/$defs/VisualizationEnvelope' }
const ajv = new Ajv2020({ allErrors: true, code: { source: true, esm: true }, strict: true })
addFormats(ajv)
ajv.addKeyword('x-apigen-contracts')
ajv.addKeyword('x-libredash-contract-role')
const validate = ajv.compile(schema)
const source = standaloneCode(ajv, validate)
await Bun.write(outputPath, `// Code generated from api/visualization/main.tsp. DO NOT EDIT.\n// @ts-nocheck\n${source}\n`)
