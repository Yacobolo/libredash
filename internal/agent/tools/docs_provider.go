package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	productdocs "github.com/Yacobolo/leapview/internal/productdocs"
	agentcore "github.com/Yacobolo/leapview/pkg/agent"
)

const (
	DocsSearchToolName = "docs_search"
	DocsReadToolName   = "docs_read"
)

type Documentation interface {
	Search(context.Context, productdocs.SearchRequest) (productdocs.SearchResult, error)
	Read(context.Context, productdocs.ReadRequest) (productdocs.ReadResult, error)
}

type DocsProvider struct {
	Documentation Documentation
}

func (p DocsProvider) Definitions() []agentcore.ToolDefinition {
	return []agentcore.ToolDefinition{
		{
			Name:        DocsSearchToolName,
			Description: "Search LeapView's version-matched product documentation. Returns ranked, bounded matches with stable document IDs and excerpts. Use the optional path prefix to narrow broad searches.",
			InputSchema: json.RawMessage(fmt.Sprintf(`{
				"type":"object",
				"properties":{
					"query":{"type":"string","minLength":1,"description":"Words or phrases to find in LeapView documentation."},
					"path":{"type":"string","description":"Optional documentation path prefix, such as guides/build, api, cli, or visuals."},
					"limit":{"type":"integer","minimum":1,"maximum":%d,"description":"Maximum matches to return; defaults to %d."}
				},
				"required":["query"],
				"additionalProperties":false
			}`, productdocs.MaxSearchLimit, productdocs.DefaultSearchLimit)),
			OutputSchema: docsSearchOutputSchema,
			Effect:       "read",
			Tags:         []string{"documentation", "search"},
			Handler: agentcore.ToolHandlerFunc(func(ctx context.Context, call agentcore.ToolCall) (agentcore.ToolResult, error) {
				return p.search(ctx, call.Arguments), nil
			}),
		},
		{
			Name:        DocsReadToolName,
			Description: "Read a bounded line window from one LeapView document returned by docs_search. Continue with nextOffset when truncated is true.",
			InputSchema: json.RawMessage(fmt.Sprintf(`{
				"type":"object",
				"properties":{
					"id":{"type":"string","minLength":1,"description":"Stable doc:<path> ID returned by docs_search."},
					"offset":{"type":"integer","minimum":1,"description":"First line to read, starting at 1; defaults to 1."},
					"limit":{"type":"integer","minimum":1,"maximum":%d,"description":"Maximum lines to read; defaults to %d and is also byte-bounded."}
				},
				"required":["id"],
				"additionalProperties":false
			}`, productdocs.MaxReadLimit, productdocs.DefaultReadLimit)),
			OutputSchema: docsReadOutputSchema,
			Effect:       "read",
			Tags:         []string{"documentation"},
			Handler: agentcore.ToolHandlerFunc(func(ctx context.Context, call agentcore.ToolCall) (agentcore.ToolResult, error) {
				return p.read(ctx, call.Arguments), nil
			}),
		},
	}
}

func (p DocsProvider) search(ctx context.Context, arguments json.RawMessage) agentcore.ToolResult {
	var request productdocs.SearchRequest
	if err := json.Unmarshal(arguments, &request); err != nil {
		return ToolError("invalid_arguments", err.Error())
	}
	if strings.TrimSpace(request.Query) == "" {
		return ToolError("invalid_arguments", "query is required")
	}
	if p.Documentation == nil {
		return ToolError("documentation_unavailable", "documentation service is not configured")
	}
	result, err := p.Documentation.Search(ctx, request)
	if err != nil {
		return documentationToolError("docs_search_failed", err)
	}
	return agentcore.ToolResult{Content: result}
}

func (p DocsProvider) read(ctx context.Context, arguments json.RawMessage) agentcore.ToolResult {
	var request productdocs.ReadRequest
	if err := json.Unmarshal(arguments, &request); err != nil {
		return ToolError("invalid_arguments", err.Error())
	}
	if strings.TrimSpace(request.ID) == "" {
		return ToolError("invalid_arguments", "id is required")
	}
	if p.Documentation == nil {
		return ToolError("documentation_unavailable", "documentation service is not configured")
	}
	result, err := p.Documentation.Read(ctx, request)
	if err != nil {
		return documentationToolError("docs_read_failed", err)
	}
	return agentcore.ToolResult{Content: result}
}

func documentationToolError(fallback string, err error) agentcore.ToolResult {
	switch {
	case errors.Is(err, productdocs.ErrInvalid):
		return ToolError("invalid_arguments", err.Error())
	case errors.Is(err, productdocs.ErrNotFound):
		return ToolError("documentation_not_found", err.Error())
	default:
		return ToolError(fallback, err.Error())
	}
}

var docsSearchOutputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"query":{"type":"string"},
		"path":{"type":"string"},
		"matches":{
			"type":"array",
			"items":{
				"type":"object",
				"properties":{
					"id":{"type":"string"},
					"path":{"type":"string"},
					"title":{"type":"string"},
					"summary":{"type":"string"},
					"url":{"type":"string"},
					"excerpt":{"type":"string"}
				},
				"required":["id","path","title","summary","url","excerpt"],
				"additionalProperties":false
			}
		},
		"truncated":{"type":"boolean"}
	},
	"required":["query","matches","truncated"],
	"additionalProperties":false
}`)

var docsReadOutputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"id":{"type":"string"},
		"path":{"type":"string"},
		"title":{"type":"string"},
		"url":{"type":"string"},
		"content":{"type":"string"},
		"lineStart":{"type":"integer"},
		"lineEnd":{"type":"integer"},
		"totalLines":{"type":"integer"},
		"nextOffset":{"type":"integer"},
		"truncated":{"type":"boolean"}
	},
	"required":["id","path","title","url","content","lineStart","lineEnd","totalLines","truncated"],
	"additionalProperties":false
}`)
