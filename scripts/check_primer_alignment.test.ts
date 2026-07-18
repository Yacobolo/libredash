import {describe, expect, test} from "bun:test";
import {mkdir, mkdtemp, rm, writeFile} from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import {checkPrimerAlignment} from "./check_primer_alignment";

async function withWorkspace(run: (workspace: string) => Promise<void>): Promise<void> {
  const workspace = await mkdtemp(path.join(os.tmpdir(), "primer-alignment-"));

  try {
    await mkdir(path.join(workspace, "docs", "reference", "primer-primitives-css"), {recursive: true});
    await mkdir(path.join(workspace, "site", "static"), {recursive: true});
    await mkdir(path.join(workspace, "site", "web"), {recursive: true});
    await mkdir(path.join(workspace, "static"), {recursive: true});
    await mkdir(path.join(workspace, "web", "components", "example"), {recursive: true});
    await writeFile(path.join(workspace, "site", "static", "site.css"), "");
    await writeFile(path.join(workspace, "site", "web", "site-page.ts"), "");
    await writeFile(path.join(workspace, "static", "app.css"), "");
    await writeFile(
      path.join(workspace, "docs", "reference", "primer-primitives-css", "size.css"),
      ":root { --base-size-4: 0.25rem; --base-size-8: 0.5rem; --control-small-size: 1.75rem; --motion-duration-short: 200ms; }\n",
    );
    await writeFile(
      path.join(workspace, "docs", "reference", "primer-primitives-css", "theme-light.css"),
      [
        ":root {",
        "  --fgColor-accent: #0969da;",
        "  --bgColor-default: #fff;",
        "  --control-bgColor-hover: #eff2f5;",
        "  --button-default-bgColor-rest: #f6f8fa;",
        "  --button-primary-bgColor-rest: #1f883d;",
        "  --bgColor-accent-muted: #ddf4ff;",
        "  --focus-outline: 2px solid #0969da;",
        "  --data-blue-color-muted: #ddf4ff;",
        "  --label-blue-bgColor-rest: #d1f0ff;",
        "  --label-blue-fgColor-rest: #005fcc;",
        "  --label-blue-borderColor: #ffffff00;",
        "}\n",
      ].join("\n"),
    );
    await run(workspace);
  } finally {
    await rm(workspace, {recursive: true, force: true});
  }
}

describe("checkPrimerAlignment", () => {
  test("accepts Primer-backed LibreDash aliases", async () => {
    await withWorkspace(async workspace => {
      await writeFile(
        path.join(workspace, "static", "app.input.css"),
        ":root { --ld-accent: var(--fgColor-accent); --ld-space-control: calc(var(--base-size-8) + var(--base-size-4)); }\n",
      );
      await writeFile(
        path.join(workspace, "web", "components", "example", "good.ts"),
        "import {css} from 'lit';\nexport const styles = css`:host { color: var(--ld-accent); padding: var(--ld-space-control); }`;\n",
      );

      await expect(checkPrimerAlignment({root: workspace})).resolves.toEqual([]);
    });
  });

  test("rejects raw color values and raw design fallbacks in product CSS", async () => {
    await withWorkspace(async workspace => {
      await writeFile(path.join(workspace, "static", "app.input.css"), ":root { --ld-accent: var(--fgColor-accent); }\n");
      await writeFile(
        path.join(workspace, "web", "components", "example", "bad.ts"),
        "import {css} from 'lit';\nexport const styles = css`.button { color: #0969da; background: var(--ld-accent, #0969da); }`;\n",
      );

      const violations = await checkPrimerAlignment({root: workspace});
      expect(violations.map(violation => violation.kind)).toEqual(["raw-color", "raw-color", "raw-var-fallback"]);
    });
  });

  test("rejects undefined design tokens and local Primer namespace extensions", async () => {
    await withWorkspace(async workspace => {
      await writeFile(
        path.join(workspace, "static", "app.input.css"),
        ":root { --base-size-10: 0.625rem; --ld-accent: var(--fgColor-accent); }\n",
      );
      await writeFile(
        path.join(workspace, "web", "components", "example", "bad.ts"),
        "import {css} from 'lit';\nexport const styles = css`:host { transition-duration: var(--motion-duration-fast); color: var(--ld-missing); }`;\n",
      );

      const violations = await checkPrimerAlignment({root: workspace});
      expect(violations.map(violation => violation.kind)).toEqual([
        "local-primer-token",
        "undefined-token",
        "undefined-token",
      ]);
    });
  });

  test("checks site styles and rejects tokens missing from compiled runtime CSS", async () => {
    await withWorkspace(async workspace => {
      await writeFile(
        path.join(workspace, "static", "app.input.css"),
        ":root { --ld-accent: var(--fgColor-accent); --container-site-reading: 32rem; }\n",
      );
      await writeFile(path.join(workspace, "static", "app.css"), ":root { --ld-accent: var(--fgColor-accent); }\n");
      await writeFile(
        path.join(workspace, "site", "static", "site.css"),
        ":root { color: #0969da; } .article { max-width: var(--container-site-reading); }\n",
      );

      const violations = await checkPrimerAlignment({root: workspace});
      expect(violations.map(violation => violation.kind)).toEqual(["raw-color", "runtime-undefined-token"]);
    });
  });

  test("excludes the Datastar inspector from product alignment checks", async () => {
    await withWorkspace(async workspace => {
      await mkdir(path.join(workspace, "web", "components", "inspector"), {recursive: true});
      await writeFile(path.join(workspace, "static", "app.input.css"), ":root { --ld-accent: var(--fgColor-accent); }\n");
      await writeFile(
        path.join(workspace, "web", "components", "inspector", "datastar-inspector.ts"),
        "import {css} from 'lit';\nexport const styles = css`:host { color: #fff; }`;\n",
      );

      await expect(checkPrimerAlignment({root: workspace})).resolves.toEqual([]);
    });
  });

  test("rejects color-mix for standard UI states and data palette asset labels", async () => {
    await withWorkspace(async workspace => {
      await writeFile(
        path.join(workspace, "static", "app.input.css"),
        [
          ":root {",
          "  --ld-bg-hover: color-mix(in srgb, var(--control-bgColor-hover), transparent 20%);",
          "  --ld-asset-dashboard-bg: var(--data-blue-color-muted);",
          "}\n",
        ].join("\n"),
      );
      await writeFile(
        path.join(workspace, "web", "components", "example", "bad.ts"),
        [
          "import {css} from 'lit';",
          "export const styles = css`",
          "  .button:focus-visible { box-shadow: 0 0 0 2px color-mix(in srgb, var(--fgColor-accent), transparent 80%); }",
          "  .chip[aria-pressed='true'] { background: color-mix(in srgb, var(--fgColor-accent), transparent 80%); }",
          "`;",
          "",
        ].join("\n"),
      );

      const violations = await checkPrimerAlignment({root: workspace});
      expect(violations.map(violation => violation.kind).sort()).toEqual([
        "asset-token",
        "standard-state-color-mix",
        "standard-state-color-mix",
        "standard-state-color-mix",
      ].sort());
    });
  });

  test("accepts Primer label tokens and standard state tokens", async () => {
    await withWorkspace(async workspace => {
      await writeFile(
        path.join(workspace, "static", "app.input.css"),
        [
          ":root {",
          "  --ld-bg-hover: var(--control-bgColor-hover);",
          "  --ld-asset-dashboard-bg: var(--label-blue-bgColor-rest);",
          "  --ld-asset-dashboard-accent: var(--label-blue-fgColor-rest);",
          "  --ld-asset-dashboard-border: var(--label-blue-borderColor);",
          "}\n",
        ].join("\n"),
      );
      await writeFile(
        path.join(workspace, "web", "components", "example", "good.ts"),
        [
          "import {css} from 'lit';",
          "export const styles = css`",
          "  .button:focus-visible { outline: var(--focus-outline); }",
          "  .chip[aria-pressed='true'] { background: var(--bgColor-accent-muted); }",
          "`;",
          "",
        ].join("\n"),
      );

      await expect(checkPrimerAlignment({root: workspace})).resolves.toEqual([]);
    });
  });

  test("rejects Primer primary button tokens in product UI", async () => {
    await withWorkspace(async workspace => {
      await writeFile(
        path.join(workspace, "static", "app.input.css"),
        ":root { --ld-button-accent-bg-rest: var(--fgColor-accent); }\n",
      );
      await writeFile(
        path.join(workspace, "web", "components", "example", "bad.ts"),
        "import {css} from 'lit';\nexport const styles = css`.submit { background: var(--button-primary-bgColor-rest); }`;\n",
      );

      const violations = await checkPrimerAlignment({root: workspace});
      expect(violations.map(violation => violation.kind)).toEqual(["primer-primary-button-token"]);
    });
  });

  test("accepts LibreDash accent button aliases", async () => {
    await withWorkspace(async workspace => {
      await writeFile(
        path.join(workspace, "static", "app.input.css"),
        ":root { --ld-button-accent-bg-rest: var(--fgColor-accent); --ld-button-bg-rest: var(--button-default-bgColor-rest); }\n",
      );
      await writeFile(
        path.join(workspace, "web", "components", "example", "good.ts"),
        "import {css} from 'lit';\nexport const styles = css`.submit { background: var(--ld-button-accent-bg-rest); }`;\n",
      );

      await expect(checkPrimerAlignment({root: workspace})).resolves.toEqual([]);
    });
  });

  test("rejects direct styling for standard button selectors", async () => {
    await withWorkspace(async workspace => {
      await writeFile(path.join(workspace, "static", "app.input.css"), ":root { --ld-accent: var(--fgColor-accent); }\n");
      await writeFile(
        path.join(workspace, "web", "components", "example", "bad.ts"),
        [
          "import {css} from 'lit';",
          "export const styles = css`",
          "  .menu button { min-height: var(--control-small-size); border: 0; background: transparent; outline: 0; }",
          "`;",
          "",
        ].join("\n"),
      );

      const violations = await checkPrimerAlignment({root: workspace});
      expect(violations.map(violation => violation.kind)).toEqual(["button-contract"]);
    });
  });

  test("accepts LibreDash invisible button aliases for standard button selectors", async () => {
    await withWorkspace(async workspace => {
      await writeFile(
        path.join(workspace, "static", "app.input.css"),
        ":root { --ld-button-height-sm: var(--base-size-8); --ld-button-invisible-bg-rest: var(--bgColor-default); --ld-button-invisible-border-rest: var(--label-blue-borderColor); }\n",
      );
      await writeFile(
        path.join(workspace, "web", "components", "example", "good.ts"),
        [
          "import {css} from 'lit';",
          "export const styles = css`",
          "  .menu button { min-height: var(--ld-button-height-sm); border: var(--base-size-4) solid var(--ld-button-invisible-border-rest); background: var(--ld-button-invisible-bg-rest); }",
          "`;",
          "",
        ].join("\n"),
      );

      await expect(checkPrimerAlignment({root: workspace})).resolves.toEqual([]);
    });
  });
});
