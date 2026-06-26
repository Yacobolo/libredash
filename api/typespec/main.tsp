import "./common.tsp";
import "./current_user.tsp";
import "./workspaces.tsp";
import "./search.tsp";
import "./bi.tsp";
import "./deployments.tsp";
import "./materializations.tsp";
import "./agent.tsp";
import "./access.tsp";
import "./audit.tsp";

using Http;
using TypeSpec.OpenAPI;

@service(#{ title: "LibreDash Headless API" })
@doc("Headless API for LibreDash workspaces, deployments, access control, materializations, and agent operations.")
@info(#{ version: "1.0.0" })
@useAuth(BearerAuth)
@tagMetadata(#[#{ name: "Current User", description: "Current principal, sessions, tokens, and permissions." }, #{ name: "Workspaces", description: "Workspace and lineage discovery." }, #{ name: "Search", description: "Progressive workspace discovery across BI assets." }, #{ name: "BI", description: "Dashboard and semantic model read APIs." }, #{ name: "Deployments", description: "Dashboard-as-code deployment operations." }, #{ name: "Materializations", description: "Headless materialization run operations." }, #{ name: "Agent", description: "Headless agent conversation and run operations." }, #{ name: "Access", description: "Principals, groups, roles, and role bindings." }, #{ name: "Audit", description: "Workspace audit event discovery." }])
namespace LibreDashAPI;
