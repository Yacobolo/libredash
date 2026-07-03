# LibreDash Style Guide

## CSS Architecture Decision

LibreDash uses a locality-first styling model:

- Primer primitives are the root design token source.
- `static/app.input.css` imports Primer primitives, imports Tailwind, defines the curated Tailwind theme, and keeps only minimal document defaults.
- Gomponents-rendered document shells and top-level custom element mounts use token-backed Tailwind utility classes directly.
- Lit web components own their local styles and consume Primer or LibreDash CSS variables directly.
- LibreDash does not keep custom global component classes for ordinary product UI.

## Token Source And Tailwind

Primer primitive CSS imports provide the foundation for color, typography, spacing, sizing, radius, border, breakpoint, viewport, and motion values.

Tailwind v4 is configured CSS-first through `@theme` and `@theme inline` in `static/app.input.css`. Utilities should resolve to Primer variables or to LibreDash semantic aliases that themselves resolve to Primer variables.

Add a token mapping before using a new design value. Do not define local tokens in Primer namespaces such as `--base-size-*`; use `--ld-*` aliases backed by Primer primitives instead.

Product UI must not use raw color values or raw design fallbacks in CSS. Raw CSS values are allowed only for non-design runtime geometry such as `0`, `100%`, `minmax(0, 1fr)`, canvas dimensions derived from data, table virtualization offsets, and SVG stroke math.

## Light DOM And Gomponents

Gomponents owns document shells, assets, Datastar roots, and explicit top-level custom element mounting. It should not compose product UI internals such as app shells, report canvases, filter docks, visual frames, sidebars, catalog cards, metric panels, or table regions.

Behavior hooks should use `data-*` attributes. Do not use a class name as both a style hook and behavior hook.

Use Tailwind built-ins such as `sr-only` rather than custom global helpers.

## Web Components

Shadow DOM components should use local Lit CSS and read variables directly with `var(...)`. Do not inject Tailwind utilities into Shadow DOM.

Light DOM web components may include a local `<style>` scoped under their custom element when third-party generated DOM requires light DOM styling. This is appropriate for React Flow components because the React Flow stylesheet targets generated light DOM.

## Global CSS Boundary

`static/app.input.css` may contain:

- Primer primitive imports.
- `@import 'tailwindcss' source(none);`
- Tailwind `@source` declarations.
- `@theme` and `@theme inline` token mappings.
- Minimal base selectors for document defaults.

It should not contain LibreDash global product selectors or compatibility aliases for old component class names.

## Third-Party Styling

DaisyUI and the runtime Tailwind browser CDN are not product dependencies. If a development-only tool needs special styling, keep it isolated or migrate it to token-backed utilities.

React Flow extracted CSS can remain as a page-specific asset where required, but LibreDash-specific React Flow overrides should live with the owning web component.

## Verification

For styling migrations, run:

- `bun run test:primer-primitives`
- `bun run test:primer-alignment`
- `bun run build:css`
- `bun run build`
- `task test`

Also inspect `static/app.css` or search sources to confirm removed custom product selectors did not return.
