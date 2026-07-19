package contracts

#Identifier: =~"^[A-Za-z_][A-Za-z0-9_]*$"
#ObjectID:   =~"^[A-Za-z_][A-Za-z0-9_-]*$"
#ResourceID: =~"^[A-Za-z_][A-Za-z0-9_.-]*$"
#FieldRef:   =~"^[A-Za-z_][A-Za-z0-9_]*\\.[A-Za-z_][A-Za-z0-9_]*$"
#AnyObject: {
	[string]: _
}

#NoCredentials: close({
	provider!: "none"
})

#EnvCredentials: close({
	provider!: "env"
	secret!:   string
})

#AmbientCredentials: close({
	provider!:    "ambient"
	region?:      string
	endpoint?:    string
	accountName?: string
})

#APIVersion: "libredash.dev/v1"

#Metadata: close({
	name!:        #ResourceID
	workspace?:   #ResourceID
	title?:       string
	description?: string
	owner?:       string
	tags?: [...string]
})

#IncludeList: close({
	include!: [...string]
})

#Project: close({
	apiVersion!: #APIVersion
	kind!:       "Project"
	metadata!:   #Metadata
	spec!: close({
		connections!: #IncludeList
		sources!:     #IncludeList
		workspaces!:  #IncludeList
	})
})

#ConnectionResource: close({
	apiVersion!: #APIVersion
	kind!:       "Connection"
	metadata!:   #Metadata
	spec!:        #Connection
})

#SourceResource: close({
	apiVersion!: #APIVersion
	kind!:       "Source"
	metadata!:   #Metadata
	spec!:        #Source
})

#WorkspaceResource: close({
	apiVersion!: #APIVersion
	kind!:       "Workspace"
	metadata!:   #Metadata
	spec!: close({
		uses!: close({
			sources!: [...#ResourceID]
		})
		models!:         #IncludeList
		semanticModels!: #IncludeList
		dashboards!:     #IncludeList
		access!:         #IncludeList
		agentPolicy!:    #IncludeList
		refreshPipelines?: #IncludeList
	})
})

#WorkspaceGroupResource: close({
	apiVersion!: #APIVersion
	kind!:       "WorkspaceGroup"
	metadata!:   #Metadata
	spec!: close({
		description?: string
		members?: [...close({
			principalId?: #ResourceID
			email?:       string
			displayName?: string
		})]
	})
})

#WorkspaceRoleBindingResource: close({
	apiVersion!: #APIVersion
	kind!:       "WorkspaceRoleBinding"
	metadata!:   #Metadata
	spec!: close({
		role!: "owner" | "admin" | "deployer" | "contributor" | "editor" | "member" | "viewer" | "platform_admin"
		subject!: close({
			kind!:        "principal" | "group" | "service_principal"
			principalId?: #ResourceID
			email?:       string
			displayName?: string
			group?:       #ResourceID
		})
	})
})

#SecurableObjectRef: close({
	type!: "workspace" | "dashboard" | "semantic_model" | "source" | "model_table" | "agent_policy" | "dataset" | "table" | "column"
	id?:   string
})

#Privilege: "USE_WORKSPACE" | "VIEW_ITEM" | "EDIT_ITEM" | "MANAGE_ITEM" | "QUERY_DATA" | "PREVIEW_DATA" | "REFRESH_DATA" | "DEPLOY" | "ACTIVATE_DEPLOYMENT" | "USE_AGENT" | "VIEW_AGENT" | "MANAGE_GRANTS" | "VIEW_AUDIT" | "MANAGE_WORKSPACE" | "MANAGE_PLATFORM"

#AccessSubject: close({
	kind!:        "principal" | "group" | "service_principal"
	principalId?: #ResourceID
	email?:       string
	displayName?: string
	group?:       #ResourceID
})

#GrantResource: close({
	apiVersion!: #APIVersion
	kind!:       "Grant"
	metadata!:   #Metadata
	spec!: close({
		object!:    #SecurableObjectRef
		subject!:   #AccessSubject
		privilege!: #Privilege
	})
})

#DataPolicyResource: close({
	apiVersion!: #APIVersion
	kind!:       "DataPolicy"
	metadata!:   #Metadata
	spec!: close({
		object!:     #SecurableObjectRef
		subject?:    #AccessSubject
		policyType!: "row_filter" | "column_mask"
		expression!: #AnyObject
	})
})

#WorkspaceAgentPolicyResource: close({
	apiVersion!: #APIVersion
	kind!:       "WorkspaceAgentPolicy"
	metadata!:   #Metadata
	spec!: close({
		enabled!: bool
		tools!: close({
			allow?: [...#ResourceID]
			deny?:  [...#ResourceID]
		})
		instructions?: string
	})
})

#RefreshPipelineResource: close({
	apiVersion!: #APIVersion
	kind!:       "RefreshPipeline"
	metadata!:   #Metadata
	spec!: close({
		semanticModel!: #ResourceID
		on?: close({
			schedule?: [...close({
				cron!:     string & !=""
				timezone?: string & !=""
			})]
		})
	})
})

#ModelTableResource: close({
	apiVersion!: #APIVersion
	kind!:       "ModelTable"
	metadata!:   #Metadata
	spec!:        #ModelTable
})

#SemanticModelResource: close({
	apiVersion!: #APIVersion
	kind!:       "SemanticModel"
	metadata!:   #Metadata
	spec!:        #ProjectSemanticModelSpec
})

#DashboardResource: close({
	apiVersion!: #APIVersion
	kind!:       "Dashboard"
	metadata!:   #Metadata
	spec!:        #DashboardSpec
})

#Connection: close({
	kind!:        "managed" | "s3" | "r2" | "gcs" | "http" | "azure_blob" | "postgres" | "mysql" | "sqlite" | "ducklake" | "quack"
	description?: string
	path?:        string
	root?:        string
	scope?:       string
	host?:        string
	port?:        int
	database?:    string
	username?:    string
	sslMode?:     string
	credentials?: #NoCredentials | #EnvCredentials | #AmbientCredentials
	options?:     #AnyObject
	defaults?: close({
		options?: #AnyObject
	})
})

#Source: close({
	format?:      "csv" | "json" | "parquet" | "excel" | "text" | "blob" | "vortex" | "delta" | "iceberg" | "lance"
	description?: string
	path?:        string
	connection?:  #Identifier
	object?:      string
	options?:     #AnyObject
	fields?: close({
		[#Identifier]: close({
			type?:        string
			description?: string
		})
	})
})

#ModelTable: close({
	kind?:   string
	source?: #ResourceID
	sources?: [...#ResourceID]
	sql?: string
	transform?: close({
		sql?: string
	})
	primaryKey!:  #Identifier
	grain?:       #Identifier
	fields?: close({
		[#Identifier]: close({
			label?:       string
			description?: string
			expr?:        string
			expression?:  string
			type?:        string
		})
	})
	description?: string
})

#ProjectSemanticModelSpec: close({
	tables!: [...#Identifier]
	relationships?: [...#Relationship]
	dimensions?: close({
		[#Identifier]: #SemanticDimension
	})
	measures!: close({
		[#Identifier]: #Measure
	})
	metrics?: close({
		[#Identifier]: #Metric
	})
})

#Measure: close({
	fact!:        #Identifier
	label?:       string
	description?: string
	aggregation!: "sum" | "count" | "count_distinct" | "avg" | "min" | "max"
	input?: close({
		field?: #FieldRef
		expression?: string
	})
	filters?: [...close({
		field!: #FieldRef
		operator!: "equals" | "in" | "contains" | "starts_with" | "greater_than_or_equal" | "less_than"
		values!: [..._]
	})]
	empty!: "zero" | "null"
	unit?:        string
	format?:      string
	hidden?:      bool
})

#SemanticDimension: close({
	label?: string
	description?: string
	type!: "string" | "number" | "boolean" | "date" | "timestamp"
	grains?: [...("day" | "week" | "month" | "quarter" | "year")]
	bindings!: close({
		[#Identifier]: close({
			field!: #FieldRef
			path?: [...#Identifier]
		})
	})
})

#Metric: close({
	label?: string
	description?: string
	expression!: string
	unit?: string
	format?: string
	hidden?: bool
})

#Relationship: close({
	id!:          #Identifier
	description?: string
	from!:        #FieldRef
	to!:          #FieldRef
	cardinality!: "many_to_one" | "one_to_one"
})

#Dashboard: close({
	id!:             #ObjectID
	title!:          string
	description?:    string
	semantic_model!: #Identifier
	filters?: close({
		[#Identifier]: #Filter
	})
	visuals!: close({
		[#Identifier]: #Visual
	})
	pages!: [...#Page]
})

#DashboardSpec: close({
	semanticModel!: #Identifier
	filters?: close({
		[#Identifier]: #Filter
	})
	visuals!: close({
		[#Identifier]: #Visual
	})
	pages!: [...#Page]
})

#Filter: close({
	type!:        "date_range" | "multi_select" | "text"
	label!:       string
	description?: string
	field!:       #FieldRef | #Identifier
	fact?:        #Identifier
	default?:     #FilterDefault
	custom?:      bool
	presets?: [...#FilterPreset]
	operator?: string
	values?: close({source?: string, limit?: int})
	default_operator?: string
	operators?: [...string]
	options?: [...close({value: string, label: string})]
	url_param?:          string
	from_url_param?:     string
	to_url_param?:       string
	operator_url_param?: string
	targets?: close({
		visuals?: [...#Identifier]
	})
})

#FilterDefault: close({
	preset?:   string
	from?:     string
	to?:       string
	operator?: string
	value?:    string
	values?: [...string]
})

#FilterPreset: close({
	value!:         string
	label!:         string
	from?:          string
	to?:            string
	relative_days?: int
})

#Visual: #ChartVisual | #KPIVisual | #DataTableVisual | #MatrixVisual | #PivotVisual

#VisualCommon: {
	title?:            string
	description?:      string
	interaction?:      null | #Interaction
}

#ChartVisual: close({
	#VisualCommon
	type!:             "line" | "area" | "bar" | "column" | "pie" | "donut" | "scatter" | "funnel" | "treemap" | "gauge" | "heatmap" | "sankey" | "graph" | "map" | "candlestick" | "boxplot" | "combo" | "waterfall" | "histogram" | "radar" | "tree" | "sunburst"
	shape?:            "category_value" | "category_series_value" | "category_multi_measure" | "category_delta" | "single_value" | "matrix" | "graph" | "geo" | "ohlc" | "distribution" | "binned_measure" | "hierarchy"
	renderer?:         "echarts" | "html"
	query!:            #VisualQuery
	options?:          #AnyObject
	renderer_options?: #AnyObject
	encode?: close({
		[string]: string
	})
})

#KPIVisual: close({
	#VisualCommon
	type!:    "kpi"
	shape?:   "single_value"
	query!:   #VisualQuery
	options?: #AnyObject
})

#TabularVisualCommon: {
	#VisualCommon
	title!:       string
	cardinality?: "bounded" | "exact"
	default_sort?: close({
		key?:       string
		direction?: "asc" | "desc" | string
	})
	style?: close({
		density?: "compact" | "comfortable" | "spacious" | string
		zebra?:   bool
		grid?:    "none" | "rows" | "columns" | "full" | string
	})
	columns?: [...#TableColumn]
	measure_formatting?: {
		[string]: [...#TableFormattingRule]
	}
}

#DataTableVisual: close({
	#TabularVisualCommon
	type!:  "table"
	query!: #TableQuery
})

#MatrixVisual: close({
	#TabularVisualCommon
	type!:  "matrix"
	query!: #TableQuery
})

#PivotVisual: close({
	#TabularVisualCommon
	type!:  "pivot"
	query!: #TableQuery
})

#VisualQuery: close({
	table?:      #Identifier
	dimensions?: #FieldRefs
	series?:     #FieldRefObject
	measures?:   #MeasureRefs
	time?: close({
		field?: #FieldRef | #Identifier
		grain?: string
		alias?: #Identifier
	})
	sort?: [...#Sort]
	limit?: int
})

#FieldRefs: [...#FieldRefValue] | close({
	[#Identifier]: (#FieldRef | #Identifier) | close({field: #FieldRef | #Identifier})
})

#MeasureRefs: [...#FieldRefValue] | close({
	[#Identifier]: null | close({
		measure!: #Identifier
	})
})

#FieldRefValue: #FieldRef | #Identifier | #FieldRefObject

#FieldRefObject: close({
	field!: #FieldRef | #Identifier
	alias!: #Identifier
})

#Sort: close({
	field?:     string
	direction?: "asc" | "desc" | string
	expr?:      string
})

#Interaction: close({
	point_selection?: #SelectionInteraction
	row_selection?:   #SelectionInteraction
})

#SelectionInteraction: close({
	toggle?: bool
	mappings?: [...close({
		field!: #FieldRef | #Identifier
		fact?:  #Identifier
		grain?: "day" | "week" | "month" | "quarter" | "year"
		value!: string
		label?: string
	})]
	targets?: [...#Identifier]
})

#TableQuery: close({
	table?: #Identifier
	fields?: [...#FieldRef]
	columns?:  #FieldRefs
	rows?:     #FieldRefs
	measures?: #MeasureRefs
})

#TableColumn: close({
	key!:          string
	label?:        string
	width?:        int
	format?:       "text" | "integer" | "decimal" | "currency" | "days" | string
	role?:         string
	align?:        string
	group?:        string
	measure?:      string
	column_value?: string
	formatting?: [...#TableFormattingRule]
})

#TableFormattingRule: close({
	kind!: "badge" | "text_color" | "background_scale" | "data_bar"
	values?: {[string]: string}
	min?:        number
	max?:        number
	color?:      string
	background?: string
	low_color?:  string
	high_color?: string
})

#Page: close({
	id!:          #ObjectID
	title!:       string
	description?: string
	canvas?: close({
		width?:  int
		height?: int
	})
	grid?: close({
		columns?:    int
		row_height?: int
		gap?:        int
		padding?:    int
	})
	components!: [...#PageComponent]
})

#PageComponent: #VisualComponent | #FilterComponent | #HeaderComponent

#PageComponentCommon: {
	id!:          #ObjectID
	description?: string
	placement!: close({
		col!:      int
		row!:      int
		col_span!: int
		row_span!: int
	})
	eyebrow?:  string
	title?:    string
	subtitle?: string
	badges?: [...string]
}

#VisualComponent: close({
	#PageComponentCommon
	kind!:   "visual"
	visual!: #Identifier
})

#FilterComponent: close({
	#PageComponentCommon
	kind!:   "filter"
	filter!: #Identifier
})

#HeaderComponent: close({
	#PageComponentCommon
	kind!: "header"
})
