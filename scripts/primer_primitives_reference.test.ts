import {describe, expect, test} from "bun:test";
import {mkdir, mkdtemp, readFile, readdir, rm, writeFile} from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import {generatePrimerPrimitivesReference, parsePrimerPrimitiveTokens} from "./primer_primitives_reference";

describe("parsePrimerPrimitiveTokens", () => {
  test("extracts primitive declarations as token-value pairs", () => {
    const tokens = parsePrimerPrimitiveTokens(`
      @custom-media --viewportRange-wide (min-width: 87.5rem);

      :root {
        --base-size-4: 0.25rem;
        --shadow-resting-small: 0 1px 0 #0000001f; /** comment */
        color: var(--fgColor-default);
      }

      [data-color-mode="dark"] {
        --bgColor-default: #0d1117;
      }
    `);

    expect(tokens).toEqual([
      {token: "--viewportRange-wide", value: "(min-width: 87.5rem)"},
      {token: "--base-size-4", value: "0.25rem"},
      {token: "--shadow-resting-small", value: "0 1px 0 #0000001f"},
      {token: "--bgColor-default", value: "#0d1117"},
    ]);
  });
});

describe("generatePrimerPrimitivesReference", () => {
  test("writes grouped Markdown files with source sections", async () => {
    const workspace = await mkdtemp(path.join(os.tmpdir(), "primer-primitives-reference-"));
    const sourceRoot = path.join(workspace, "source");
    const outputRoot = path.join(workspace, "output");

    try {
      await mkdir(path.join(sourceRoot, "base", "motion"), {recursive: true});
      await mkdir(path.join(sourceRoot, "base", "size"), {recursive: true});
      await mkdir(path.join(sourceRoot, "functional", "spacing"), {recursive: true});
      await mkdir(path.join(sourceRoot, "functional", "themes"), {recursive: true});
      await writeFile(path.join(sourceRoot, "base", "size", "size.css"), ":root { --base-size-4: 0.25rem; }\n");
      await writeFile(path.join(sourceRoot, "base", "motion", "motion.css"), ":root { --base-duration-100: 80ms; }\n");
      await writeFile(path.join(sourceRoot, "functional", "spacing", "space.css"), ":root { --stack-gap-normal: 1rem; }\n");
      await writeFile(path.join(sourceRoot, "functional", "themes", "light.css"), ":root { --bgColor-default: #fff; }\n");
      await writeFile(path.join(sourceRoot, "primitives.css"), "@import './base/size/size.css';\n");

      await generatePrimerPrimitivesReference({sourceRoot, outputRoot});

      const entries = await readdir(outputRoot);
      expect(entries.sort()).toEqual(["motion.md", "size.md", "themes"]);
      expect((await readdir(path.join(outputRoot, "themes"))).sort()).toEqual(["light.md"]);
      expect(await readFile(path.join(outputRoot, "size.md"), "utf8")).toBe(
        [
          "# Size",
          "",
          "Source: `@primer/primitives/dist/css/base/size/size.css`",
          "",
          "--base-size-4: 0.25rem",
          "",
          "Source: `@primer/primitives/dist/css/functional/spacing/space.css`",
          "",
          "--stack-gap-normal: 1rem",
          "",
        ].join("\n"),
      );
      expect(await readFile(path.join(outputRoot, "themes", "light.md"), "utf8")).toBe(
        [
          "# Light Theme",
          "",
          "Source: `@primer/primitives/dist/css/functional/themes/light.css`",
          "",
          "--bgColor-default: #fff",
          "",
        ].join("\n"),
      );
    } finally {
      await rm(workspace, {recursive: true, force: true});
    }
  });
});
