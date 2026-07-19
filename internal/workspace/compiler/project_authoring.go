package compiler

import (
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	"gopkg.in/yaml.v3"
)

type resourceEnvelope struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   metadata  `yaml:"metadata"`
	Spec       yaml.Node `yaml:"spec"`
}

type metadata struct {
	Name        string   `yaml:"name"`
	Workspace   string   `yaml:"workspace"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Owner       string   `yaml:"owner"`
	Tags        []string `yaml:"tags"`
}

type projectResource struct {
	Connections includeList `yaml:"connections"`
	Sources     includeList `yaml:"sources"`
	Workspaces  includeList `yaml:"workspaces"`
}

type includeList struct {
	Include []string `yaml:"include"`
}

type workspaceSpec struct {
	Uses             workspaceUses `yaml:"uses"`
	Models           includeList   `yaml:"models"`
	SemanticModels   includeList   `yaml:"semanticModels"`
	Dashboards       includeList   `yaml:"dashboards"`
	Access           includeList   `yaml:"access"`
	AgentPolicy      includeList   `yaml:"agentPolicy"`
	RefreshPipelines includeList   `yaml:"refreshPipelines"`
}

type workspaceUses struct {
	Sources []string `yaml:"sources"`
}

type sourceSpec struct {
	Format      string                        `yaml:"format"`
	Description string                        `yaml:"description"`
	Path        string                        `yaml:"path"`
	Connection  string                        `yaml:"connection"`
	Object      string                        `yaml:"object"`
	Options     map[string]any                `yaml:"options"`
	Fields      map[string]projectSourceField `yaml:"fields"`
}

type projectSourceField struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

type projectSemanticModelSpec struct {
	Tables        []string                                   `yaml:"tables"`
	Relationships []semanticmodel.Relationship               `yaml:"relationships"`
	Dimensions    map[string]semanticmodel.SemanticDimension `yaml:"dimensions"`
	Measures      map[string]semanticmodel.MetricMeasure     `yaml:"measures"`
	Metrics       map[string]semanticmodel.Metric            `yaml:"metrics"`
}

type dashboardSpec struct {
	SemanticModel string                             `yaml:"semanticModel"`
	Filters       map[string]report.FilterDefinition `yaml:"filters"`
	Visuals       map[string]dashboardVisualSpec     `yaml:"visuals"`
	Pages         []projectDashboardPage             `yaml:"pages"`
}

type dashboardVisualSpec struct {
	Type    string
	Chart   *report.Visual
	Tabular *report.TableVisual
}

func (v *dashboardVisualSpec) UnmarshalYAML(value *yaml.Node) error {
	var discriminator struct {
		Type string `yaml:"type"`
	}
	if err := value.Decode(&discriminator); err != nil {
		return err
	}
	v.Type = discriminator.Type
	switch discriminator.Type {
	case "table", "matrix", "pivot":
		var definition report.TableVisual
		if err := value.Decode(&definition); err != nil {
			return err
		}
		switch discriminator.Type {
		case "table":
			definition.Kind = "data_table"
		case "matrix":
			definition.Kind = "matrix_table"
		case "pivot":
			definition.Kind = "pivot_table"
		}
		v.Tabular = &definition
	default:
		var definition report.Visual
		if err := value.Decode(&definition); err != nil {
			return err
		}
		definition.Type = discriminator.Type
		v.Chart = &definition
	}
	return nil
}

func splitDashboardVisuals(in map[string]dashboardVisualSpec) (map[string]report.Visual, map[string]report.TableVisual) {
	charts := make(map[string]report.Visual)
	tables := make(map[string]report.TableVisual)
	for id, visual := range in {
		if visual.Chart != nil {
			charts[id] = *visual.Chart
		}
		if visual.Tabular != nil {
			tables[id] = *visual.Tabular
		}
	}
	return charts, tables
}

// splitDashboardFilterTargets keeps the renderer-specific query services
// internal while presenting a single visual target namespace to authors.
func splitDashboardFilterTargets(filters map[string]report.FilterDefinition, tables map[string]report.TableVisual) map[string]report.FilterDefinition {
	for id, filter := range filters {
		visuals := make([]string, 0, len(filter.Targets.Visuals))
		tabular := make([]string, 0, len(filter.Targets.Visuals))
		for _, target := range filter.Targets.Visuals {
			if _, ok := tables[target]; ok {
				tabular = append(tabular, target)
			} else {
				visuals = append(visuals, target)
			}
		}
		filter.Targets.Visuals = visuals
		filter.Targets.Tables = tabular
		filters[id] = filter
	}
	return filters
}

type projectModelTableSpec struct {
	Source      string                               `yaml:"source"`
	Sources     []string                             `yaml:"sources"`
	SourceReads map[string][]string                  `yaml:"sourceReads"`
	SQL         string                               `yaml:"sql"`
	Transform   semanticmodel.Transform              `yaml:"transform"`
	Columns     map[string]semanticmodel.ModelColumn `yaml:"columns"`
	PrimaryKey  string                               `yaml:"primaryKey"`
	Grain       string                               `yaml:"grain"`
	Fields      map[string]projectModelField         `yaml:"fields"`
	Description string                               `yaml:"description"`
}

type projectModelField struct {
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
	Expr        string `yaml:"expr"`
	Expression  string `yaml:"expression"`
	Type        string `yaml:"type"`
}

type projectDashboardPage struct {
	ID          string                 `yaml:"id"`
	Title       string                 `yaml:"title"`
	Description string                 `yaml:"description"`
	Canvas      dashboard.PageCanvas   `yaml:"canvas"`
	Grid        dashboard.PageGrid     `yaml:"grid"`
	Components  []dashboard.PageVisual `yaml:"components"`
}

type workspaceGroupSpec struct {
	Description string                     `yaml:"description"`
	Members     []workspaceGroupMemberSpec `yaml:"members"`
}

type workspaceGroupMemberSpec struct {
	PrincipalID string `yaml:"principalId"`
	Email       string `yaml:"email"`
	DisplayName string `yaml:"displayName"`
}

type workspaceRoleBindingSpec struct {
	Role    string                          `yaml:"role"`
	Subject workspaceRoleBindingSubjectSpec `yaml:"subject"`
}

type workspaceRoleBindingSubjectSpec struct {
	Kind        string `yaml:"kind"`
	PrincipalID string `yaml:"principalId"`
	Email       string `yaml:"email"`
	DisplayName string `yaml:"displayName"`
	Group       string `yaml:"group"`
}

type workspaceSecurableObjectSpec struct {
	Type string `yaml:"type"`
	ID   string `yaml:"id"`
}

type workspaceGrantSpec struct {
	Object    workspaceSecurableObjectSpec    `yaml:"object"`
	Subject   workspaceRoleBindingSubjectSpec `yaml:"subject"`
	Privilege string                          `yaml:"privilege"`
}

type workspaceDataPolicySpec struct {
	Object     workspaceSecurableObjectSpec    `yaml:"object"`
	Subject    workspaceRoleBindingSubjectSpec `yaml:"subject"`
	PolicyType string                          `yaml:"policyType"`
	Expression yaml.Node                       `yaml:"expression"`
}

type workspaceAgentPolicySpec struct {
	Enabled      bool                          `yaml:"enabled"`
	Tools        workspaceAgentPolicyToolsSpec `yaml:"tools"`
	Instructions string                        `yaml:"instructions"`
}

type workspaceAgentPolicyToolsSpec struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

type refreshPipelineSpec struct {
	SemanticModel string                `yaml:"semanticModel"`
	On            refreshPipelineOnSpec `yaml:"on"`
}

type refreshPipelineOnSpec struct {
	Schedule []refreshPipelineScheduleSpec `yaml:"schedule"`
}

type refreshPipelineScheduleSpec struct {
	Cron     string `yaml:"cron"`
	Timezone string `yaml:"timezone"`
}
