package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	agentcore "github.com/Yacobolo/leapview/pkg/agent"
)

const (
	CatalogSearchToolName = "catalog_search"
	CatalogListToolName   = "catalog_list"
	CatalogGetToolName    = "catalog_get"

	DefaultCatalogSearchLimit = 10
	MaxCatalogSearchLimit     = 25
	DefaultCatalogListLimit   = 25
	MaxCatalogListLimit       = 50
)

type CatalogType string

const (
	CatalogTypeWorkspace     CatalogType = "workspace"
	CatalogTypeDashboard     CatalogType = "dashboard"
	CatalogTypePage          CatalogType = "page"
	CatalogTypeVisual        CatalogType = "visual"
	CatalogTypeFilter        CatalogType = "filter"
	CatalogTypeSemanticModel CatalogType = "semantic_model"
	CatalogTypeSemanticTable CatalogType = "semantic_table"
	CatalogTypeField         CatalogType = "field"
	CatalogTypeMeasure       CatalogType = "measure"
)

var catalogTypes = map[CatalogType]struct{}{
	CatalogTypeWorkspace:     {},
	CatalogTypeDashboard:     {},
	CatalogTypePage:          {},
	CatalogTypeVisual:        {},
	CatalogTypeFilter:        {},
	CatalogTypeSemanticModel: {},
	CatalogTypeSemanticTable: {},
	CatalogTypeField:         {},
	CatalogTypeMeasure:       {},
}

var catalogChildren = map[CatalogType]map[CatalogType]struct{}{
	CatalogTypeWorkspace: {
		CatalogTypeDashboard:     {},
		CatalogTypeSemanticModel: {},
	},
	CatalogTypeDashboard: {
		CatalogTypePage: {},
	},
	CatalogTypePage: {
		CatalogTypeVisual: {},
		CatalogTypeFilter: {},
	},
	CatalogTypeSemanticModel: {
		CatalogTypeSemanticTable: {},
		CatalogTypeMeasure:       {},
	},
	CatalogTypeSemanticTable: {
		CatalogTypeField: {},
	},
}

type CatalogRef struct {
	WorkspaceID string      `json:"workspaceId"`
	Type        CatalogType `json:"type"`
	ID          string      `json:"id"`
}

type CatalogLocation struct {
	DashboardID string `json:"dashboardId"`
	PageID      string `json:"pageId"`
}

type CatalogHierarchyItem struct {
	Ref  CatalogRef `json:"ref"`
	Name string     `json:"name"`
}

type CatalogWorkspace struct {
	Ref  CatalogRef `json:"ref"`
	Name string     `json:"name"`
}

type CatalogItem struct {
	Ref          CatalogRef             `json:"ref"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	Workspace    CatalogWorkspace       `json:"workspace"`
	Hierarchy    []CatalogHierarchyItem `json:"hierarchy"`
	Locations    []CatalogLocation      `json:"locations,omitempty"`
	Href         string                 `json:"href,omitempty"`
	Capabilities []string               `json:"capabilities"`
}

type CatalogSearchContext struct {
	DashboardID string `json:"dashboardId,omitempty"`
	PageID      string `json:"pageId,omitempty"`
}

type CatalogSearchRequest struct {
	Query        string                `json:"query"`
	Types        []CatalogType         `json:"types,omitempty"`
	WorkspaceIDs []string              `json:"workspaceIds,omitempty"`
	Context      *CatalogSearchContext `json:"context,omitempty"`
	Cursor       string                `json:"cursor,omitempty"`
	Limit        int                   `json:"limit,omitempty"`
}

type CatalogListRequest struct {
	Parent     *CatalogRef   `json:"parent,omitempty"`
	ChildTypes []CatalogType `json:"childTypes,omitempty"`
	Cursor     string        `json:"cursor,omitempty"`
	Limit      int           `json:"limit,omitempty"`
}

type CatalogGetRequest struct {
	Ref      CatalogRef       `json:"ref"`
	Location *CatalogLocation `json:"location,omitempty"`
}

type CatalogPage struct {
	Items      []CatalogItem `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

type CatalogGetResult struct {
	Item    CatalogItem    `json:"item"`
	Details map[string]any `json:"details"`
}

type Catalog interface {
	Search(context.Context, Scope, CatalogSearchRequest) (CatalogPage, error)
	List(context.Context, Scope, CatalogListRequest) (CatalogPage, error)
	Get(context.Context, Scope, CatalogGetRequest) (CatalogGetResult, error)
}

type CatalogError struct {
	Code    string
	Message string
}

func (e *CatalogError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type CatalogProvider struct {
	Catalog Catalog
}

func (p CatalogProvider) Definitions(scope Scope) []agentcore.ToolDefinition {
	return []agentcore.ToolDefinition{
		{
			Name:         CatalogSearchToolName,
			Description:  "Search the complete authorized LeapView BI catalog across workspaces. Use this when you know words from a resource name or description but not its exact location.",
			InputSchema:  catalogSearchInputSchema,
			OutputSchema: catalogPageOutputSchema,
			Effect:       "read",
			Tags:         []string{"catalog", "search"},
			Handler: agentcore.ToolHandlerFunc(func(ctx context.Context, call agentcore.ToolCall) (agentcore.ToolResult, error) {
				var request CatalogSearchRequest
				if err := decodeCatalogArguments(call.Arguments, &request); err != nil {
					return ToolError("invalid_arguments", err.Error()), nil
				}
				request.Query = strings.TrimSpace(request.Query)
				if request.Query == "" {
					return ToolError("invalid_arguments", "query is required"), nil
				}
				if request.Limit == 0 {
					request.Limit = DefaultCatalogSearchLimit
				}
				if err := validateCatalogLimit(request.Limit, MaxCatalogSearchLimit); err != nil {
					return ToolError("invalid_arguments", err.Error()), nil
				}
				if err := validateCatalogTypes(request.Types); err != nil {
					return ToolError("invalid_arguments", err.Error()), nil
				}
				if p.Catalog == nil {
					return ToolError("catalog_unavailable", "catalog service is not configured"), nil
				}
				result, err := p.Catalog.Search(ctx, scope, request)
				if err != nil {
					return catalogToolError("catalog_search_failed", err), nil
				}
				return agentcore.ToolResult{Content: result}, nil
			}),
		},
		{
			Name:         CatalogListToolName,
			Description:  "Browse one deterministic level of the authorized LeapView catalog hierarchy. Omit parent to list workspaces, then pass returned refs to continue browsing.",
			InputSchema:  catalogListInputSchema,
			OutputSchema: catalogPageOutputSchema,
			Effect:       "read",
			Tags:         []string{"catalog", "browse"},
			Handler: agentcore.ToolHandlerFunc(func(ctx context.Context, call agentcore.ToolCall) (agentcore.ToolResult, error) {
				var request CatalogListRequest
				if err := decodeCatalogArguments(call.Arguments, &request); err != nil {
					return ToolError("invalid_arguments", err.Error()), nil
				}
				if request.Limit == 0 {
					request.Limit = DefaultCatalogListLimit
				}
				if err := validateCatalogLimit(request.Limit, MaxCatalogListLimit); err != nil {
					return ToolError("invalid_arguments", err.Error()), nil
				}
				request.ChildTypes = normalizedCatalogTypes(request.ChildTypes)
				if request.Parent == nil {
					if len(request.ChildTypes) > 0 && (len(request.ChildTypes) != 1 || request.ChildTypes[0] != CatalogTypeWorkspace) {
						return ToolError("invalid_arguments", "root can only list workspace children"), nil
					}
				} else {
					*request.Parent = normalizedCatalogRef(*request.Parent)
					if err := validateCatalogRef(*request.Parent); err != nil {
						return ToolError("invalid_arguments", err.Error()), nil
					}
					if err := validateCatalogChildTypes(request.Parent.Type, request.ChildTypes); err != nil {
						return ToolError("invalid_arguments", err.Error()), nil
					}
				}
				if p.Catalog == nil {
					return ToolError("catalog_unavailable", "catalog service is not configured"), nil
				}
				result, err := p.Catalog.List(ctx, scope, request)
				if err != nil {
					return catalogToolError("catalog_list_failed", err), nil
				}
				return agentcore.ToolResult{Content: result}, nil
			}),
		},
		{
			Name:         CatalogGetToolName,
			Description:  "Get the compact definition and type-specific metadata for one exact catalog ref. A dashboard/page location is required when a visual or filter is shared.",
			InputSchema:  catalogGetInputSchema,
			OutputSchema: catalogGetOutputSchema,
			Effect:       "read",
			Tags:         []string{"catalog", "describe"},
			Handler: agentcore.ToolHandlerFunc(func(ctx context.Context, call agentcore.ToolCall) (agentcore.ToolResult, error) {
				var request CatalogGetRequest
				if err := decodeCatalogArguments(call.Arguments, &request); err != nil {
					return ToolError("invalid_arguments", err.Error()), nil
				}
				request.Ref = normalizedCatalogRef(request.Ref)
				if err := validateCatalogRef(request.Ref); err != nil {
					return ToolError("invalid_arguments", err.Error()), nil
				}
				if request.Location != nil {
					request.Location.DashboardID = strings.TrimSpace(request.Location.DashboardID)
					request.Location.PageID = strings.TrimSpace(request.Location.PageID)
					if request.Location.DashboardID == "" || request.Location.PageID == "" {
						return ToolError("invalid_arguments", "location requires dashboardId and pageId"), nil
					}
				}
				if p.Catalog == nil {
					return ToolError("catalog_unavailable", "catalog service is not configured"), nil
				}
				result, err := p.Catalog.Get(ctx, scope, request)
				if err != nil {
					return catalogToolError("catalog_get_failed", err), nil
				}
				return agentcore.ToolResult{Content: result}, nil
			}),
		},
	}
}

func decodeCatalogArguments(arguments json.RawMessage, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("arguments must contain one JSON object")
		}
		return err
	}
	return nil
}

func validateCatalogLimit(limit, maximum int) error {
	if limit < 1 || limit > maximum {
		return fmt.Errorf("limit must be between 1 and %d", maximum)
	}
	return nil
}

func validateCatalogTypes(types []CatalogType) error {
	for _, typ := range types {
		if _, ok := catalogTypes[typ]; !ok {
			return fmt.Errorf("unsupported catalog type %q", typ)
		}
	}
	return nil
}

func validateCatalogRef(ref CatalogRef) error {
	if ref.WorkspaceID == "" {
		return fmt.Errorf("ref.workspaceId is required")
	}
	if _, ok := catalogTypes[ref.Type]; !ok {
		return fmt.Errorf("unsupported catalog type %q", ref.Type)
	}
	if ref.ID == "" {
		return fmt.Errorf("ref.id is required")
	}
	if ref.Type == CatalogTypeWorkspace && ref.ID != ref.WorkspaceID {
		return fmt.Errorf("workspace ref id must equal workspaceId")
	}
	return nil
}

func normalizedCatalogRef(ref CatalogRef) CatalogRef {
	ref.WorkspaceID = strings.TrimSpace(ref.WorkspaceID)
	ref.ID = strings.TrimSpace(ref.ID)
	return ref
}

func normalizedCatalogTypes(types []CatalogType) []CatalogType {
	seen := map[CatalogType]struct{}{}
	out := make([]CatalogType, 0, len(types))
	for _, typ := range types {
		if _, duplicate := seen[typ]; duplicate {
			continue
		}
		seen[typ] = struct{}{}
		out = append(out, typ)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func validateCatalogChildTypes(parent CatalogType, children []CatalogType) error {
	allowed, ok := catalogChildren[parent]
	if !ok {
		return fmt.Errorf("catalog type %q cannot have children", parent)
	}
	for _, child := range children {
		if _, ok := allowed[child]; !ok {
			return fmt.Errorf("catalog type %q cannot list child type %q", parent, child)
		}
	}
	return nil
}

func catalogToolError(fallback string, err error) agentcore.ToolResult {
	var catalogErr *CatalogError
	if errors.As(err, &catalogErr) && strings.TrimSpace(catalogErr.Code) != "" {
		return ToolError(catalogErr.Code, catalogErr.Message)
	}
	return ToolError(fallback, err.Error())
}

var catalogRefSchema = `{
	"type":"object",
	"properties":{
		"workspaceId":{"type":"string","minLength":1},
		"type":{"type":"string","enum":["workspace","dashboard","page","visual","filter","semantic_model","semantic_table","field","measure"]},
		"id":{"type":"string","minLength":1}
	},
	"required":["workspaceId","type","id"],
	"additionalProperties":false
}`

var catalogLocationSchema = `{
	"type":"object",
	"properties":{
		"dashboardId":{"type":"string","minLength":1},
		"pageId":{"type":"string","minLength":1}
	},
	"required":["dashboardId","pageId"],
	"additionalProperties":false
}`

var catalogSearchInputSchema = json.RawMessage(fmt.Sprintf(`{
	"type":"object",
	"properties":{
		"query":{"type":"string","minLength":1},
		"types":{"type":"array","items":{"type":"string","enum":["workspace","dashboard","page","visual","filter","semantic_model","semantic_table","field","measure"]}},
		"workspaceIds":{"type":"array","items":{"type":"string","minLength":1}},
		"context":{"type":"object","properties":{"dashboardId":{"type":"string"},"pageId":{"type":"string"}},"additionalProperties":false},
		"cursor":{"type":"string"},
		"limit":{"type":"integer","minimum":1,"maximum":%d}
	},
	"required":["query"],
	"additionalProperties":false
}`, MaxCatalogSearchLimit))

var catalogListInputSchema = json.RawMessage(fmt.Sprintf(`{
	"type":"object",
	"properties":{
		"parent":%s,
		"childTypes":{"type":"array","items":{"type":"string","enum":["workspace","dashboard","page","visual","filter","semantic_model","semantic_table","field","measure"]}},
		"cursor":{"type":"string"},
		"limit":{"type":"integer","minimum":1,"maximum":%d}
	},
	"additionalProperties":false
}`, catalogRefSchema, MaxCatalogListLimit))

var catalogGetInputSchema = json.RawMessage(fmt.Sprintf(`{
	"type":"object",
	"properties":{"ref":%s,"location":%s},
	"required":["ref"],
	"additionalProperties":false
}`, catalogRefSchema, catalogLocationSchema))

var catalogItemSchema = fmt.Sprintf(`{
	"type":"object",
	"properties":{
		"ref":%s,
		"name":{"type":"string"},
		"description":{"type":"string"},
		"workspace":{"type":"object","properties":{"ref":%s,"name":{"type":"string"}},"required":["ref","name"],"additionalProperties":false},
		"hierarchy":{"type":"array","items":{"type":"object","properties":{"ref":%s,"name":{"type":"string"}},"required":["ref","name"],"additionalProperties":false}},
		"locations":{"type":"array","items":%s},
		"href":{"type":"string"},
		"capabilities":{"type":"array","items":{"type":"string","enum":["catalog_get","catalog_list","query_semantic_model","query_dashboard_visual","query_visual"]}}
	},
	"required":["ref","name","workspace","hierarchy","capabilities"],
	"additionalProperties":false
}`, catalogRefSchema, catalogRefSchema, catalogRefSchema, catalogLocationSchema)

var catalogPageOutputSchema = json.RawMessage(fmt.Sprintf(`{
	"type":"object",
	"properties":{"items":{"type":"array","items":%s},"nextCursor":{"type":"string"}},
	"required":["items"],
	"additionalProperties":false
}`, catalogItemSchema))

var catalogPlacementSchema = `{
	"type":"object",
	"properties":{
		"col":{"type":"integer"},"row":{"type":"integer"},"colSpan":{"type":"integer"},"rowSpan":{"type":"integer"},
		"x":{"type":"number"},"y":{"type":"number"},"width":{"type":"number"},"height":{"type":"number"}
	},
	"additionalProperties":false
}`

var catalogColumnSchema = `{
	"type":"object",
	"properties":{"key":{"type":"string"},"label":{"type":"string"},"role":{"type":"string"},"format":{"type":"string"}},
	"required":["key","label"],
	"additionalProperties":false
}`

var catalogDetailsSchema = `{
	"oneOf":[
		{
			"type":"object",
			"properties":{"type":{"type":"string","enum":["workspace"]},"activeServingStateId":{"type":"string"}},
			"required":["type","activeServingStateId"],
			"additionalProperties":false
		},
		{
			"type":"object",
			"properties":{
				"type":{"type":"string","enum":["dashboard"]},
				"semanticModelRef":` + catalogRefSchema + `,
				"pageCount":{"type":"integer"},
				"visualCount":{"type":"integer"},
				"filterCount":{"type":"integer"}
			},
			"required":["type","semanticModelRef","pageCount","visualCount","filterCount"],
			"additionalProperties":false
		},
		{
			"type":"object",
			"properties":{
				"type":{"type":"string","enum":["page"]},
				"components":{
					"type":"array",
					"items":{
						"type":"object",
						"properties":{
							"id":{"type":"string"},"kind":{"type":"string"},"ref":{"type":"string"},
							"title":{"type":"string"},"description":{"type":"string"},
							"placement":` + catalogPlacementSchema + `,
							"x":{"type":"number"},"y":{"type":"number"},"width":{"type":"number"},"height":{"type":"number"},
							"visualId":{"type":"string"},"filterId":{"type":"string"}
						},
						"required":["id","kind"],
						"additionalProperties":false
					}
				}
			},
			"required":["type","components"],
			"additionalProperties":false
		},
		{
			"type":"object",
			"properties":{
				"type":{"type":"string","enum":["visual"]},
				"visualType":{"type":"string"},"shape":{"type":"string"},"renderer":{"type":"string"},
				"query":{"type":"object","additionalProperties":true},
				"columns":{"type":"array","items":` + catalogColumnSchema + `},
				"placement":` + catalogPlacementSchema + `
			},
			"required":["type","visualType","shape","renderer","query","columns","placement"],
			"additionalProperties":false
		},
		{
			"type":"object",
			"properties":{
				"type":{"type":"string","enum":["filter"]},
				"field":{"type":"string"},
				"configuration":{"type":"object","additionalProperties":true},
				"placement":` + catalogPlacementSchema + `
			},
			"required":["type","field","configuration","placement"],
			"additionalProperties":false
		},
		{
			"type":"object",
			"properties":{
				"type":{"type":"string","enum":["semantic_model"]},
				"semanticTableCount":{"type":"integer"},"fieldCount":{"type":"integer"},
				"measureCount":{"type":"integer"},"dashboardCount":{"type":"integer"},
				"dashboardUsage":{"type":"array","items":` + catalogRefSchema + `}
			},
			"required":["type","semanticTableCount","fieldCount","measureCount","dashboardCount","dashboardUsage"],
			"additionalProperties":false
		},
		{
			"type":"object",
			"properties":{
				"type":{"type":"string","enum":["semantic_table"]},
				"source":{"type":"string"},"sources":{"type":"array","items":{"type":"string"}},
				"grain":{"type":"string"},"primaryKey":{"type":"string"},"keys":{"type":"array","items":{"type":"string"}},
				"fieldCount":{"type":"integer"},"measureCount":{"type":"integer"}
			},
			"required":["type","source","sources","grain","primaryKey","keys","fieldCount","measureCount"],
			"additionalProperties":false
		},
		{
			"type":"object",
			"properties":{
				"type":{"type":"string","enum":["field"]},"kind":{"type":"string"},
				"table":{"type":"string"},"label":{"type":"string"},"dataType":{"type":"string"},
				"grain":{"type":"string"},"timeGrains":{"type":"array","items":{"type":"string"}}
			},
			"required":["type","kind","label","dataType"],
			"additionalProperties":false
		},
		{
			"type":"object",
			"properties":{
				"type":{"type":"string","enum":["measure"]},"kind":{"type":"string"},
				"table":{"type":"string"},"label":{"type":"string"},"aggregation":{"type":"string"},
				"unit":{"type":"string"},"format":{"type":"string"},"hidden":{"type":"boolean"}
			},
			"required":["type","kind","label","unit","format","hidden"],
			"additionalProperties":false
		}
	]
}`

var catalogGetOutputSchema = json.RawMessage(fmt.Sprintf(`{
	"type":"object",
	"properties":{"item":%s,"details":%s},
	"required":["item","details"],
	"additionalProperties":false
}`, catalogItemSchema, catalogDetailsSchema))
