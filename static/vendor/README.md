# Vendored Browser Dependencies

## Datastar

- File: `datastar-1.0.2.js`
- Version: `v1.0.2`
- Source: `https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js`
- Upstream: `https://github.com/starfederation/datastar`

LeapView serves Datastar locally so page streaming does not depend on a CDN at runtime.

## Model Context Protocol mark

- File: `mcp-mark.svg`
- Source: `docs/favicon.svg` in the official
  [`modelcontextprotocol/modelcontextprotocol`](https://github.com/modelcontextprotocol/modelcontextprotocol)
  repository
- License: MIT

The transparent, `currentColor` variant preserves the official mark geometry
while allowing the public site to apply its Primer-backed foreground tokens in
light and dark themes.

## Integration logos

The public-site build vendors the grayscale SVGs used by DuckDB's
[`#ecosystem`](https://duckdb.org/#ecosystem) diagram. They were extracted from
`_includes/ecosystem_diagram.html` in `duckdb/duckdb-web` revision
`ecc3163f86370638a5eea3961932fd1e16e8def1`.

Hetzner is not present in DuckDB's diagram. Its square glyph comes from
`simple-icons/simple-icons` revision
`a78ea66ee5d93dfd2ab07458de8dff522dcf3d91` and uses the same grayscale
treatment as the DuckDB-derived assets.

The logos identify third-party compatibility and do not imply endorsement.
