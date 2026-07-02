import {mkdir, readdir, readFile, rm, writeFile} from "node:fs/promises";
import path from "node:path";

export type PrimerPrimitiveToken = {
  token: string;
  value: string;
};

type PrimerPrimitiveSource = {
  sourcePath: string;
  relativeSource: string;
};

type PrimerPrimitiveReference = {
  title: string;
  outputPath: string;
  sources: PrimerPrimitiveSource[];
};

export function parsePrimerPrimitiveTokens(css: string): PrimerPrimitiveToken[] {
  const withoutComments = css.replace(/\/\*[\s\S]*?\*\//g, "");
  const declarationPattern =
    /@custom-media\s+(--[-_a-zA-Z0-9]+)\s+([^;]+);|(?:^|[{\n;])\s*(--[-_a-zA-Z0-9]+)\s*:\s*([^;{}]+);/g;
  const tokens: PrimerPrimitiveToken[] = [];

  for (const match of withoutComments.matchAll(declarationPattern)) {
    tokens.push({
      token: match[1] ?? match[3],
      value: (match[2] ?? match[4]).trim().replace(/\s+/g, " "),
    });
  }

  return tokens;
}

async function listCssSources(root: string): Promise<string[]> {
  const entries = await readdir(root, {withFileTypes: true});
  const files = await Promise.all(
    entries.map(async entry => {
      const fullPath = path.join(root, entry.name);

      if (entry.isDirectory()) {
        return listCssSources(fullPath);
      }

      if (entry.isFile() && entry.name.endsWith(".css") && entry.name !== "primitives.css") {
        return [fullPath];
      }

      return [];
    }),
  );

  return files.flat().sort((left, right) => left.localeCompare(right));
}

function renderTokens(tokens: PrimerPrimitiveToken[]): string {
  if (tokens.length === 0) {
    return "";
  }

  return `${tokens.map(({token, value}) => `${token}: ${value}`).join("\n")}\n`;
}

function renderReference(reference: PrimerPrimitiveReference): Promise<string> {
  return Promise.all(
    reference.sources.map(async source => {
      const css = await readFile(source.sourcePath, "utf8");
      const tokens = parsePrimerPrimitiveTokens(css);
      const primitivePath = `@primer/primitives/dist/css/${source.relativeSource.split(path.sep).join("/")}`;

      return `Source: \`${primitivePath}\`\n\n${renderTokens(tokens)}`.trimEnd();
    }),
  ).then(sections => `# ${reference.title}\n\n${sections.join("\n\n")}\n`);
}

function titleFromThemeFile(relativeSource: string): string {
  const themeName = path.basename(relativeSource, ".css");
  const title = themeName
    .split("-")
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");

  return `${title} Theme`;
}

function referenceGroups(sources: PrimerPrimitiveSource[]): PrimerPrimitiveReference[] {
  const byRelativeSource = new Map(sources.map(source => [source.relativeSource.split(path.sep).join("/"), source]));
  const source = (relativeSource: string) => byRelativeSource.get(relativeSource);
  const pick = (relativeSources: string[]) => relativeSources.map(relativeSource => source(relativeSource)).filter(Boolean);
  const themes = sources
    .filter(source => source.relativeSource.split(path.sep).join("/").startsWith("functional/themes/"))
    .map(source => {
      const themeName = path.basename(source.relativeSource, ".css");

      return {
        title: titleFromThemeFile(source.relativeSource),
        outputPath: `themes/${themeName}.md`,
        sources: [source],
      };
    });

  return [
    {
      title: "Motion",
      outputPath: "motion.md",
      sources: pick(["base/motion/motion.css", "functional/motion/motion.css"]),
    },
    {
      title: "Size",
      outputPath: "size.md",
      sources: pick([
        "base/size/size.css",
        "base/size/z-index.css",
        "functional/size/border.css",
        "functional/size/breakpoints.css",
        "functional/size/radius.css",
        "functional/size/size-coarse.css",
        "functional/size/size-fine.css",
        "functional/size/size.css",
        "functional/size/viewport.css",
        "functional/size/z-index.css",
        "functional/spacing/space.css",
      ]),
    },
    {
      title: "Typography",
      outputPath: "typography.md",
      sources: pick(["base/typography/typography.css", "functional/typography/typography.css"]),
    },
    ...themes.sort((left, right) => left.outputPath.localeCompare(right.outputPath)),
  ].filter(reference => reference.sources.length > 0);
}

export async function generatePrimerPrimitivesReference(options?: {
  sourceRoot?: string;
  outputRoot?: string;
}): Promise<string[]> {
  const sourceRoot =
    options?.sourceRoot ?? path.join(process.cwd(), "node_modules", "@primer", "primitives", "dist", "css");
  const outputRoot = options?.outputRoot ?? path.join(process.cwd(), "docs", "reference", "primer-primitives");
  const sources = (await listCssSources(sourceRoot)).map(sourcePath => {
    const relativeSource = path.relative(sourceRoot, sourcePath);

    return {
      sourcePath,
      relativeSource,
    };
  });
  const references = referenceGroups(sources);
  const writtenFiles: string[] = [];

  await rm(outputRoot, {recursive: true, force: true});
  await mkdir(outputRoot, {recursive: true});

  for (const reference of references) {
    const outputPath = path.join(outputRoot, reference.outputPath);
    const markdown = await renderReference(reference);

    await mkdir(path.dirname(outputPath), {recursive: true});
    await writeFile(outputPath, markdown, "utf8");
    writtenFiles.push(outputPath);
  }

  return writtenFiles;
}

if (import.meta.main) {
  const files = await generatePrimerPrimitivesReference();
  console.log(`Generated ${files.length} Primer primitive reference files.`);
}
