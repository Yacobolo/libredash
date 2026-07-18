// Package configspec defines LibreDash's process-global environment contract.
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
	{Name: "LIBREDASH_ADDR", Field: "Addr", Type: TypeString, Category: "server", Scope: "serve,healthcheck", Description: "HTTP listen address.", Example: ":8080", Runtime: true, Lifecycle: "supported"},
	{Name: "LIBREDASH_AGENT_API_KEY", Field: "AgentAPIKey", Type: TypeString, Category: "agent", Scope: "serve", Description: "API key for the configured agent model provider.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_AGENT_BASE_URL", Field: "AgentBaseURL", Type: TypeString, Default: "https://api.openai.com/v1", Category: "agent", Scope: "serve", Description: "OpenAI-compatible agent API base URL.", Example: "https://api.openai.com/v1", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_AGENT_MODEL", Field: "AgentModel", Type: TypeString, Category: "agent", Scope: "serve", Description: "Agent model identifier.", Example: "gpt-5", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_ALLOWED_HOSTS", Field: "AllowedHosts", Type: TypeString, Category: "security", Scope: "serve", Description: "Comma- or whitespace-separated exact hosts and wildcard suffixes accepted in production.", Example: "libredash.example.com", Runtime: true, Lifecycle: "supported", EnvExample: "libredash.example.com"},
	{Name: "LIBREDASH_API_TOKEN", Field: "APIToken", Type: TypeString, Category: "client", Scope: "publish,api", Description: "API token used by non-interactive CLI commands.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_API_TOKEN_ONLY_AUTH", Field: "APITokenOnlyAuth", Type: TypeBool, Category: "authentication", Scope: "serve", Description: "Disable browser authentication and accept API tokens only.", Example: "true", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_ASSET_VERSION", Field: "AssetVersion", Type: TypeString, Category: "assets", Scope: "serve", Description: "Optional browser asset cache-busting version override.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_AZURE_CALLBACK_URL", Field: "AzureCallbackURL", Type: TypeString, Category: "authentication", Scope: "serve", Description: "HTTPS callback URL registered with Azure AD or Entra ID.", Example: "https://libredash.example.com/auth/azureadv2/callback", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_AZURE_CLIENT_ID", Field: "AzureClientID", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Azure AD or Entra ID OAuth client identifier.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_AZURE_CLIENT_SECRET", Field: "AzureSecret", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Azure AD or Entra ID OAuth client secret.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_AZURE_TENANT", Field: "AzureTenant", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Optional Azure AD or Entra ID tenant identifier.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_BASE_URL", Type: TypeString, Category: "development", Scope: "ui-qa", Description: "Base URL used by browser QA tooling.", Default: "http://localhost:8195", Lifecycle: "development"},
	{Name: "LIBREDASH_BOOTSTRAP_ADMIN_EMAIL", Field: "BootstrapEmail", Type: TypeString, Category: "administration", Scope: "admin bootstrap", Description: "Email assigned to the production bootstrap administrator.", Example: "admin@example.com", Runtime: true, Lifecycle: "supported", EnvExample: "admin@example.com"},
	{Name: "LIBREDASH_BOOTSTRAP_CACHE_DIR", Type: TypeString, Category: "bootstrap", Scope: "bootstrap tools", Description: "Download cache directory used by dataset bootstrap tools.", Lifecycle: "tooling"},
	{Name: "LIBREDASH_BOOTSTRAP_FORCE", Type: TypeBool, Category: "bootstrap", Scope: "bootstrap tools", Description: "Force dataset bootstrap tools to refresh existing files.", Default: "false", Lifecycle: "tooling"},
	{Name: "LIBREDASH_BRIDGE_BENCH_ITERATIONS", Type: TypeInt, Default: "120", Category: "development", Scope: "browser benchmark", Description: "Measured Datastar bridge benchmark iterations.", Lifecycle: "development"},
	{Name: "LIBREDASH_BRIDGE_BENCH_WARMUP", Type: TypeInt, Default: "20", Category: "development", Scope: "browser benchmark", Description: "Warm-up Datastar bridge benchmark iterations.", Lifecycle: "development"},
	{Name: "LIBREDASH_CLI_CONFIG", Field: "CLIConfig", Type: TypeString, Category: "client", Scope: "client commands", Description: "Path to the local CLI target and token configuration file.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_COOKIE_SECURE", Field: "CookieSecureRaw", Type: TypeBool, DecodeType: TypeString, Category: "security", Scope: "serve", Description: "Secure-cookie override; defaults to true for production browser authentication.", Example: "true", Runtime: true, Lifecycle: "supported", EnvExample: "true"},
	{Name: "LIBREDASH_CSRF_KEY", Field: "CSRFKey", Type: TypeString, Category: "security", Scope: "serve", Description: "Key used for CSRF protection and OAuth state cookies; production requires at least 32 characters.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", EnvExample: "replace-with-at-least-32-characters"},
	{Name: "LIBREDASH_DEV_AUTH_BYPASS", Field: "DevAuthBypass", Type: TypeBool, Category: "authentication", Scope: "serve", Description: "Bypass authentication in development; forbidden in production.", Default: "false", Runtime: true, Lifecycle: "development", Commented: true},
	{Name: "LIBREDASH_DEV_API_TOKEN", Field: "DevAPIToken", Type: TypeString, Default: "dev", Category: "authentication", Scope: "serve", Description: "Static bearer credential accepted by the public API in development; replace the development default on shared machines.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "development", Commented: true},
	{Name: "LIBREDASH_DEV_LOG_LINES", Type: TypeInt, Default: "120", Category: "development", Scope: "dev server", Description: "Number of log lines shown by the managed development server.", Lifecycle: "development"},
	{Name: "LIBREDASH_DEV_PORT_COUNT", Type: TypeInt, Default: "100", Category: "development", Scope: "dev server", Description: "Number of ports scanned by the managed development server.", Lifecycle: "development"},
	{Name: "LIBREDASH_DEV_PORT_START", Type: TypeInt, Default: "8100", Category: "development", Scope: "dev server", Description: "First port scanned by the managed development server.", Lifecycle: "development"},
	{Name: "LIBREDASH_DEV_PROJECT", Type: TypeString, Default: "dashboards/libredash.yaml", Category: "development", Scope: "dev server", Description: "Project published by the managed development server.", Lifecycle: "development"},
	{Name: "LIBREDASH_DEV_READY_ATTEMPTS", Type: TypeInt, Default: "150", Category: "development", Scope: "dev server", Description: "Readiness attempts made by the managed development server.", Lifecycle: "development"},
	{Name: "LIBREDASH_DEV_READY_INTERVAL", Type: TypeDuration, Default: "200ms", Category: "development", Scope: "dev server", Description: "Delay between managed development server readiness attempts.", Lifecycle: "development"},
	{Name: "LIBREDASH_DEV_RESTART", Type: TypeBool, Default: "false", Category: "development", Scope: "dev server", Description: "Force the managed development server to restart.", Lifecycle: "development"},
	{Name: "LIBREDASH_DEV_SKIP_PUBLISH", Type: TypeBool, Default: "false", Category: "development", Scope: "dev server", Description: "Skip automatic project publishing in the managed development server.", Lifecycle: "development"},
	{Name: "LIBREDASH_DEV_WORKTREE", Type: TypeString, Category: "development", Scope: "dev server", Description: "Worktree path exported by the managed development server.", Lifecycle: "internal"},
	{Name: "LIBREDASH_DUCKDB_DIR", Field: "DuckDBDir", Type: TypeString, Category: "storage", Scope: "serve", Description: "Directory containing workspace DuckDB runtime files.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_DUCKLAKE_CATALOG_PATH", Field: "DuckLakeCatalog", Type: TypeString, Category: "storage", Scope: "serve,admin", Description: "Path to the single global DuckLake catalog.", Example: "/var/lib/libredash/ducklake/catalog.sqlite", Runtime: true, Lifecycle: "supported", EnvExample: "/var/lib/libredash/ducklake/catalog.sqlite"},
	{Name: "LIBREDASH_EXEC_JOB_LEASE_TIMEOUT", Field: "ExecJobLeaseTimeout", Type: TypeDuration, Default: "2m", Category: "execution", Scope: "serve", Description: "Lease duration before an abandoned refresh job may be reclaimed.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_EXEC_MAX_QUEUED_READS", Field: "ExecMaxQueuedReads", Type: TypeInt, Default: "64", Category: "execution", Scope: "serve", Description: "Maximum queued interactive read queries; negative disables queuing.", Runtime: true, Lifecycle: "supported", EnvExample: "64"},
	{Name: "LIBREDASH_EXEC_MAX_QUEUED_WRITES", Field: "ExecMaxQueuedWrites", Type: TypeInt, Default: "64", Category: "execution", Scope: "serve", Description: "Maximum queued refresh jobs; negative disables queuing.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_EXEC_MAX_RUNNING_READS", Field: "ExecMaxRunningReads", Type: TypeInt, Default: "4", Category: "execution", Scope: "serve", Description: "Maximum concurrently running interactive read queries.", Runtime: true, Lifecycle: "supported", EnvExample: "4"},
	{Name: "LIBREDASH_EXEC_MAX_RUNNING_WRITES", Field: "ExecMaxRunningWrites", Type: TypeInt, Default: "1", Category: "execution", Scope: "serve", Description: "Maximum concurrently running refresh jobs.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_EXEC_READ_QUEUE_TIMEOUT", Field: "ExecReadQueueTimeout", Type: TypeDuration, Default: "30s", Category: "execution", Scope: "serve", Description: "Maximum time an interactive read may wait in the queue.", Runtime: true, Lifecycle: "supported", EnvExample: "30s"},
	{Name: "LIBREDASH_EXEC_READ_TIMEOUT", Field: "ExecReadTimeout", Type: TypeDuration, Default: "2m", Category: "execution", Scope: "serve", Description: "Maximum execution time for an interactive read query.", Runtime: true, Lifecycle: "supported", EnvExample: "2m"},
	{Name: "LIBREDASH_QUERY_CACHE_MAX_BYTES", Field: "QueryCacheMaxBytes", Type: TypeInt, Default: "67108864", Category: "execution", Scope: "serve", Description: "Maximum retained bytes for governed interactive query results per semantic model runtime.", Runtime: true, Lifecycle: "supported", EnvExample: "67108864"},
	{Name: "LIBREDASH_QUERY_CACHE_MAX_ENTRIES", Field: "QueryCacheMaxEntries", Type: TypeInt, Default: "256", Category: "execution", Scope: "serve", Description: "Maximum governed interactive query result entries retained per semantic model runtime.", Runtime: true, Lifecycle: "supported", EnvExample: "256"},
	{Name: "LIBREDASH_GID", Type: TypeInt, Category: "deployment", Scope: "Hetzner provisioner", Description: "Temporary container group identifier used during provisioning.", Lifecycle: "internal"},
	{Name: "LIBREDASH_HEALTHCHECK_URL", Field: "HealthcheckURL", Type: TypeString, Category: "operations", Scope: "healthcheck", Description: "Explicit readiness URL used by the healthcheck command.", Example: "http://127.0.0.1:8080/readyz", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_HOME", Field: "HomeDir", Type: TypeString, Default: ".libredash", Category: "storage", Scope: "serve,admin,client", Description: "Instance state directory containing databases, artifacts, and runtime files.", Example: "/var/lib/libredash", Runtime: true, Lifecycle: "supported", EnvExample: "/var/lib/libredash"},
	{Name: "LIBREDASH_IMAGE", Type: TypeString, Category: "deployment", Scope: "Hetzner provisioner", Description: "Immutable LibreDash OCI image reference consumed by deployment tooling.", Example: "ghcr.io/yacobolo/libredash@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Lifecycle: "tooling"},
	{Name: "LIBREDASH_LOCAL_AUTH", Field: "LocalAuth", Type: TypeBool, Category: "authentication", Scope: "serve", Description: "Enable administrator-managed local browser authentication.", Example: "true", Runtime: true, Lifecycle: "supported", EnvExample: "true"},
	{Name: "LIBREDASH_MANAGED_DATA_BACKEND", Field: "ManagedDataBackend", Type: TypeString, Default: "local", Category: "managed data", Scope: "serve", Description: "Storage backend for project-global managed data; supported values are local and s3.", Runtime: true, Lifecycle: "supported", EnvExample: "local"},
	{Name: "LIBREDASH_MANAGED_DATA_DIR", Field: "ManagedDataDir", Type: TypeString, Default: ".libredash/managed-data", Category: "managed data", Scope: "serve", Description: "Private local root for managed-data objects, upload staging, and verified runtime views; S3 deployments use it as the runtime cache.", Runtime: true, Lifecycle: "supported", EnvExample: "/var/lib/libredash/managed-data"},
	{Name: "LIBREDASH_MANAGED_DATA_GC_GRACE_PERIOD", Field: "ManagedDataGCGracePeriod", Type: TypeDuration, Default: "24h", Category: "managed data", Scope: "serve", Description: "Minimum age of unreferenced managed-data objects before garbage collection.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_GC_INTERVAL", Field: "ManagedDataGCInterval", Type: TypeDuration, Default: "1h", Category: "managed data", Scope: "serve", Description: "Interval between managed-data garbage-collection passes.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_MAX_FILES", Field: "ManagedDataMaxFiles", Type: TypeInt, Default: "10000", Category: "managed data", Scope: "serve", Description: "Maximum number of files in one managed-data revision.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_MAX_FILE_BYTES", Field: "ManagedDataMaxFileBytes", Type: TypeInt64, Default: "1073741824", Category: "managed data", Scope: "serve", Description: "Maximum size in bytes of one managed-data file.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_MAX_REVISION_BYTES", Field: "ManagedDataMaxRevisionBytes", Type: TypeInt64, Default: "10737418240", Category: "managed data", Scope: "serve", Description: "Maximum total size in bytes of one managed-data revision.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_MIN_FREE_BYTES", Field: "ManagedDataMinFreeBytes", Type: TypeInt64, Default: "5368709120", Category: "managed data", Scope: "serve", Description: "Minimum free bytes required before accepting local managed-data uploads.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_S3_ACCESS_KEY_ID", Field: "ManagedDataS3AccessKeyID", Type: TypeString, Category: "managed data", Scope: "serve", Description: "Optional S3 access-key identifier for managed-data storage.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_S3_BUCKET", Field: "ManagedDataS3Bucket", Type: TypeString, Category: "managed data", Scope: "serve", Description: "S3 bucket used for managed-data objects and staging.", Example: "libredash-managed-data", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_S3_ENDPOINT", Field: "ManagedDataS3Endpoint", Type: TypeString, Category: "managed data", Scope: "serve", Description: "Optional S3-compatible endpoint URL.", Example: "https://s3.example.com", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_S3_PATH_STYLE", Field: "ManagedDataS3PathStyle", Type: TypeBool, Default: "false", Category: "managed data", Scope: "serve", Description: "Use path-style addressing for S3-compatible managed-data storage.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_S3_PREFIX", Field: "ManagedDataS3Prefix", Type: TypeString, Default: "managed-data", Category: "managed data", Scope: "serve", Description: "Object-key prefix for managed data in the configured S3 bucket.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_S3_REGION", Field: "ManagedDataS3Region", Type: TypeString, Category: "managed data", Scope: "serve", Description: "S3 region used for managed-data requests.", Example: "eu-west-1", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_S3_SECRET_ACCESS_KEY", Field: "ManagedDataS3SecretAccessKey", Type: TypeString, Category: "managed data", Scope: "serve", Description: "Optional S3 secret access key for managed-data storage.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_S3_SESSION_TOKEN", Field: "ManagedDataS3SessionToken", Type: TypeString, Category: "managed data", Scope: "serve", Description: "Optional temporary S3 session token for managed-data storage.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_MANAGED_DATA_UPLOAD_SESSION_TTL", Field: "ManagedDataUploadSessionTTL", Type: TypeDuration, Default: "24h", Category: "managed data", Scope: "serve", Description: "Lifetime of an incomplete managed-data upload session.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_METRICS_BEARER_TOKEN", Field: "MetricsBearerToken", Type: TypeString, Category: "operations", Scope: "serve", Description: "Bearer token protecting the Prometheus metrics endpoint; production requires at least 32 characters.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", EnvExample: "replace-with-at-least-32-characters"},
	{Name: "LIBREDASH_PERF_ITERATIONS", Type: TypeInt, Default: "5", Category: "development", Scope: "dashboard performance QA", Description: "Measured interaction iterations run by the configured dashboard performance scenario.", Lifecycle: "development"},
	{Name: "LIBREDASH_PERF_ENFORCE_THRESHOLDS", Type: TypeBool, Default: "false", Category: "development", Scope: "dashboard performance QA", Description: "Fail dashboard performance QA when phase latency or query-count thresholds are exceeded.", Lifecycle: "development"},
	{Name: "LIBREDASH_PERF_LOG", Type: TypeString, Default: ".tmp/dev-server.log", Category: "development", Scope: "dashboard performance QA", Description: "Development server log consumed by dashboard performance QA.", Lifecycle: "development"},
	{Name: "LIBREDASH_PERF_MAX_ALL_TARGET_P95_MS", Type: TypeInt, Default: "1000", Category: "development", Scope: "dashboard performance QA", Description: "Maximum all-target settlement p95 when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LIBREDASH_PERF_MAX_CRITICAL_KPI_P95_MS", Type: TypeInt, Default: "1000", Category: "development", Scope: "dashboard performance QA", Description: "Maximum critical-KPI settlement p95 when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LIBREDASH_PERF_MAX_FIRST_TARGET_PAINT_P95_MS", Type: TypeInt, Default: "500", Category: "development", Scope: "dashboard performance QA", Description: "Maximum first-target paint p95 when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LIBREDASH_PERF_MAX_OPTIMISTIC_FEEDBACK_P95_MS", Type: TypeInt, Default: "16", Category: "development", Scope: "dashboard performance QA", Description: "Maximum local optimistic-feedback p95 when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LIBREDASH_PERF_MAX_QUERIES", Type: TypeInt, Default: "4", Category: "development", Scope: "dashboard performance QA", Description: "Maximum physical queries per measured refresh when performance thresholds are enforced.", Lifecycle: "development"},
	{Name: "LIBREDASH_PERF_OUTPUT", Type: TypeString, Category: "development", Scope: "dashboard performance QA", Description: "Optional JSON output path; defaults to a suite-specific file under .tmp.", Lifecycle: "development"},
	{Name: "LIBREDASH_PERF_SCENARIO", Type: TypeString, Default: "scripts/performance/movielens.json", Category: "development", Scope: "dashboard performance QA", Description: "Path to a dashboard performance scenario manifest.", Lifecycle: "development"},
	{Name: "LIBREDASH_OIDC_CALLBACK_URL", Field: "OIDCCallbackURL", Type: TypeString, Category: "authentication", Scope: "serve", Description: "HTTPS callback URL registered with the generic OIDC provider.", Example: "https://libredash.example.com/auth/oidc/callback", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_OIDC_CLIENT_ID", Field: "OIDCClientID", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Generic OIDC client identifier.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_OIDC_CLIENT_SECRET", Field: "OIDCSecret", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Generic OIDC client secret.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_OIDC_ISSUER_URL", Field: "OIDCIssuerURL", Type: TypeString, Category: "authentication", Scope: "serve", Description: "HTTPS issuer URL for the generic OIDC provider.", Example: "https://issuer.example.com", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_OIDC_PROVIDER_ID", Field: "OIDCProviderID", Type: TypeString, Default: "oidc", Category: "authentication", Scope: "serve", Description: "Route-safe identifier for the generic OIDC provider.", Example: "oidc", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_OIDC_SCOPES", Field: "OIDCScopes", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Comma- or whitespace-separated additional OIDC scopes.", Example: "openid profile email", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_PRODUCTION", Field: "Production", Type: TypeBool, Category: "server", Scope: "serve,admin", Description: "Enable production serving and validation behavior.", Default: "false", Runtime: true, Lifecycle: "supported", EnvExample: "1"},
	{Name: "LIBREDASH_QUACK_TOKEN", Type: TypeString, Category: "connection", Scope: "Quack development profile", Description: "Credential injected into the Quack connection declaration.", Example: SecretPlaceholder, Secret: true, Lifecycle: "external"},
	{Name: "LIBREDASH_SCIM_BEARER_TOKEN", Field: "SCIMBearerToken", Type: TypeString, Category: "authentication", Scope: "serve", Description: "Bearer token enabling SCIM provisioning; production requires at least 32 characters when set.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_SITE_BASE_URL", Type: TypeString, Category: "site", Scope: "public site", Description: "Externally visible HTTP(S) origin used for canonical URLs, discovery documents, and transport policy.", Example: "https://docs.libredash.dev", Lifecycle: "supported"},
	{Name: "LIBREDASH_SMOKE_PORT", Type: TypeInt, Default: "18080", Category: "development", Scope: "production image smoke test", Description: "Host port used by the production image smoke test.", Lifecycle: "internal"},
	{Name: "LIBREDASH_TARGET", Field: "Target", Type: TypeString, Category: "client", Scope: "client commands", Description: "Default LibreDash API target URL.", Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_TOKEN_HASH_KEY", Field: "TokenHashKey", Type: TypeString, Category: "security", Scope: "serve", Description: "Optional dedicated key for deterministic API-token fingerprints; falls back to the CSRF key.", Example: SecretPlaceholder, Secret: true, Runtime: true, Lifecycle: "supported", Commented: true},
	{Name: "LIBREDASH_TRUST_PROXY_HEADERS", Field: "TrustProxyHeaders", Type: TypeBool, Category: "security", Scope: "serve", Description: "Trust client-address headers only when a trusted proxy overwrites them.", Default: "false", Runtime: true, Lifecycle: "supported", EnvExample: "false"},
	{Name: "LIBREDASH_UID", Type: TypeInt, Category: "deployment", Scope: "Hetzner provisioner", Description: "Temporary container user identifier used during provisioning.", Lifecycle: "internal"},
	{Name: "LIBREDASH_WAREHOUSE_DSN", Type: TypeString, Category: "connection", Scope: "example connection", Description: "Example externally supplied warehouse connection credential.", Example: SecretPlaceholder, Secret: true, Lifecycle: "external"},
}

type PredicateKind string

const (
	PredicateAll       PredicateKind = "all"
	PredicateAny       PredicateKind = "any"
	PredicateNot       PredicateKind = "not"
	PredicatePresent   PredicateKind = "present"
	PredicateTrue      PredicateKind = "true"
	PredicateMinLength PredicateKind = "min_length"
	PredicateHTTPSURL  PredicateKind = "https_url"
	PredicateSlug      PredicateKind = "route_slug"
	PredicateEquals    PredicateKind = "equals"
	PredicateOneOf     PredicateKind = "one_of"
	PredicatePositive  PredicateKind = "positive"
	PredicateAtLeast   PredicateKind = "at_least_setting"
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
func HTTPSURL(name string) Predicate  { return Predicate{Kind: PredicateHTTPSURL, Name: name} }
func RouteSlug(name string) Predicate { return Predicate{Kind: PredicateSlug, Name: name} }
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
	production    = True("LIBREDASH_PRODUCTION")
	oidcAny       = Any(Present("LIBREDASH_OIDC_ISSUER_URL"), Present("LIBREDASH_OIDC_CLIENT_ID"), Present("LIBREDASH_OIDC_CLIENT_SECRET"), Present("LIBREDASH_OIDC_CALLBACK_URL"), Present("LIBREDASH_OIDC_SCOPES"))
	oidcComplete  = All(Present("LIBREDASH_OIDC_ISSUER_URL"), Present("LIBREDASH_OIDC_CLIENT_ID"), Present("LIBREDASH_OIDC_CLIENT_SECRET"), Present("LIBREDASH_OIDC_CALLBACK_URL"))
	azureAny      = Any(Present("LIBREDASH_AZURE_CLIENT_ID"), Present("LIBREDASH_AZURE_CLIENT_SECRET"), Present("LIBREDASH_AZURE_CALLBACK_URL"), Present("LIBREDASH_AZURE_TENANT"))
	azureComplete = All(Present("LIBREDASH_AZURE_CLIENT_ID"), Present("LIBREDASH_AZURE_CLIENT_SECRET"), Present("LIBREDASH_AZURE_CALLBACK_URL"))
	browserAuth   = Any(True("LIBREDASH_LOCAL_AUTH"), oidcComplete, azureComplete)
	managedData   = Present("LIBREDASH_MANAGED_DATA_BACKEND")
	managedS3     = Equals("LIBREDASH_MANAGED_DATA_BACKEND", "s3")
)

var rules = []Rule{
	{ID: "production-dev-bypass", Description: "Production cannot bypass authentication.", When: production, Assert: Not(True("LIBREDASH_DEV_AUTH_BYPASS")), Message: "production serve must not enable LIBREDASH_DEV_AUTH_BYPASS"},
	{ID: "production-oidc-complete", Description: "OIDC settings are all-or-none in production.", When: All(production, oidcAny), Assert: oidcComplete, Message: "production OIDC auth requires LIBREDASH_OIDC_ISSUER_URL, LIBREDASH_OIDC_CLIENT_ID, LIBREDASH_OIDC_CLIENT_SECRET, and LIBREDASH_OIDC_CALLBACK_URL"},
	{ID: "production-azure-complete", Description: "Azure settings are all-or-none in production.", When: All(production, azureAny), Assert: azureComplete, Message: "production Azure auth requires LIBREDASH_AZURE_CLIENT_ID, LIBREDASH_AZURE_CLIENT_SECRET, and LIBREDASH_AZURE_CALLBACK_URL"},
	{ID: "production-auth-mode", Description: "Production requires local, API-token-only, OIDC, or Azure authentication.", When: production, Assert: Any(True("LIBREDASH_LOCAL_AUTH"), True("LIBREDASH_API_TOKEN_ONLY_AUTH"), oidcComplete, azureComplete), Message: "production serve requires OIDC auth env vars, Azure auth env vars, LIBREDASH_LOCAL_AUTH, or LIBREDASH_API_TOKEN_ONLY_AUTH"},
	{ID: "production-csrf-key", Description: "Production requires a CSRF key with at least 32 characters.", When: production, Assert: MinLength("LIBREDASH_CSRF_KEY", 32), Message: "production serve requires LIBREDASH_CSRF_KEY with at least 32 characters"},
	{ID: "production-metrics-token", Description: "Production requires a metrics bearer token with at least 32 characters.", When: production, Assert: MinLength("LIBREDASH_METRICS_BEARER_TOKEN", 32), Message: "production metrics scraping requires LIBREDASH_METRICS_BEARER_TOKEN with at least 32 characters"},
	{ID: "production-allowed-host", Description: "Production requires an explicit allowed host or a browser-auth callback host.", When: production, Assert: Any(Present("LIBREDASH_ALLOWED_HOSTS"), Present("LIBREDASH_OIDC_CALLBACK_URL"), Present("LIBREDASH_AZURE_CALLBACK_URL")), Message: "production serve requires LIBREDASH_ALLOWED_HOSTS or an OIDC/Azure callback URL host"},
	{ID: "production-secure-cookie", Description: "Production browser authentication requires secure cookies unless API-token-only mode is also enabled.", When: All(production, browserAuth, Not(True("LIBREDASH_API_TOKEN_ONLY_AUTH"))), Assert: True("LIBREDASH_COOKIE_SECURE"), Message: "production browser auth requires LIBREDASH_COOKIE_SECURE=true"},
	{ID: "production-oidc-issuer-https", Description: "The production OIDC issuer must use HTTPS.", When: All(production, oidcComplete), Assert: HTTPSURL("LIBREDASH_OIDC_ISSUER_URL"), Message: "production serve requires LIBREDASH_OIDC_ISSUER_URL to be an https URL"},
	{ID: "production-oidc-callback-https", Description: "The production OIDC callback must use HTTPS.", When: All(production, oidcComplete), Assert: HTTPSURL("LIBREDASH_OIDC_CALLBACK_URL"), Message: "production serve requires LIBREDASH_OIDC_CALLBACK_URL to be an https URL"},
	{ID: "production-oidc-provider-slug", Description: "The OIDC provider identifier must be route-safe.", When: All(production, oidcComplete), Assert: RouteSlug("LIBREDASH_OIDC_PROVIDER_ID"), Message: "LIBREDASH_OIDC_PROVIDER_ID must be a route-safe slug containing only letters, numbers, dots, underscores, or dashes"},
	{ID: "production-azure-callback-https", Description: "The production Azure callback must use HTTPS.", When: All(production, azureComplete), Assert: HTTPSURL("LIBREDASH_AZURE_CALLBACK_URL"), Message: "production serve requires LIBREDASH_AZURE_CALLBACK_URL to be an https URL"},
	{ID: "production-scim-token", Description: "A configured production SCIM token must contain at least 32 characters.", When: All(production, Present("LIBREDASH_SCIM_BEARER_TOKEN")), Assert: MinLength("LIBREDASH_SCIM_BEARER_TOKEN", 32), Message: "production SCIM provisioning requires LIBREDASH_SCIM_BEARER_TOKEN with at least 32 characters"},
	{ID: "managed-data-backend", Description: "Managed data uses a supported storage backend.", When: managedData, Assert: OneOf("LIBREDASH_MANAGED_DATA_BACKEND", "local", "s3"), Message: "LIBREDASH_MANAGED_DATA_BACKEND must be local or s3"},
	{ID: "managed-data-runtime-dir", Description: "Every managed-data backend requires a private local runtime and staging directory.", When: managedData, Assert: Present("LIBREDASH_MANAGED_DATA_DIR"), Message: "managed-data storage requires LIBREDASH_MANAGED_DATA_DIR"},
	{ID: "managed-data-s3-location", Description: "The S3 managed-data backend requires a bucket and region.", When: managedS3, Assert: All(Present("LIBREDASH_MANAGED_DATA_S3_BUCKET"), Present("LIBREDASH_MANAGED_DATA_S3_REGION")), Message: "S3 managed-data storage requires LIBREDASH_MANAGED_DATA_S3_BUCKET and LIBREDASH_MANAGED_DATA_S3_REGION"},
	{ID: "managed-data-s3-credentials", Description: "Managed-data S3 credentials are either omitted or configured as a complete key pair.", When: managedS3, Assert: Any(All(Not(Present("LIBREDASH_MANAGED_DATA_S3_ACCESS_KEY_ID")), Not(Present("LIBREDASH_MANAGED_DATA_S3_SECRET_ACCESS_KEY")), Not(Present("LIBREDASH_MANAGED_DATA_S3_SESSION_TOKEN"))), All(Present("LIBREDASH_MANAGED_DATA_S3_ACCESS_KEY_ID"), Present("LIBREDASH_MANAGED_DATA_S3_SECRET_ACCESS_KEY"))), Message: "managed-data S3 credentials require both LIBREDASH_MANAGED_DATA_S3_ACCESS_KEY_ID and LIBREDASH_MANAGED_DATA_S3_SECRET_ACCESS_KEY; a session token also requires that pair"},
	{ID: "managed-data-positive-limits", Description: "Managed-data upload, session, garbage-collection, and free-space limits are positive.", When: managedData, Assert: All(Positive("LIBREDASH_MANAGED_DATA_MAX_FILES"), Positive("LIBREDASH_MANAGED_DATA_MAX_FILE_BYTES"), Positive("LIBREDASH_MANAGED_DATA_MAX_REVISION_BYTES"), Positive("LIBREDASH_MANAGED_DATA_UPLOAD_SESSION_TTL"), Positive("LIBREDASH_MANAGED_DATA_GC_INTERVAL"), Positive("LIBREDASH_MANAGED_DATA_GC_GRACE_PERIOD"), Positive("LIBREDASH_MANAGED_DATA_MIN_FREE_BYTES")), Message: "managed-data limits, durations, and free-space thresholds must be positive"},
	{ID: "managed-data-revision-limit", Description: "The managed-data revision limit is at least the per-file limit.", When: managedData, Assert: AtLeast("LIBREDASH_MANAGED_DATA_MAX_REVISION_BYTES", "LIBREDASH_MANAGED_DATA_MAX_FILE_BYTES"), Message: "LIBREDASH_MANAGED_DATA_MAX_REVISION_BYTES must be at least LIBREDASH_MANAGED_DATA_MAX_FILE_BYTES"},
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
