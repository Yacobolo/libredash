package compiler

import (
	"fmt"

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
	Uses           workspaceUses `yaml:"uses"`
	Models         includeList   `yaml:"models"`
	SemanticModels includeList   `yaml:"semanticModels"`
	Dashboards     includeList   `yaml:"dashboards"`
	Access         includeList   `yaml:"access"`
	AgentPolicy    includeList   `yaml:"agentPolicy"`
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
	BaseTable     string                       `yaml:"baseTable"`
	Tables        []string                     `yaml:"tables"`
	Relationships []semanticmodel.Relationship `yaml:"relationships"`
	Measures      projectSemanticModelMeasures `yaml:"measures"`
}

type projectSemanticModelMeasures struct {
	Defaults semanticmodel.MeasureDefaults
	Items    map[string]semanticmodel.MetricMeasure
}

type dashboardSpec struct {
	SemanticModel string                             `yaml:"semanticModel"`
	Filters       map[string]report.FilterDefinition `yaml:"filters"`
	Visuals       map[string]report.Visual           `yaml:"visuals"`
	Tables        map[string]report.TableVisual      `yaml:"tables"`
	Pages         []projectDashboardPage             `yaml:"pages"`
}

type projectModelTableSpec struct {
	Kind        string                                 `yaml:"kind"`
	Source      string                                 `yaml:"source"`
	Sources     []string                               `yaml:"sources"`
	SourceReads map[string][]string                    `yaml:"sourceReads"`
	SQL         string                                 `yaml:"sql"`
	Transform   semanticmodel.Transform                `yaml:"transform"`
	Columns     map[string]semanticmodel.ModelColumn   `yaml:"columns"`
	PrimaryKey  string                                 `yaml:"primaryKey"`
	Grain       string                                 `yaml:"grain"`
	Fields      map[string]projectModelField           `yaml:"fields"`
	Measures    map[string]semanticmodel.MetricMeasure `yaml:"measures"`
	Description string                                 `yaml:"description"`
}

type projectModelField struct {
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
	Expr        string `yaml:"expr"`
	Expression  string `yaml:"expression"`
	Type        string `yaml:"type"`
}

type projectDashboardPage struct {
	Name        string                 `yaml:"name"`
	Title       string                 `yaml:"title"`
	Description string                 `yaml:"description"`
	Canvas      dashboard.PageCanvas   `yaml:"canvas"`
	Grid        dashboard.PageGrid     `yaml:"grid"`
	Visuals     []dashboard.PageVisual `yaml:"visuals"`
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

func (m *projectSemanticModelMeasures) UnmarshalYAML(value *yaml.Node) error {
	m.Items = map[string]semanticmodel.MetricMeasure{}
	if value == nil || value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return nil
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("semantic model measures must be a mapping")
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		key := value.Content[index].Value
		item := value.Content[index+1]
		if key == "defaults" {
			if err := item.Decode(&m.Defaults); err != nil {
				return err
			}
			continue
		}
		var measure semanticmodel.MetricMeasure
		if item.Kind != yaml.ScalarNode || item.Tag != "!!null" {
			if err := item.Decode(&measure); err != nil {
				return err
			}
		}
		m.Items[key] = measure
	}
	return nil
}
