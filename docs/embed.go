// Package docs owns authored and generated Markdown for the public documentation site.
package docs

import "embed"

// Files contains the unified catalog, FTS5 search index, authored articles, generated
// references, downloadable schemas, and API contract.
//
//go:embed *.md *.json *.sqlite3 articles api guides reference visuals
var Files embed.FS
