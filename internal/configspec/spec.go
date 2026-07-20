// Package configspec defines LeapView's process-global environment contract.
// It intentionally contains data only so runtime configuration and generators
// consume the same source of truth.
package configspec

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const SecretPlaceholder = "<secret>"

type ValueType string

const (
	TypeString   ValueType = "string"
	TypeBool     ValueType = "boolean"
	TypeInt      ValueType = "integer"
	TypeInt64    ValueType = "integer64"
	TypeDuration ValueType = "duration"
)

type Setting struct {
	Name        string
	Field       string
	Type        ValueType
	DecodeType  ValueType
	Default     string
	Category    string
	Scope       string
	Description string
	Example     string
	Secret      bool
	Runtime     bool
	Lifecycle   string
	AliasFor    string
	EnvExample  string
	Commented   bool
}

// Settings returns a stable, name-sorted copy of the global catalog.
func Settings() []Setting {
	settings := append([]Setting(nil), settings...)
	sort.Slice(settings, func(i, j int) bool { return settings[i].Name < settings[j].Name })
	return settings
}

var settings = []Setting{
	{Name: "LEAPVIEW_ADDR", Field: "Addr", Type: TypeString, Category: "server", Scope: "serve,healthcheck", Description: "HTTP listen address.", Example: ":8080", Runtime: true, Lifecycle: "supported"},
	{Name: "LEAPVIEW_AGENT_API_KEY", Field: "AgentAPIKey", Type: TypeString, Category: "agent", Scope: "serve", Description: "API key for the configured agent model provider.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_AGENT_BASE_URL", Field: "AgentBaseURL", Type: TypeString, Default: "https://api.openai.com/v1", Category: "agent", Scope: "serve", Description: "OpenAI-compatible agent API base URL.", Example: "https://api.openai.com/v1", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_AGENT_MODEL", Field: "AgentModel", Type: TypeString, Category: "agent", Scope: "serve", Description: "Agent model identifier.", Example: "gpt-5", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_ALLOWED_HOSTS", Field: "AllowedHosts", Type: TypeString, Category: "security", Scope: "serve", Description: "Comma- or whitespace-separated exact hosts and wildcard suffixes accepted in production.", Example: "leapview.example.com", Runtime: true, Lifecycle: "supported", EnvExample: "leapview.example.com"},
	{Name: "LEAPVIEW_API_TOKEN", Field: "APIToken", Type: TypeString, Category: "client", Scope: "publish,api", Description: "API token used by non-interactive CLI commands.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_API_TOKEN_ONLY_AUTH", Field: "APITokenOnlyAuth", Type: TypeBool, Category: "authentication", Scope: "serve", Description: "Disable browser authentication and accept API tokens only.", Example: "true", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_ASSET_VERSION", Field: "AssetVersion", Type: TypeString, Category: "assets", Scope: "serve", Description: "Optional browser asset cache-busting version override.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_AZURE_CALLBACK_URL", Field: "AzureCallbackURL", Type: TypeString, Category: "authentication", Scope: "serve", Description: "HTTPS callback URL registered with Azure AD or Entra ID.", Example: "https://leapview.example.com/auth/azureadv2/callback", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_AZURE_CLIENT_ID", Field: "AzureClientID", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Azure AD or Entra ID OAuth client identifier.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_AZURE_CLIENT_SECRET", Field: "AzureSecret", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Azure AD or Entra ID OAuth client secret.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_AZURE_TENANT", Field: "AzureTenant", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Optional Azure AD or Entra ID tenant identifier.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_BASE_URL", Type: TypeString, Category: "development", Scope: "ui-qa", Description: "Base URL used by browser QA tooling.", Default: "http://localhost:8195", Lifecycle: "development"},
	{Name: "LEAPVIEW_BOOTSTRAP_ADMIN_EMAIL", Field: "BootstrapEmail", Type: TypeString, Category: "administration", Scope: "instance initialization", Description: "Email assigned to the initial production administrator.", Example: "admin@example.com", Runtime: true, Lifecycle: "supported", EnvExample: "admin@example.com"},
	{Name: "LEAPVIEW_BOOTSTRAP_CACHE_DIR", Type: TypeString, Category: "bootstrap", Scope: "bootstrap tools", Description: "Download cache directory used by dataset bootstrap tools.", Lifecycle: "tooling"},
	{Name: "LEAPVIEW_BOOTSTRAP_FORCE", Type: TypeBool, Category: "bootstrap", Scope: "bootstrap tools", Description: "Force dataset bootstrap tools to refresh existing files.", Default: "false", Lifecycle: "tooling"},
	{Name: "LEAPVIEW_BRIDGE_BENCH_ITERATIONS", Type: TypeInt, Default: "120", Category: "development", Scope: "browser benchmark", Description: "Measured Datastar bridge benchmark iterations.", Lifecycle: "development"},
	{Name: "LEAPVIEW_BRIDGE_BENCH_WARMUP", Type: TypeInt, Default: "20", Category: "development", Scope: "browser benchmark", Description: "Warm-up Datastar bridge benchmark iterations.", Lifecycle: "development"},
	{Name: "LEAPVIEW_CLI_CONFIG", Field: "CLIConfig", Type: TypeString, Category: "client", Scope: "client commands", Description: "Path to the local CLI target and token configuration file.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_COOKIE_SECURE", Field: "CookieSecureRaw", Type: TypeBool, DecodeType: TypeString, Category: "security", Scope: "serve", Description: "Secure-cookie override; defaults to true for production browser authentication.", Example: "true", Runtime: true, Lifecycle: "supported", EnvExample: "true"},
	{Name: "LEAPVIEW_CSRF_KEY", Field: "CSRFKey", Type: TypeString, Category: "security", Scope: "serve", Description: "Key used for CSRF protection and OAuth state cookies; production requires at least 32 characters.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", EnvExample: "replace-with-at-least-32-characters"},
	{Name: "LEAPVIEW_DEV_AUTH_BYPASS", Field: "DevAuthBypass", Type: TypeBool, Category: "authentication", Scope: "serve", Description: "Bypass authentication in development; forbidden in production.", Default: "false", Runtime: true, Lifecycle: "development", Commented: true},
	{Name: "LEAPVIEW_DEV_API_TOKEN", Field: "DevAPIToken", Type: TypeString, Default: "dev", Category: "authentication", Scope: "serve", Description: "Static bearer credential accepted by the public API in development; replace the development default on shared machines.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "development", Commented: true},
	{Name: "LEAPVIEW_DEV_LOG_LINES", Type: TypeInt, Default: "120", Category: "development", Scope: "dev server", Description: "Number of log lines shown by the managed development server.", Lifecycle: "development"},
	{Name: "LEAPVIEW_DEV_PORT_COUNT", Type: TypeInt, Default: "100", Category: "development", Scope: "dev server", Description: "Number of ports scanned by the managed development server.", Lifecycle: "development"},
	{Name: "LEAPVIEW_DEV_PORT_START", Type: TypeInt, Default: "8100", Category: "development", Scope: "dev server", Description: "First port scanned by the managed development server.", Lifecycle: "development"},
	{Name: "LEAPVIEW_DEV_PROJECT", Type: TypeString, Default: "dashboards/leapview.yaml", Category: "development", Scope: "dev server", Description: "Project published by the managed development server.", Lifecycle: "development"},
	{Name: "LEAPVIEW_DEV_READY_ATTEMPTS", Type: TypeInt, Default: "150", Category: "development", Scope: "dev server", Description: "Readiness attempts made by the managed development server.", Lifecycle: "development"},
	{Name: "LEAPVIEW_DEV_READY_INTERVAL", Type: TypeDuration, Default: "200ms", Category: "development", Scope: "dev server", Description: "Delay between managed development server readiness attempts.", Lifecycle: "development"},
	{Name: "LEAPVIEW_DEV_RESTART", Type: TypeBool, Default: "false", Category: "development", Scope: "dev server", Description: "Force the managed development server to restart.", Lifecycle: "development"},
	{Name: "LEAPVIEW_DEV_SKIP_PUBLISH", Type: TypeBool, Default: "false", Category: "development", Scope: "dev server", Description: "Skip automatic project publishing in the managed development server.", Lifecycle: "development"},
	{Name: "LEAPVIEW_DEV_WORKTREE", Type: TypeString, Category: "development", Scope: "dev server", Description: "Worktree path exported by the managed development server.", Lifecycle: "internal"},
	{Name: "LEAPVIEW_DUCKDB_DIR", Field: "DuckDBDir", Type: TypeString, Category: "storage", Scope: "serve", Description: "Directory containing workspace DuckDB runtime files.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_DUCKLAKE_CATALOG_PATH", Field: "DuckLakeCatalog", Type: TypeString, Category: "storage", Scope: "serve,admin", Description: "Path to the single global DuckLake catalog.", Example: "/var/lib/leapview/ducklake/catalog.sqlite", Runtime: true, Lifecycle: "supported", EnvExample: "/var/lib/leapview/ducklake/catalog.sqlite"},
	{Name: "LEAPVIEW_EXEC_JOB_LEASE_TIMEOUT", Field: "ExecJobLeaseTimeout", Type: TypeDuration, Default: "2m", Category: "execution", Scope: "serve", Description: "Lease duration before an abandoned refresh job may be reclaimed.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_EXEC_MAX_QUEUED_READS", Field: "ExecMaxQueuedReads", Type: TypeInt, Default: "64", Category: "execution", Scope: "serve", Description: "Maximum queued interactive read queries; negative disables queuing.", Runtime: true, Lifecycle: "supported", EnvExample: "64"},
	{Name: "LEAPVIEW_EXEC_MAX_QUEUED_WRITES", Field: "ExecMaxQueuedWrites", Type: TypeInt, Default: "64", Category: "execution", Scope: "serve", Description: "Maximum queued refresh jobs; negative disables queuing.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_EXEC_MAX_RUNNING_READS", Field: "ExecMaxRunningReads", Type: TypeInt, Default: "4", Category: "execution", Scope: "serve", Description: "Maximum concurrently running interactive read queries.", Runtime: true, Lifecycle: "supported", EnvExample: "4"},
	{Name: "LEAPVIEW_EXEC_MAX_RUNNING_WRITES", Field: "ExecMaxRunningWrites", Type: TypeInt, Default: "1", Category: "execution", Scope: "serve", Description: "Maximum concurrently running refresh jobs.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_EXEC_READ_QUEUE_TIMEOUT", Field: "ExecReadQueueTimeout", Type: TypeDuration, Default: "30s", Category: "execution", Scope: "serve", Description: "Maximum time an interactive read may wait in the queue.", Runtime: true, Lifecycle: "supported", EnvExample: "30s"},
	{Name: "LEAPVIEW_EXEC_READ_TIMEOUT", Field: "ExecReadTimeout", Type: TypeDuration, Default: "2m", Category: "execution", Scope: "serve", Description: "Maximum execution time for an interactive read query.", Runtime: true, Lifecycle: "supported", EnvExample: "2m"},
	{Name: "LEAPVIEW_ENVIRONMENT", Field: "Environment", Type: TypeString, Category: "server", Scope: "serve,admin", Description: "Single serving environment permanently bound to this LeapView instance.", Example: "prod", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_QUERY_CACHE_MAX_BYTES", Field: "QueryCacheMaxBytes", Type: TypeInt, Default: "67108864", Category: "execution", Scope: "serve", Description: "Maximum retained bytes for governed interactive query results per semantic model runtime.", Runtime: true, Lifecycle: "supported", EnvExample: "67108864"},
	{Name: "LEAPVIEW_QUERY_CACHE_MAX_ENTRIES", Field: "QueryCacheMaxEntries", Type: TypeInt, Default: "256", Category: "execution", Scope: "serve", Description: "Maximum governed interactive query result entries retained per semantic model runtime.", Runtime: true, Lifecycle: "supported", EnvExample: "256"},
	{Name: "LEAPVIEW_HEALTHCHECK_URL", Field: "HealthcheckURL", Type: TypeString, Category: "operations", Scope: "healthcheck", Description: "Explicit readiness URL used by the healthcheck command.", Example: "http://127.0.0.1:8080/readyz", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_HOME", Field: "HomeDir", Type: TypeString, Default: ".leapview", Category: "storage", Scope: "serve,admin,client", Description: "Instance state directory containing databases, artifacts, and runtime files.", Example: "/var/lib/leapview", Runtime: true, Lifecycle: "supported", EnvExample: "/var/lib/leapview"},
	{Name: "LEAPVIEW_IMAGE", Type: TypeString, Category: "deployment", Scope: "Hetzner provisioner", Description: "Immutable LeapView OCI image reference consumed by deployment tooling.", Example: "ghcr.io/yacobolo/leapview@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Lifecycle: "tooling"},
	{Name: "LEAPVIEW_LOCAL_AUTH", Field: "LocalAuth", Type: TypeBool, Category: "authentication", Scope: "serve", Description: "Enable administrator-managed local browser authentication.", Example: "true", Runtime: true, Lifecycle: "supported", EnvExample: "true"},
	{Name: "LEAPVIEW_MANAGED_DATA_BACKEND", Field: "ManagedDataBackend", Type: TypeString, Default: "local", Category: "managed data", Scope: "serve", Description: "Storage backend for project-global managed data; supported values are local and s3.", Runtime: true, Lifecycle: "supported", EnvExample: "local"},
	{Name: "LEAPVIEW_MANAGED_DATA_DIR", Field: "ManagedDataDir", Type: TypeString, Category: "managed data", Scope: "serve", Description: "Private local root for managed-data objects, upload staging, and verified runtime views; defaults beneath LEAPVIEW_HOME.", Runtime: true, Lifecycle: "supported", EnvExample: "/var/lib/leapview/managed-data", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_GC_GRACE_PERIOD", Field: "ManagedDataGCGracePeriod", Type: TypeDuration, Default: "24h", Category: "managed data", Scope: "serve", Description: "Minimum age of unreferenced managed-data objects before garbage collection.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_GC_INTERVAL", Field: "ManagedDataGCInterval", Type: TypeDuration, Default: "1h", Category: "managed data", Scope: "serve", Description: "Interval between managed-data garbage-collection passes.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_MAX_FILES", Field: "ManagedDataMaxFiles", Type: TypeInt, Default: "10000", Category: "managed data", Scope: "serve", Description: "Maximum number of files in one managed-data revision.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_MAX_FILE_BYTES", Field: "ManagedDataMaxFileBytes", Type: TypeInt64, Default: "1073741824", Category: "managed data", Scope: "serve", Description: "Maximum size in bytes of one managed-data file.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_MAX_REVISION_BYTES", Field: "ManagedDataMaxRevisionBytes", Type: TypeInt64, Default: "10737418240", Category: "managed data", Scope: "serve", Description: "Maximum total size in bytes of one managed-data revision.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_MIN_FREE_BYTES", Field: "ManagedDataMinFreeBytes", Type: TypeInt64, Default: "5368709120", Category: "managed data", Scope: "serve", Description: "Minimum free bytes required before accepting local managed-data uploads.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_S3_ACCESS_KEY_ID", Field: "ManagedDataS3AccessKeyID", Type: TypeString, Category: "managed data", Scope: "serve", Description: "Optional S3 access-key identifier for managed-data storage.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_S3_BUCKET", Field: "ManagedDataS3Bucket", Type: TypeString, Category: "managed data", Scope: "serve", Description: "S3 bucket used for managed-data objects and staging.", Example: "leapview-managed-data", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_S3_ENDPOINT", Field: "ManagedDataS3Endpoint", Type: TypeString, Category: "managed data", Scope: "serve", Description: "Optional S3-compatible endpoint URL.", Example: "https://s3.example.com", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_S3_PATH_STYLE", Field: "ManagedDataS3PathStyle", Type: TypeBool, Default: "false", Category: "managed data", Scope: "serve", Description: "Use path-style addressing for S3-compatible managed-data storage.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_S3_PREFIX", Field: "ManagedDataS3Prefix", Type: TypeString, Default: "managed-data", Category: "managed data", Scope: "serve", Description: "Object-key prefix for managed data in the configured S3 bucket.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_S3_REGION", Field: "ManagedDataS3Region", Type: TypeString, Category: "managed data", Scope: "serve", Description: "S3 region used for managed-data requests.", Example: "eu-west-1", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_S3_SECRET_ACCESS_KEY", Field: "ManagedDataS3SecretAccessKey", Type: TypeString, Category: "managed data", Scope: "serve", Description: "Optional S3 secret access key for managed-data storage.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_S3_SESSION_TOKEN", Field: "ManagedDataS3SessionToken", Type: TypeString, Category: "managed data", Scope: "serve", Description: "Optional temporary S3 session token for managed-data storage.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_MANAGED_DATA_UPLOAD_SESSION_TTL", Field: "ManagedDataUploadSessionTTL", Type: TypeDuration, Default: "24h", Category: "managed data", Scope: "serve", Description: "Lifetime of an incomplete managed-data upload session.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_METRICS_BEARER_TOKEN", Field: "MetricsBearerToken", Type: TypeString, Category: "operations", Scope: "serve", Description: "Bearer token protecting the Prometheus metrics endpoint; production requires at least 32 characters.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", EnvExample: "replace-with-at-least-32-characters"},
	{Name: "LEAPVIEW_MCP_OAUTH_ISSUER_URL", Field: "MCPOAuthIssuerURL", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Optional external OAuth issuer for MCP JWT access tokens; when omitted, LeapView provides the MCP authorization server.", Example: "https://identity.example.com", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_PERF_ITERATIONS", Type: TypeInt, Default: "5", Category: "development", Scope: "dashboard performance QA", Description: "Measured interaction iterations run by the configured dashboard performance scenario.", Lifecycle: "development"},
	{Name: "LEAPVIEW_PERF_ENFORCE_THRESHOLDS", Type: TypeBool, Default: "false", Category: "development", Scope: "dashboard performance QA", Description: "Fail dashboard performance QA when phase latency or query-count thresholds are exceeded.", Lifecycle: "development"},
	{Name: "LEAPVIEW_PERF_LOG", Type: TypeString, Default: ".tmp/dev-server.log", Category: "development", Scope: "dashboard performance QA", Description: "Development server log consumed by dashboard performance QA.", Lifecycle: "development"},
	{Name: "LEAPVIEW_PERF_MAX_ALL_TARGET_P95_MS", Type: TypeInt, Default: "1000", Category: "development", Scope: "dashboard performance QA", Description: "Maximum all-target settlement p95 when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LEAPVIEW_PERF_MAX_CRITICAL_KPI_P95_MS", Type: TypeInt, Default: "1000", Category: "development", Scope: "dashboard performance QA", Description: "Maximum critical-KPI settlement p95 when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LEAPVIEW_PERF_MAX_FIRST_TARGET_PAINT_P95_MS", Type: TypeInt, Default: "500", Category: "development", Scope: "dashboard performance QA", Description: "Maximum first-target paint p95 when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LEAPVIEW_PERF_MAX_OPTIMISTIC_FEEDBACK_P95_MS", Type: TypeInt, Default: "16", Category: "development", Scope: "dashboard performance QA", Description: "Maximum local optimistic-feedback p95 when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LEAPVIEW_PERF_MAX_QUERIES", Type: TypeInt, Default: "4", Category: "development", Scope: "dashboard performance QA", Description: "Maximum physical queries per measured refresh when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LEAPVIEW_PERF_OUTPUT", Type: TypeString, Category: "development", Scope: "dashboard performance QA", Description: "Optional JSON output path; defaults to a suite-specific file under .tmp.", Lifecycle: "development"},
	{Name: "LEAPVIEW_PERF_SCENARIO", Type: TypeString, Default: "scripts/performance/movielens.json", Category: "development", Scope: "dashboard performance QA", Description: "Path to a dashboard performance scenario manifest.", Lifecycle: "development"},
	{Name: "LEAPVIEW_OIDC_CALLBACK_URL", Field: "OIDCCallbackURL", Type: TypeString, Category: "authentication", Scope: "serve", Description: "HTTPS callback URL registered with the generic OIDC provider.", Example: "https://leapview.example.com/auth/oidc/callback", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_OIDC_CLIENT_ID", Field: "OIDCClientID", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Generic OIDC client identifier.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_OIDC_CLIENT_SECRET", Field: "OIDCSecret", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Generic OIDC client secret.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_OIDC_ISSUER_URL", Field: "OIDCIssuerURL", Type: TypeString, Category: "authentication", Scope: "serve", Description: "HTTPS issuer URL for the generic OIDC provider.", Example: "https://issuer.example.com", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_OIDC_PROVIDER_ID", Field: "OIDCProviderID", Type: TypeString, Default: "oidc", Category: "authentication", Scope: "serve", Description: "Route-safe identifier for the generic OIDC provider.", Example: "oidc", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_OIDC_SCOPES", Field: "OIDCScopes", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Comma- or whitespace-separated additional OIDC scopes.", Example: "openid profile email", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_PRODUCTION", Field: "Production", Type: TypeBool, Category: "server", Scope: "serve,admin", Description: "Enable production serving and validation behavior.", Default: "false", Runtime: true, Lifecycle: "supported", EnvExample: "1"},
	{Name: "LEAPVIEW_PUBLIC_URL", Field: "PublicURL", Type: TypeString, Category: "server", Scope: "serve", Description: "Canonical externally visible LeapView origin used for MCP resource identity and OAuth discovery.", Example: "https://leapview.example.com", Runtime: true, Lifecycle: "supported", EnvExample: "https://leapview.example.com"},
	{Name: "LEAPVIEW_QUACK_TOKEN", Type: TypeString, Category: "connection", Scope: "Quack development profile", Description: "Credential injected into the Quack connection declaration.", Example: SecretPlaceholder, Secret: true, Lifecycle: "external"},
	{Name: "LEAPVIEW_SCIM_BEARER_TOKEN", Field: "SCIMBearerToken", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Bearer token enabling SCIM provisioning; production requires at least 32 characters when set.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_SITE_BASE_URL", Type: TypeString, Category: "site", Scope: "public site", Description: "Externally visible HTTP(S) origin used for canonical URLs, discovery documents, and transport policy.", Example: "https://leapview.dev", Lifecycle: "supported"},
	{Name: "LEAPVIEW_SMOKE_PORT", Type: TypeInt, Default: "18080", Category: "development", Scope: "production image smoke test", Description: "Host port used by the production image smoke test.", Lifecycle: "internal"},
	{Name: "LEAPVIEW_TARGET", Field: "Target", Type: TypeString, Category: "client", Scope: "client commands", Description: "Default LeapView API target URL.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_TOKEN_HASH_KEY", Field: "TokenHashKey", Type: TypeString, Category: "security", Scope: "serve", Description: "Optional dedicated key for deterministic API-token fingerprints; falls back to the CSRF key.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LEAPVIEW_TRUST_PROXY_HEADERS", Field: "TrustProxyHeaders", Type: TypeBool, Category: "security", Scope: "serve", Description: "Trust client-address headers only when a trusted proxy overwrites them.", Default: "false", Runtime: true, Lifecycle: "supported", EnvExample: "false"},
	{Name: "LEAPVIEW_WAREHOUSE_DSN", Type: TypeString, Category: "connection", Scope: "example connection", Description: "Example externally supplied warehouse connection credential.", Example: SecretPlaceholder, Secret: true, Lifecycle: "external"},
}

type PredicateKind string

const (
	PredicateAll         PredicateKind = "all"
	PredicateAny         PredicateKind = "any"
	PredicateNot         PredicateKind = "not"
	PredicatePresent     PredicateKind = "present"
	PredicateTrue        PredicateKind = "true"
	PredicateMinLength   PredicateKind = "min_length"
	PredicateHTTPSURL    PredicateKind = "https_url"
	PredicateHTTPSOrigin PredicateKind = "https_origin"
	PredicateSlug        PredicateKind = "route_slug"
	PredicateEquals      PredicateKind = "equals"
	PredicateOneOf       PredicateKind = "one_of"
	PredicatePositive    PredicateKind = "positive"
	PredicateAtLeast     PredicateKind = "at_least_setting"
)

type Predicate struct {
	Kind      PredicateKind `json:"kind"`
	Name      string        `json:"name,omitempty"`
	Minimum   int           `json:"minimum,omitempty"`
	Value     string        `json:"value,omitempty"`
	Values    []string      `json:"values,omitempty"`
	OtherName string        `json:"otherName,omitempty"`
	Children  []Predicate   `json:"children,omitempty"`
}

func All(children ...Predicate) Predicate { return Predicate{Kind: PredicateAll, Children: children} }
func Any(children ...Predicate) Predicate { return Predicate{Kind: PredicateAny, Children: children} }
func Not(child Predicate) Predicate {
	return Predicate{Kind: PredicateNot, Children: []Predicate{child}}
}
func Present(name string) Predicate { return Predicate{Kind: PredicatePresent, Name: name} }
func True(name string) Predicate    { return Predicate{Kind: PredicateTrue, Name: name} }
func MinLength(name string, minimum int) Predicate {
	return Predicate{Kind: PredicateMinLength, Name: name, Minimum: minimum}
}
func HTTPSURL(name string) Predicate    { return Predicate{Kind: PredicateHTTPSURL, Name: name} }
func HTTPSOrigin(name string) Predicate { return Predicate{Kind: PredicateHTTPSOrigin, Name: name} }
func RouteSlug(name string) Predicate   { return Predicate{Kind: PredicateSlug, Name: name} }
func Equals(name, value string) Predicate {
	return Predicate{Kind: PredicateEquals, Name: name, Value: value}
}
func OneOf(name string, values ...string) Predicate {
	return Predicate{Kind: PredicateOneOf, Name: name, Values: append([]string(nil), values...)}
}
func Positive(name string) Predicate { return Predicate{Kind: PredicatePositive, Name: name} }
func AtLeast(name, otherName string) Predicate {
	return Predicate{Kind: PredicateAtLeast, Name: name, OtherName: otherName}
}

type Rule struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	When        Predicate `json:"when,omitempty"`
	Assert      Predicate `json:"assert"`
	Message     string    `json:"message"`
}

func Rules() []Rule { return append([]Rule(nil), rules...) }

var (
	production    = True("LEAPVIEW_PRODUCTION")
	oidcAny       = Any(Present("LEAPVIEW_OIDC_ISSUER_URL"), Present("LEAPVIEW_OIDC_CLIENT_ID"), Present("LEAPVIEW_OIDC_CLIENT_SECRET"), Present("LEAPVIEW_OIDC_CALLBACK_URL"), Present("LEAPVIEW_OIDC_SCOPES"))
	oidcComplete  = All(Present("LEAPVIEW_OIDC_ISSUER_URL"), Present("LEAPVIEW_OIDC_CLIENT_ID"), Present("LEAPVIEW_OIDC_CLIENT_SECRET"), Present("LEAPVIEW_OIDC_CALLBACK_URL"))
	azureAny      = Any(Present("LEAPVIEW_AZURE_CLIENT_ID"), Present("LEAPVIEW_AZURE_CLIENT_SECRET"), Present("LEAPVIEW_AZURE_CALLBACK_URL"), Present("LEAPVIEW_AZURE_TENANT"))
	azureComplete = All(Present("LEAPVIEW_AZURE_CLIENT_ID"), Present("LEAPVIEW_AZURE_CLIENT_SECRET"), Present("LEAPVIEW_AZURE_CALLBACK_URL"))
	browserAuth   = Any(True("LEAPVIEW_LOCAL_AUTH"), oidcComplete, azureComplete)
	managedData   = Present("LEAPVIEW_MANAGED_DATA_BACKEND")
	managedS3     = Equals("LEAPVIEW_MANAGED_DATA_BACKEND", "s3")
)

var rules = []Rule{
	{ID: "production-dev-bypass", Description: "Production cannot bypass authentication.", When: production, Assert: Not(True("LEAPVIEW_DEV_AUTH_BYPASS")), Message: "production serve must not enable LEAPVIEW_DEV_AUTH_BYPASS"},
	{ID: "production-oidc-complete", Description: "OIDC settings are all-or-none in production.", When: All(production, oidcAny), Assert: oidcComplete, Message: "production OIDC auth requires LEAPVIEW_OIDC_ISSUER_URL, LEAPVIEW_OIDC_CLIENT_ID, LEAPVIEW_OIDC_CLIENT_SECRET, and LEAPVIEW_OIDC_CALLBACK_URL"},
	{ID: "production-azure-complete", Description: "Azure settings are all-or-none in production.", When: All(production, azureAny), Assert: azureComplete, Message: "production Azure auth requires LEAPVIEW_AZURE_CLIENT_ID, LEAPVIEW_AZURE_CLIENT_SECRET, and LEAPVIEW_AZURE_CALLBACK_URL"},
	{ID: "production-auth-mode", Description: "Production requires local, API-token-only, OIDC, or Azure authentication.", When: production, Assert: Any(True("LEAPVIEW_LOCAL_AUTH"), True("LEAPVIEW_API_TOKEN_ONLY_AUTH"), oidcComplete, azureComplete), Message: "production serve requires OIDC auth env vars, Azure auth env vars, LEAPVIEW_LOCAL_AUTH, or LEAPVIEW_API_TOKEN_ONLY_AUTH"},
	{ID: "production-csrf-key", Description: "Production requires a CSRF key with at least 32 characters.", When: production, Assert: MinLength("LEAPVIEW_CSRF_KEY", 32), Message: "production serve requires LEAPVIEW_CSRF_KEY with at least 32 characters"},
	{ID: "production-metrics-token", Description: "Production requires a metrics bearer token with at least 32 characters.", When: production, Assert: MinLength("LEAPVIEW_METRICS_BEARER_TOKEN", 32), Message: "production metrics scraping requires LEAPVIEW_METRICS_BEARER_TOKEN with at least 32 characters"},
	{ID: "production-public-url", Description: "Production requires a canonical public URL.", When: production, Assert: Present("LEAPVIEW_PUBLIC_URL"), Message: "production serve requires LEAPVIEW_PUBLIC_URL"},
	{ID: "production-public-url-https", Description: "The production public URL must be an HTTPS origin without a path, query, fragment, or credentials.", When: production, Assert: HTTPSOrigin("LEAPVIEW_PUBLIC_URL"), Message: "production serve requires LEAPVIEW_PUBLIC_URL to be an https origin"},
	{ID: "production-mcp-oauth-issuer-https", Description: "An external production MCP OAuth issuer must use HTTPS.", When: All(production, Present("LEAPVIEW_MCP_OAUTH_ISSUER_URL")), Assert: HTTPSURL("LEAPVIEW_MCP_OAUTH_ISSUER_URL"), Message: "production serve requires LEAPVIEW_MCP_OAUTH_ISSUER_URL to be an https URL"},
	{ID: "production-allowed-host", Description: "Production derives an allowed host from its public URL, explicit hosts, or a browser-auth callback host.", When: production, Assert: Any(Present("LEAPVIEW_PUBLIC_URL"), Present("LEAPVIEW_ALLOWED_HOSTS"), Present("LEAPVIEW_OIDC_CALLBACK_URL"), Present("LEAPVIEW_AZURE_CALLBACK_URL")), Message: "production serve requires LEAPVIEW_PUBLIC_URL, LEAPVIEW_ALLOWED_HOSTS, or an OIDC/Azure callback URL host"},
	{ID: "production-secure-cookie", Description: "Production browser authentication requires secure cookies unless API-token-only mode is also enabled.", When: All(production, browserAuth, Not(True("LEAPVIEW_API_TOKEN_ONLY_AUTH"))), Assert: True("LEAPVIEW_COOKIE_SECURE"), Message: "production browser auth requires LEAPVIEW_COOKIE_SECURE=true"},
	{ID: "production-oidc-issuer-https", Description: "The production OIDC issuer must use HTTPS.", When: All(production, oidcComplete), Assert: HTTPSURL("LEAPVIEW_OIDC_ISSUER_URL"), Message: "production serve requires LEAPVIEW_OIDC_ISSUER_URL to be an https URL"},
	{ID: "production-oidc-callback-https", Description: "The production OIDC callback must use HTTPS.", When: All(production, oidcComplete), Assert: HTTPSURL("LEAPVIEW_OIDC_CALLBACK_URL"), Message: "production serve requires LEAPVIEW_OIDC_CALLBACK_URL to be an https URL"},
	{ID: "production-oidc-provider-slug", Description: "The OIDC provider identifier must be route-safe.", When: All(production, oidcComplete), Assert: RouteSlug("LEAPVIEW_OIDC_PROVIDER_ID"), Message: "LEAPVIEW_OIDC_PROVIDER_ID must be a route-safe slug containing only letters, numbers, dots, underscores, or dashes"},
	{ID: "production-azure-callback-https", Description: "The production Azure callback must use HTTPS.", When: All(production, azureComplete), Assert: HTTPSURL("LEAPVIEW_AZURE_CALLBACK_URL"), Message: "production serve requires LEAPVIEW_AZURE_CALLBACK_URL to be an https URL"},
	{ID: "production-scim-token", Description: "A configured production SCIM token must contain at least 32 characters.", When: All(production, Present("LEAPVIEW_SCIM_BEARER_TOKEN")), Assert: MinLength("LEAPVIEW_SCIM_BEARER_TOKEN", 32), Message: "production SCIM provisioning requires LEAPVIEW_SCIM_BEARER_TOKEN with at least 32 characters"},
	{ID: "managed-data-backend", Description: "Managed data uses a supported storage backend.", When: managedData, Assert: OneOf("LEAPVIEW_MANAGED_DATA_BACKEND", "local", "s3"), Message: "LEAPVIEW_MANAGED_DATA_BACKEND must be local or s3"},
	{ID: "managed-data-runtime-dir", Description: "Every managed-data backend requires a private local runtime and staging directory.", When: managedData, Assert: Present("LEAPVIEW_MANAGED_DATA_DIR"), Message: "managed-data storage requires LEAPVIEW_MANAGED_DATA_DIR"},
	{ID: "managed-data-s3-location", Description: "The S3 managed-data backend requires a bucket and region.", When: managedS3, Assert: All(Present("LEAPVIEW_MANAGED_DATA_S3_BUCKET"), Present("LEAPVIEW_MANAGED_DATA_S3_REGION")), Message: "S3 managed-data storage requires LEAPVIEW_MANAGED_DATA_S3_BUCKET and LEAPVIEW_MANAGED_DATA_S3_REGION"},
	{ID: "managed-data-s3-credentials", Description: "Managed-data S3 credentials are either omitted or configured as a complete key pair.", When: managedS3, Assert: Any(All(Not(Present("LEAPVIEW_MANAGED_DATA_S3_ACCESS_KEY_ID")), Not(Present("LEAPVIEW_MANAGED_DATA_S3_SECRET_ACCESS_KEY")), Not(Present("LEAPVIEW_MANAGED_DATA_S3_SESSION_TOKEN"))), All(Present("LEAPVIEW_MANAGED_DATA_S3_ACCESS_KEY_ID"), Present("LEAPVIEW_MANAGED_DATA_S3_SECRET_ACCESS_KEY"))), Message: "managed-data S3 credentials require both LEAPVIEW_MANAGED_DATA_S3_ACCESS_KEY_ID and LEAPVIEW_MANAGED_DATA_S3_SECRET_ACCESS_KEY; a session token also requires that pair"},
	{ID: "managed-data-positive-limits", Description: "Managed-data upload, session, garbage-collection, and free-space limits are positive.", When: managedData, Assert: All(Positive("LEAPVIEW_MANAGED_DATA_MAX_FILES"), Positive("LEAPVIEW_MANAGED_DATA_MAX_FILE_BYTES"), Positive("LEAPVIEW_MANAGED_DATA_MAX_REVISION_BYTES"), Positive("LEAPVIEW_MANAGED_DATA_UPLOAD_SESSION_TTL"), Positive("LEAPVIEW_MANAGED_DATA_GC_INTERVAL"), Positive("LEAPVIEW_MANAGED_DATA_GC_GRACE_PERIOD"), Positive("LEAPVIEW_MANAGED_DATA_MIN_FREE_BYTES")), Message: "managed-data limits, durations, and free-space thresholds must be positive"},
	{ID: "managed-data-revision-limit", Description: "The managed-data revision limit is at least the per-file limit.", When: managedData, Assert: AtLeast("LEAPVIEW_MANAGED_DATA_MAX_REVISION_BYTES", "LEAPVIEW_MANAGED_DATA_MAX_FILE_BYTES"), Message: "LEAPVIEW_MANAGED_DATA_MAX_REVISION_BYTES must be at least LEAPVIEW_MANAGED_DATA_MAX_FILE_BYTES"},
}

func (r Rule) References() []string {
	seen := map[string]struct{}{}
	collectReferences(r.When, seen)
	collectReferences(r.Assert, seen)
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func collectReferences(predicate Predicate, seen map[string]struct{}) {
	if predicate.Name != "" {
		seen[predicate.Name] = struct{}{}
	}
	if predicate.OtherName != "" {
		seen[predicate.OtherName] = struct{}{}
	}
	for _, child := range predicate.Children {
		collectReferences(child, seen)
	}
}

func Validate(values map[string]any) error {
	for _, rule := range rules {
		if rule.When.Kind != "" && !rule.When.Evaluate(values) {
			continue
		}
		if !rule.Assert.Evaluate(values) {
			return fmt.Errorf("%s", rule.Message)
		}
	}
	return nil
}

func (p Predicate) Evaluate(values map[string]any) bool {
	switch p.Kind {
	case "":
		return true
	case PredicateAll:
		for _, child := range p.Children {
			if !child.Evaluate(values) {
				return false
			}
		}
		return true
	case PredicateAny:
		for _, child := range p.Children {
			if child.Evaluate(values) {
				return true
			}
		}
		return false
	case PredicateNot:
		return len(p.Children) == 1 && !p.Children[0].Evaluate(values)
	case PredicatePresent:
		return present(values[p.Name])
	case PredicateTrue:
		value, ok := values[p.Name].(bool)
		return ok && value
	case PredicateMinLength:
		value, _ := values[p.Name].(string)
		return len(strings.TrimSpace(value)) >= p.Minimum
	case PredicateHTTPSURL:
		value, _ := values[p.Name].(string)
		parsed, err := url.Parse(strings.TrimSpace(value))
		return err == nil && parsed.Scheme == "https" && parsed.Host != ""
	case PredicateHTTPSOrigin:
		value, _ := values[p.Name].(string)
		parsed, err := url.Parse(strings.TrimSpace(value))
		return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
			(parsed.Path == "" || parsed.Path == "/") && parsed.RawQuery == "" && parsed.Fragment == ""
	case PredicateSlug:
		return routeSlug(values[p.Name])
	case PredicateEquals:
		value, _ := values[p.Name].(string)
		return strings.TrimSpace(value) == p.Value
	case PredicateOneOf:
		value, _ := values[p.Name].(string)
		value = strings.TrimSpace(value)
		for _, allowed := range p.Values {
			if value == allowed {
				return true
			}
		}
		return false
	case PredicatePositive:
		value, ok := numericValue(values[p.Name])
		return ok && value > 0
	case PredicateAtLeast:
		value, valueOK := numericValue(values[p.Name])
		other, otherOK := numericValue(values[p.OtherName])
		return valueOK && otherOK && value >= other
	default:
		return false
	}
}

func numericValue(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case time.Duration:
		return int64(value), true
	default:
		return 0, false
	}
}

func present(value any) bool {
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value) != ""
	case bool:
		return value
	case int:
		return value != 0
	default:
		return value != nil
	}
}

func routeSlug(value any) bool {
	id, _ := value.(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return true
	}
	if len(id) > 64 {
		return false
	}
	for index, char := range []byte(id) {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		if index > 0 && (char == '-' || char == '_' || char == '.') {
			continue
		}
		return false
	}
	return true
}
