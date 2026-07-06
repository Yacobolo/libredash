package compiler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/analytics/connectors"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func validateProject(project Project) error {
	for connectionName, connection := range project.Connections {
		if _, err := connection.Validate(connectionName); err != nil {
			return resourceError(project.ConnectionPaths[connectionName], "connection:"+connectionName, "spec", "Connection %q %s", connectionName, err.Error())
		}
	}
	for sourceName, source := range project.Sources {
		if _, ok := project.Connections[source.Connection]; !ok {
			return resourceError(project.SourcePaths[sourceName], "source:"+sourceName, "spec.connection", "Source %q references unknown Connection %q", sourceName, source.Connection)
		}
		if source.Path != "" && source.Format == "" {
			format, ok := connectors.InferFormat(source.Path)
			if !ok {
				return resourceError(project.SourcePaths[sourceName], "source:"+sourceName, "spec.format", "Source %q path %q requires format", sourceName, source.Path)
			}
			source.Format = format
		}
		if err := source.Validate(localSourceName(sourceName), project.Connections); err != nil {
			return resourceError(project.SourcePaths[sourceName], "source:"+sourceName, "spec", "Source %q %s", sourceName, err.Error())
		}
	}
	for _, workspaceProject := range project.Workspaces {
		for source := range workspaceProject.AllowedSources {
			if _, ok := project.Sources[source]; !ok {
				return resourceError(workspaceProject.Path, "workspace:"+workspaceProject.ID, "spec.uses.sources", "Workspace %q allows unknown Source %q", workspaceProject.ID, source)
			}
		}
		if len(workspaceProject.SemanticModels) == 0 {
			return resourceError(workspaceProject.Path, "workspace:"+workspaceProject.ID, "spec.semanticModels", "Workspace %q requires SemanticModel resources", workspaceProject.ID)
		}
		for tableName, table := range workspaceProject.Models {
			for _, source := range table.Sources {
				if _, ok := workspaceProject.AllowedSources[source]; !ok {
					return resourceError(workspaceProject.ModelPaths[tableName], "model_table:"+workspaceProject.ID+"."+tableName, "spec.sources", "ModelTable %q.%q reads source %q outside uses.sources", workspaceProject.ID, tableName, source)
				}
			}
			if table.Source != "" {
				if _, ok := workspaceProject.AllowedSources[table.Source]; !ok {
					return resourceError(workspaceProject.ModelPaths[tableName], "model_table:"+workspaceProject.ID+"."+tableName, "spec.source", "ModelTable %q.%q reads source %q outside uses.sources", workspaceProject.ID, tableName, table.Source)
				}
			}
			if err := validateProjectTableSources(workspaceProject.ID, tableName, workspaceProject.ModelPaths[tableName], table); err != nil {
				return err
			}
		}
		for name, dashboard := range workspaceProject.Dashboards {
			if _, ok := workspaceProject.SemanticModels[dashboard.SemanticModel]; !ok {
				return resourceError(workspaceProject.DashboardPaths[name], "dashboard:"+workspaceProject.ID+"."+name, "spec.semanticModel", "Dashboard %q.%q references unknown SemanticModel %q", workspaceProject.ID, name, dashboard.SemanticModel)
			}
		}
		if err := validateWorkspaceAccess(workspaceProject); err != nil {
			return err
		}
		if err := validateWorkspaceAgentPolicies(workspaceProject); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkspaceAgentPolicies(workspaceProject *WorkspaceProject) error {
	for name, policy := range workspaceProject.AgentPolicies {
		path := workspaceProject.AgentPolicyPaths[name]
		allow := map[string]struct{}{}
		for _, tool := range policy.Tools.Allow {
			if !workspace.IsKnownAgentTool(tool) {
				return resourceError(path, "workspace_agent_policy:"+workspaceProject.ID+"."+name, "spec.tools.allow", "WorkspaceAgentPolicy %q.%q references unknown agent tool %q", workspaceProject.ID, name, tool)
			}
			allow[tool] = struct{}{}
		}
		for _, tool := range policy.Tools.Deny {
			if !workspace.IsKnownAgentTool(tool) {
				return resourceError(path, "workspace_agent_policy:"+workspaceProject.ID+"."+name, "spec.tools.deny", "WorkspaceAgentPolicy %q.%q references unknown agent tool %q", workspaceProject.ID, name, tool)
			}
			if _, ok := allow[tool]; ok {
				return resourceError(path, "workspace_agent_policy:"+workspaceProject.ID+"."+name, "spec.tools", "WorkspaceAgentPolicy %q.%q agent tool %q is both allowed and denied", workspaceProject.ID, name, tool)
			}
		}
	}
	return nil
}

func validateWorkspaceAccess(workspaceProject *WorkspaceProject) error {
	validRoles := map[string]struct{}{
		access.RoleOwner:         {},
		access.RoleAdmin:         {},
		access.RoleDeployer:      {},
		access.RoleContributor:   {},
		access.RoleEditor:        {},
		access.RoleMember:        {},
		access.RoleViewer:        {},
		access.RolePlatformAdmin: {},
	}
	for name, group := range workspaceProject.AccessGroups {
		for index, member := range group.Members {
			if member.PrincipalID == "" && member.Email == "" {
				return resourceError(workspaceProject.AccessPaths["WorkspaceGroup:"+name], "workspace_group:"+workspaceProject.ID+"."+name, fmt.Sprintf("spec.members[%d]", index), "WorkspaceGroup %q.%q member requires principalId or email", workspaceProject.ID, name)
			}
		}
	}
	for name, binding := range workspaceProject.AccessRoleBindings {
		path := workspaceProject.AccessPaths["WorkspaceRoleBinding:"+name]
		if _, ok := validRoles[binding.Role]; !ok {
			return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.role", "WorkspaceRoleBinding %q.%q references unknown role %q", workspaceProject.ID, name, binding.Role)
		}
		switch binding.Subject.Kind {
		case string(access.SubjectGroup):
			if binding.Subject.Group == "" {
				return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.subject.group", "WorkspaceRoleBinding %q.%q group subject requires group", workspaceProject.ID, name)
			}
			if _, ok := workspaceProject.AccessGroups[binding.Subject.Group]; !ok {
				return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.subject.group", "WorkspaceRoleBinding %q.%q references unknown WorkspaceGroup %q", workspaceProject.ID, name, binding.Subject.Group)
			}
		case string(access.SubjectPrincipal):
			if binding.Subject.PrincipalID == "" && binding.Subject.Email == "" {
				return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.subject", "WorkspaceRoleBinding %q.%q principal subject requires principalId or email", workspaceProject.ID, name)
			}
		default:
			return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.subject.kind", "WorkspaceRoleBinding %q.%q has unsupported subject kind %q", workspaceProject.ID, name, binding.Subject.Kind)
		}
	}
	for name, grant := range workspaceProject.AccessGrants {
		path := workspaceProject.AccessPaths["Grant:"+name]
		if err := validateWorkspaceObjectRef(path, "grant:"+workspaceProject.ID+"."+name, "Grant", workspaceProject.ID, name, grant.Object); err != nil {
			return err
		}
		if !validPrivilege(access.Privilege(grant.Privilege)) {
			return resourceError(path, "grant:"+workspaceProject.ID+"."+name, "spec.privilege", "Grant %q.%q has unsupported privilege %q", workspaceProject.ID, name, grant.Privilege)
		}
		if err := validateWorkspaceAccessSubject(path, "grant:"+workspaceProject.ID+"."+name, "Grant", workspaceProject.ID, name, grant.Subject, workspaceProject.AccessGroups); err != nil {
			return err
		}
	}
	for name, policy := range workspaceProject.AccessDataPolicies {
		path := workspaceProject.AccessPaths["DataPolicy:"+name]
		if err := validateWorkspaceObjectRef(path, "data_policy:"+workspaceProject.ID+"."+name, "DataPolicy", workspaceProject.ID, name, policy.Object); err != nil {
			return err
		}
		switch policy.PolicyType {
		case "row_filter", "column_mask":
		default:
			return resourceError(path, "data_policy:"+workspaceProject.ID+"."+name, "spec.policyType", "DataPolicy %q.%q has unsupported policyType %q", workspaceProject.ID, name, policy.PolicyType)
		}
		if strings.TrimSpace(policy.ExpressionJSON) == "" {
			return resourceError(path, "data_policy:"+workspaceProject.ID+"."+name, "spec.expression", "DataPolicy %q.%q requires expression", workspaceProject.ID, name)
		}
		if strings.TrimSpace(policy.Subject.Kind) != "" {
			if err := validateWorkspaceAccessSubject(path, "data_policy:"+workspaceProject.ID+"."+name, "DataPolicy", workspaceProject.ID, name, policy.Subject, workspaceProject.AccessGroups); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateWorkspaceAccessSubject(path, resourceID, kind, workspaceID, name string, subject workspace.WorkspaceRoleBindingSubject, groups map[string]workspace.WorkspaceGroup) error {
	switch subject.Kind {
	case string(access.SubjectGroup):
		if subject.Group == "" {
			return resourceError(path, resourceID, "spec.subject.group", "%s %q.%q group subject requires group", kind, workspaceID, name)
		}
		if _, ok := groups[subject.Group]; !ok {
			return resourceError(path, resourceID, "spec.subject.group", "%s %q.%q references unknown WorkspaceGroup %q", kind, workspaceID, name, subject.Group)
		}
	case string(access.SubjectPrincipal):
		if subject.PrincipalID == "" && subject.Email == "" {
			return resourceError(path, resourceID, "spec.subject", "%s %q.%q principal subject requires principalId or email", kind, workspaceID, name)
		}
	case string(access.SubjectServicePrincipal):
		if subject.PrincipalID == "" {
			return resourceError(path, resourceID, "spec.subject.principalId", "%s %q.%q service_principal subject requires principalId", kind, workspaceID, name)
		}
	default:
		return resourceError(path, resourceID, "spec.subject.kind", "%s %q.%q has unsupported subject kind %q", kind, workspaceID, name, subject.Kind)
	}
	return nil
}

func validateWorkspaceObjectRef(path, resourceID, kind, workspaceID, name string, object workspace.WorkspaceSecurableObjectRef) error {
	switch access.SecurableType(object.Type) {
	case access.SecurableWorkspace:
		return nil
	case access.SecurableDashboard,
		access.SecurableSemanticModel,
		access.SecurableSource,
		access.SecurableModelTable,
		access.SecurableAgentPolicy,
		access.SecurableDataset,
		access.SecurableTable,
		access.SecurableColumn:
		if object.ID == "" {
			return resourceError(path, resourceID, "spec.object.id", "%s %q.%q object id is required for %q", kind, workspaceID, name, object.Type)
		}
		return nil
	default:
		return resourceError(path, resourceID, "spec.object.type", "%s %q.%q has unsupported object type %q", kind, workspaceID, name, object.Type)
	}
}

func validPrivilege(privilege access.Privilege) bool {
	switch privilege {
	case access.PrivilegeUseWorkspace,
		access.PrivilegeViewItem,
		access.PrivilegeEditItem,
		access.PrivilegeManageItem,
		access.PrivilegeQueryData,
		access.PrivilegePreviewData,
		access.PrivilegeRefreshData,
		access.PrivilegeDeploy,
		access.PrivilegeActivatePublish,
		access.PrivilegeUseAgent,
		access.PrivilegeViewAgent,
		access.PrivilegeManageGrants,
		access.PrivilegeViewAudit,
		access.PrivilegeManageWorkspace,
		access.PrivilegeManagePlatform:
		return true
	default:
		return false
	}
}

func validateProjectTableSources(workspaceID, tableName, path string, table semanticmodel.Table) error {
	sql := strings.TrimSpace(table.Transform.SQL)
	if sql == "" {
		sql = strings.TrimSpace(table.SQL)
	}
	if sql == "" {
		return nil
	}
	declared := append([]string{}, table.Sources...)
	if table.Source != "" {
		declared = append(declared, table.Source)
	}
	sort.Strings(declared)
	inferred, rawRefs, unqualifiedRefs := (&semanticmodel.Model{}).SQLSourceRefs(sql)
	if len(rawRefs) > 0 {
		return resourceError(path, "model_table:"+workspaceID+"."+tableName, "spec.sql", "ModelTable %q.%q SQL must reference sources through source.<name>; raw.<name> is internal", workspaceID, tableName)
	}
	if len(unqualifiedRefs) > 0 {
		return resourceError(path, "model_table:"+workspaceID+"."+tableName, "spec.sql", "ModelTable %q.%q SQL must reference sources through source.<name>; found unqualified relation %q", workspaceID, tableName, unqualifiedRefs[0])
	}
	if !sameStringList(declared, inferred) {
		return resourceError(path, "model_table:"+workspaceID+"."+tableName, "spec.sources", "ModelTable %q.%q SQL source references %v do not match declared sources %v", workspaceID, tableName, inferred, declared)
	}
	return nil
}
