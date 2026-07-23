// Package docsearch builds and queries the documentation site's immutable FTS5 index.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Yacobolo/leapview/internal/productdocs"
	_ "modernc.org/sqlite"
	"modernc.org/sqlite/vfs"
)

const Filename = "search-index.sqlite3"

const schema = `CREATE VIRTUAL TABLE search_documents USING fts5(
	slug UNINDEXED,
	title,
	summary,
	section,
	category,
	body,
	generated UNINDEXED,
	tokenize = 'unicode61 remove_diacritics 2',
	prefix = '2 3 4'
)`

// Document is the build-time representation stored in the search index.
type Document struct {
	Slug      string
	Title     string
	Summary   string
	Section   string
	Category  string
	Body      string
	Generated bool
}

// Build writes a complete immutable search index to path.
func Build(path string, documents []Document) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create search index directory: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace search index: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()

	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		return fmt.Errorf("open search index: %w", err)
	}
	defer func() {
		if closeErr := database.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close search index: %w", closeErr)
		}
	}()
	database.SetMaxOpenConns(1)

	if _, err := database.Exec(`PRAGMA journal_mode = OFF; PRAGMA synchronous = OFF; PRAGMA page_size = 4096;`); err != nil {
		return fmt.Errorf("configure search index: %w", err)
	}
	if _, err := database.Exec(schema); err != nil {
		return fmt.Errorf("create search index: %w", err)
	}

	transaction, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin search index transaction: %w", err)
	}
	statement, err := transaction.Prepare(`INSERT INTO search_documents(slug, title, summary, section, category, body, generated) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = transaction.Rollback()
		return fmt.Errorf("prepare search document insert: %w", err)
	}
	for _, document := range documents {
		if _, err := statement.Exec(document.Slug, document.Title, document.Summary, document.Section, document.Category, document.Body, document.Generated); err != nil {
			_ = statement.Close()
			_ = transaction.Rollback()
			return fmt.Errorf("index documentation %q: %w", document.Slug, err)
		}
	}
	if err := statement.Close(); err != nil {
		_ = transaction.Rollback()
		return fmt.Errorf("close search document insert: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit search index: %w", err)
	}
	if _, err := database.Exec(`INSERT INTO search_documents(search_documents) VALUES ('optimize'); VACUUM;`); err != nil {
		return fmt.Errorf("optimize search index: %w", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("set search index permissions: %w", err)
	}
	return nil
}

// Index is a read-only FTS5 database backed directly by an fs.FS.
type Index struct {
	database *sql.DB
	vfs      *vfs.FS
}

// Open registers fsys as a read-only SQLite VFS and opens filename from it.
func Open(fsys fs.FS, filename string) (*Index, error) {
	vfsName, filesystem, err := vfs.New(fsys)
	if err != nil {
		return nil, fmt.Errorf("register embedded search index: %w", err)
	}
	dsn := "file:" + url.PathEscape(filename) + "?mode=ro&immutable=1&vfs=" + url.QueryEscape(vfsName)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = filesystem.Close()
		return nil, fmt.Errorf("open embedded search index: %w", err)
	}
	database.SetMaxOpenConns(4)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		_ = filesystem.Close()
		return nil, fmt.Errorf("read embedded search index: %w", err)
	}
	return &Index{database: database, vfs: filesystem}, nil
}

// Close releases the database and its registered VFS.
func (index *Index) Close() error {
	databaseErr := index.database.Close()
	vfsErr := index.vfs.Close()
	if databaseErr != nil {
		return databaseErr
	}
	return vfsErr
}

// Count returns the number of indexed documents.
func (index *Index) Count(ctx context.Context) (int, error) {
	var count int
	err := index.database.QueryRowContext(ctx, `SELECT count(*) FROM search_documents`).Scan(&count)
	return count, err
}

// Slugs returns every indexed slug in source order.
func (index *Index) Slugs(ctx context.Context) ([]string, error) {
	rows, err := index.database.QueryContext(ctx, `SELECT slug FROM search_documents ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		slugs = append(slugs, slug)
	}
	return slugs, rows.Err()
}

// Search returns ranked matches. User input is compiled into an FTS expression
// instead of being passed through as FTS syntax.
func (index *Index) Search(ctx context.Context, query string, limit int) ([]productdocs.SearchMatch, error) {
	expression := matchExpression(query)
	if expression == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := index.database.QueryContext(ctx, `
		SELECT slug, snippet(search_documents, 5, '', '', ' … ', 24)
		FROM search_documents
		WHERE search_documents MATCH ?
		ORDER BY
			CASE WHEN lower(title) = lower(?) THEN 0 ELSE 1 END,
			CAST(generated AS INTEGER),
			bm25(search_documents, 0, 12, 5, 1, 1, 1, 0),
			title COLLATE NOCASE
		LIMIT ?`, expression, strings.TrimSpace(query), limit)
	if err != nil {
		return nil, fmt.Errorf("query documentation search index: %w", err)
	}
	defer rows.Close()
	results := make([]productdocs.SearchMatch, 0)
	for rows.Next() {
		var result productdocs.SearchMatch
		if err := rows.Scan(&result.Slug, &result.Excerpt); err != nil {
			return nil, fmt.Errorf("read documentation search result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate documentation search results: %w", err)
	}
	return results, nil
}

func matchExpression(query string) string {
	type segment struct {
		text   string
		phrase bool
	}
	query = strings.ToLower(strings.TrimSpace(query))
	segments := make([]segment, 0)
	var current strings.Builder
	phrase := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		segments = append(segments, segment{text: current.String(), phrase: phrase})
		current.Reset()
	}
	for _, character := range query {
		if character == '"' {
			flush()
			phrase = !phrase
			continue
		}
		current.WriteRune(character)
	}
	flush()

	expressions := make([]string, 0)
	for _, segment := range segments {
		terms := searchTerms(segment.text)
		if len(terms) == 0 {
			continue
		}
		if segment.phrase && len(terms) > 1 {
			expressions = append(expressions, `"`+strings.Join(terms, " ")+`"`)
			continue
		}
		for _, term := range terms {
			expressions = append(expressions, `"`+term+`"*`)
		}
	}
	return strings.Join(expressions, " AND ")
}

func searchTerms(value string) []string {
	return strings.FieldsFunc(value, func(character rune) bool {
		return character != '_' && !unicode.IsLetter(character) && !unicode.IsNumber(character)
	})
}
