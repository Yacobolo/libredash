package compiler

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/analytics/connectors"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/workspace"
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
		for _, name := range sortedMapKeys(workspaceProject.Publications) {
			publication := workspaceProject.Publications[name]
			path := workspaceProject.PublicationPaths[name]
			resourceID := "dashboard_publication:" + workspaceProject.ID + "." + name
			dashboard, ok := workspaceProject.Dashboards[publication.Dashboard]
			if !ok {
				return resourceError(path, resourceID, "spec.dashboard", "DashboardPublication %q.%q references unknown Dashboard %q", workspaceProject.ID, name, publication.Dashboard)
			}
			pageFound := false
			for _, page := range dashboard.Pages {
				if page.ID == publication.DefaultPage {
					pageFound = true
					break
				}
			}
			if !pageFound {
				return resourceError(path, resourceID, "spec.defaultPage", "DashboardPublication %q.%q references unknown page %q in Dashboard %q", workspaceProject.ID, name, publication.DefaultPage, publication.Dashboard)
			}
			origins := make([]string, 0, len(publication.AllowedOrigins))
			seenOrigins := map[string]struct{}{}
			for index, authored := range publication.AllowedOrigins {
				origin, err := validatePublicationOrigin(authored)
				field := fmt.Sprintf("spec.embedding.allowedOrigins[%d]", index)
				if err != nil {
					return resourceError(path, resourceID, field, "DashboardPublication %q.%q origin %q %s", workspaceProject.ID, name, authored, err.Error())
				}
				if _, duplicate := seenOrigins[origin]; duplicate {
					return resourceError(path, resourceID, field, "DashboardPublication %q.%q has duplicate origin %q", workspaceProject.ID, name, origin)
				}
				seenOrigins[origin] = struct{}{}
				origins = append(origins, origin)
			}
			sort.Strings(origins)
			publication.AllowedOrigins = origins
			workspaceProject.Publications[name] = publication
		}
		pipelinesByModel := map[string]string{}
		for _, name := range sortedMapKeys(workspaceProject.RefreshPipelines) {
			pipeline := workspaceProject.RefreshPipelines[name]
			path := workspaceProject.RefreshPipelinePaths[name]
			if _, ok := workspaceProject.SemanticModels[pipeline.SemanticModel]; !ok {
				return resourceError(path, "refresh_pipeline:"+workspaceProject.ID+"."+name, "spec.semanticModel", "RefreshPipeline %q.%q references unknown semantic model %q", workspaceProject.ID, name, pipeline.SemanticModel)
			}
			if existing, ok := pipelinesByModel[pipeline.SemanticModel]; ok {
				return resourceError(path, "refresh_pipeline:"+workspaceProject.ID+"."+name, "spec.semanticModel", "semantic model %q already has refresh pipeline %q", pipeline.SemanticModel, existing)
			}
			pipelinesByModel[pipeline.SemanticModel] = name
		}
		if err := validateWorkspaceAccess(workspaceProject); err != nil {
			return err
		}
	}
	return nil
}

func validatePublicationOrigin(authored string) (string, error) {
	if authored == "" || strings.TrimSpace(authored) != authored {
		return "", fmt.Errorf("must be a non-empty exact origin")
	}
	if strings.Contains(authored, "*") {
		return "", fmt.Errorf("must not contain wildcards")
	}
	parsed, err := url.Parse(authored)
	if err != nil || parsed.IsAbs() == false || parsed.Host == "" || parsed.Opaque != "" {
		return "", fmt.Errorf("must be an absolute HTTP(S) origin")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("must not contain credentials")
	}
	if parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("must contain an origin only, without path, query, or fragment")
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("must include a hostname")
	}
	if parsed.Scheme != "https" {
		loopback := strings.EqualFold(hostname, "localhost")
		if ip := net.ParseIP(hostname); ip != nil {
			loopback = ip.IsLoopback()
		}
		if parsed.Scheme != "http" || !loopback {
			return "", fmt.Errorf("must use https except for loopback development origins")
		}
	}
	return parsed.Scheme + "://" + parsed.Host, nil
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
		if grant.Subject.Kind == string(access.SubjectDashboardPublication) {
			return resourceError(path, "grant:"+workspaceProject.ID+"."+name, "spec.subject.kind", "Grant %q.%q dashboard_publication subjects are only supported by DataPolicy", workspaceProject.ID, name)
		}
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
			if policy.Subject.Kind == "dashboard_publication" {
				if _, ok := workspaceProject.Publications[policy.Subject.Publication]; !ok {
					return resourceError(path, "data_policy:"+workspaceProject.ID+"."+name, "spec.subject.publication", "DataPolicy %q.%q references unknown DashboardPublication %q", workspaceProject.ID, name, policy.Subject.Publication)
				}
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
	case "dashboard_publication":
		if subject.Publication == "" {
			return resourceError(path, resourceID, "spec.subject.publication", "%s %q.%q dashboard_publication subject requires publication", kind, workspaceID, name)
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
		access.PrivilegeActivateDeployment,
		access.PrivilegeManagePublications,
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
