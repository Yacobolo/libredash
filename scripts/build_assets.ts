type BuildOptions = Parameters<typeof Bun.build>[0]

type AssetBuild = {
  label: string
  clean: string[]
  options: BuildOptions
}

const builds: AssetBuild[] = [
  single('app-shell', 'web/components/app/app-shell.ts', 'static/app-shell.js'),
  single('catalog-page', 'web/components/app/catalog-page.ts', 'static/catalog-page.js'),
  split('dashboard-page', 'web/components/dashboard/dashboard-page.ts', 'static', 'dashboard-page.js', 'chunks/dashboard-[name]-[hash].[ext]'),
  single('workspace-page', 'web/components/workspace/workspace-page.ts', 'static/workspace-page.js'),
  single('data-explorer', 'web/components/data/data-explorer.ts', 'static/data-explorer.js'),
  split('chat-page', 'web/components/chat/chat-page.ts', 'static', 'chat-page.js', 'chunks/chat-[name]-[hash].[ext]'),
  splitByName('admin-page', 'web/components/admin/admin-page.ts', 'static', 'chunks/admin-[name]-[hash].[ext]'),
  single('login-page', 'web/components/login/login-page.ts', 'static/login-page.js'),
  single('monaco-editor-worker', 'web/components/shared/monaco-editor-worker.ts', 'static/monaco-editor-worker.js'),
  single('url-sync', 'web/components/dashboard/filters/url-sync.ts', 'static/url-sync.js'),
  single('datastar-inspector', 'web/components/inspector/index.ts', 'static/datastar-inspector.js'),
  single('topology-background', 'web/components/login/topology-background.ts', 'static/topology-background.js'),
  {
    label: 'semantic-model-graph',
    clean: ['static/semantic-model-graph.js', 'static/semantic-model-graph.css'],
    options: {
      entrypoints: ['web/components/shared/semantic-model-graph.ts'],
      target: 'browser',
      format: 'esm',
      outdir: 'static',
      naming: { entry: 'semantic-model-graph.[ext]' },
    },
  },
  {
    label: 'asset-lineage-graph',
    clean: ['static/asset-lineage-graph.js', 'static/asset-lineage-graph.css'],
    options: {
      entrypoints: ['web/components/shared/asset-lineage-graph.ts'],
      target: 'browser',
      format: 'esm',
      outdir: 'static',
      naming: { entry: 'asset-lineage-graph.[ext]' },
    },
  },
]

for (const build of builds) {
  await runBuild(build)
}
await Bun.write('static/monaco-editor-css.css', Bun.file('static/admin-page.css'))

function single(label: string, entrypoint: string, outputPath: string): AssetBuild {
  const output = outputParts(outputPath)
  return {
    label,
    clean: [outputPath],
    options: {
      entrypoints: [entrypoint],
      target: 'browser',
      format: 'esm',
      outdir: output.dir,
      naming: { entry: output.name },
    },
  }
}

function split(label: string, entrypoint: string, outdir: string, entryName: string, chunkName: string): AssetBuild {
  const chunkPrefix = chunkName.slice(0, chunkName.indexOf('['))
  return {
    label,
    clean: [`${outdir}/${entryName}`, `${outdir}/${chunkPrefix}*.js`, `${outdir}/chunk-*.js`],
    options: {
      entrypoints: [entrypoint],
      target: 'browser',
      format: 'esm',
      splitting: true,
      outdir,
      naming: { entry: entryName, chunk: chunkName },
    },
  }
}

function splitByName(label: string, entrypoint: string, outdir: string, chunkName: string): AssetBuild {
  const name = entrypointName(entrypoint)
  const chunkPrefix = chunkName.slice(0, chunkName.indexOf('['))
  return {
    label,
    clean: [`${outdir}/${name}.js`, `${outdir}/${chunkPrefix}*.js`, `${outdir}/chunk-*.js`],
    options: {
      entrypoints: [entrypoint],
      target: 'browser',
      format: 'esm',
      splitting: true,
      outdir,
      naming: { entry: '[name].[ext]', chunk: chunkName },
    },
  }
}

async function runBuild(build: AssetBuild): Promise<void> {
  await cleanPaths(build.clean)
  const result = await Bun.build(build.options)
  for (const log of result.logs) {
    console.error(log)
  }
  if (!result.success) {
    throw new Error(`failed to build ${build.label}`)
  }
}

async function cleanPaths(paths: string[]): Promise<void> {
  await Promise.all(paths.map((path) => removePath(path)))
}

async function removePath(path: string): Promise<void> {
  const glob = new Bun.Glob(path)
  let removed = false
  for await (const match of glob.scan({ cwd: '.', dot: true, onlyFiles: false })) {
    await Bun.$`rm -rf ${match}`.quiet()
    removed = true
  }
  if (!removed && !path.includes('*')) {
    await Bun.$`rm -rf ${path}`.quiet()
  }
}

function outputParts(path: string): { dir: string; name: string } {
  const slash = path.lastIndexOf('/')
  if (slash < 0) {
    return { dir: '.', name: path }
  }
  return { dir: path.slice(0, slash), name: path.slice(slash + 1) }
}

function entrypointName(path: string): string {
  const slash = path.lastIndexOf('/')
  const name = slash < 0 ? path : path.slice(slash + 1)
  return name.replace(/\.[^.]+$/, '')
}
