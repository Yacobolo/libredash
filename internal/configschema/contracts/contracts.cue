package contracts

#Identifier: =~"^[A-Za-z_][A-Za-z0-9_]*$"
#ObjectID:   =~"^[A-Za-z_][A-Za-z0-9_-]*$"
#FieldRef:   =~"^[A-Za-z_][A-Za-z0-9_]*\\.[A-Za-z_][A-Za-z0-9_]*$"
#AnyObject: {
	[string]: _
}

#Catalog: close({
	workspace?: close({
		id?:          string
		title?:       string
		description?: string
	})
	semantic_models!: [...#CatalogAsset]
	dashboards!: [...#CatalogDashboard]
})

#CatalogAsset: close({
	id!:          #Identifier
	title!:       string
	path!:        string
	description?: string
})

#CatalogDashboard: close({
	id!:          #ObjectID
	title!:       string
	path!:        string
	description?: string
	tags?: [...string]
})

#SemanticModel: close({
	name!:               #Identifier
	title?:              string
	description?:        string
	default_connection?: #Identifier
	connections?: close({
		[#Identifier]: #Connection
	})
	sources!: close({
		[#Identifier]: #Source
	})
	models!: close({
		[#Identifier]: #ModelTable
	})
	semantic_models!: close({
		[#Identifier]: #SemanticModelSpec
	})
})

#Connection: close({
	kind!:        string
	description?: string
	path?:        string
	root?:        string
	scope?:       string
	auth?:        #AnyObject
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
			description?: string
		})
	})
})

#ModelTable: close({
	kind?:   string
	source?: #Identifier
	sources?: [...#Identifier]
	sql?: string
	transform?: close({
		sql?: string
	})
	primary_key!: #Identifier
	grain?:       #Identifier
	fields?: close({
		[#Identifier]: close({
			label?:       string
			description?: string
			expr?:        string
		})
	})
	measures?: close({
		[#Identifier]: #Measure
	})
	description?: string
})

#SemanticModelSpec: close({
	base_table: #Identifier
	tables: [...#Identifier]
	relationships?: [...#Relationship]
	measures?: close({
		defaults?:     #MeasureDefaults
		[#Identifier]: #Measure | #MeasureDefaults
	})
})

#MeasureDefaults: close({
	table?: #Identifier
	grain?: #Identifier
	time?:  #FieldRef
	grains?: [...string]
})

#Measure: close({
	table?:       #Identifier
	label?:       string
	description?: string
	expr?:        string
	expression?:  string
	unit?:        string
	format?:      string
	grain?:       #Identifier
	time?:        #FieldRef
	grains?: [...string]
})

#Relationship: close({
	id?:          #Identifier
	description?: string
	from!:        #FieldRef
	to!:          #FieldRef
	cardinality?: string
	active?:      bool
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
	tables?: close({
		[#Identifier]: #Table
	})
	pages!: [...#Page]
})

#Filter: close({
	type!:        "date_range" | "multi_select" | "text"
	label!:       string
	description?: string
	field!:       #FieldRef
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
		tables?: [...#Identifier]
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

#Visual: close({
	title?:            string
	description?:      string
	kind?:             "chart" | "kpi"
	shape?:            "category_value" | "category_series_value" | "category_multi_measure" | "category_delta" | "single_value" | "matrix" | "graph" | "geo" | "ohlc" | "distribution" | "binned_measure" | "hierarchy"
	renderer?:         "echarts" | "html"
	type?:             "line" | "area" | "bar" | "column" | "pie" | "donut" | "scatter" | "funnel" | "treemap" | "gauge" | "heatmap" | "sankey" | "graph" | "map" | "candlestick" | "boxplot" | "combo" | "waterfall" | "histogram" | "radar" | "tree" | "sunburst" | "kpi"
	query!:            #VisualQuery
	options?:          #AnyObject
	renderer_options?: #AnyObject
	interaction?:      null | #Interaction
	encode?: close({
		[string]: string
	})
})

#VisualQuery: close({
	table?:      #Identifier
	dimensions?: #FieldRefs
	series?:     #FieldRefObject
	measures?:   #MeasureRefs
	time?: close({
		field?: #FieldRef
		grain?: string
		alias?: #Identifier
	})
	sort?: [...#Sort]
	limit?: int
})

#FieldRefs: [...#FieldRefValue] | close({
	[#Identifier]: #FieldRef | close({field: #FieldRef})
})

#MeasureRefs: [...#FieldRefValue] | close({
	[#Identifier]: null | close({
		measure?: #Identifier | #FieldRef
		expr?:    string
		table?:   #Identifier
		grain?:   #Identifier
		time?:    #FieldRef
		grains?: [...string]
		format?: string
	})
})

#FieldRefValue: #FieldRef | #FieldRefObject

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
	mode?:   string
	mappings?: [...close({
		field!: #FieldRef
		value!: string
		label?: string
	})]
	targets?: [...#Identifier]
})

#Table: close({
	kind?:        "data_table" | "matrix_table" | "pivot_table"
	title!:       string
	description?: string
	query!:       #TableQuery
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
	interaction?: null | #Interaction
	measure_formatting?: {
		[string]: [...#TableFormattingRule]
	}
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
	visuals!: [...#PageVisual]
})

#PageVisual: close({
	id!:          #ObjectID
	kind!:        "header" | "filter_card" | "kpi_card" | "line_chart" | "area_chart" | "bar_chart" | "column_chart" | "pie_chart" | "donut_chart" | "scatter_chart" | "funnel_chart" | "treemap_chart" | "gauge_chart" | "heatmap_chart" | "sankey_chart" | "graph_chart" | "map_chart" | "candlestick_chart" | "boxplot_chart" | "combo_chart" | "waterfall_chart" | "histogram_chart" | "radar_chart" | "tree_chart" | "sunburst_chart" | "table"
	visual?:      #Identifier
	table?:       #Identifier
	filter?:      #Identifier
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
})
