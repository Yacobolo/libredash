package hetzner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTerraformProductionContracts(t *testing.T) {
	variables := readFile(t, "variables.tf")
	main := readFile(t, "main.tf")
	cloudInit := readFile(t, "cloud-init.yaml.tftpl")
	requireContains(t, variables, `variable "libredash_image"`)
	requireContains(t, variables, `@sha256:`)
	requireContains(t, variables, `variable "ssh_allowed_cidrs"`)
	if strings.Contains(variables, `default     = ["0.0.0.0/0", "::/0"]`) {
		t.Fatal("SSH must not be open to the world by default")
	}
	if strings.Contains(variables, `variable "repo_ref"`) || strings.Contains(variables, `variable "repo_url"`) {
		t.Fatal("production deployment must consume an image, not mutable source refs")
	}
	requireMatch(t, main, `(?m)^\s*backups\s*=\s*true\s*$`)
	requireMatch(t, main, `(?m)^\s*shutdown_before_deletion\s*=\s*true\s*$`)
	if strings.Contains(cloudInit, "libredashctl_b64") {
		t.Fatal("cloud-init must not embed a source-tree controller script")
	}
	if strings.Contains(cloudInit, "git clone") || strings.Contains(cloudInit, "docker build") {
		t.Fatal("cloud-init must not clone or build application source")
	}
}

func TestHetznerConsumesGenericComposeLifecycle(t *testing.T) {
	main := readFile(t, "main.tf")
	cloudInit := readFile(t, "cloud-init.yaml.tftpl")
	provision := readFile(t, filepath.Join("files", "provision.sh.tftpl"))
	for _, fragment := range []string{
		`${path.module}/../compose/compose.yaml`,
		`${path.module}/../compose/compose.https.yaml`,
		`${path.module}/../compose/deployment.env.example`,
	} {
		requireContains(t, main, fragment)
	}
	for _, fragment := range []string{"compose_https_b64", "deployment_example_b64", "libredashctl_wrapper_b64", "backup_hook_b64"} {
		requireContains(t, cloudInit, fragment)
	}
	requireContains(t, provision, `docker cp "$controller_container:/usr/local/libexec/libredashctl" /opt/libredash/libredashctl`)
	extract := strings.Index(provision, `docker cp "$controller_container:/usr/local/libexec/libredashctl"`)
	initialize := strings.Index(provision, `libredashctl init`)
	start := strings.Index(provision, `libredashctl start`)
	if extract < 0 || initialize < 0 || start < 0 || extract > initialize || initialize > start {
		t.Fatal("generic offline initialization must complete before start")
	}
	for _, forbidden := range []string{"/api/v1/me/api-tokens", "/api/v1/principals", "admin bootstrap", "docker compose"} {
		if strings.Contains(provision, forbidden) {
			t.Fatalf("provider provisioning maintains a separate lifecycle path %q", forbidden)
		}
	}
}

func TestProviderScriptsAreSmallValidLayers(t *testing.T) {
	for _, script := range []string{
		filepath.Join("files", "libredashctl-wrapper"),
		filepath.Join("files", "libredash-backup-hook"),
		filepath.Join("files", "provision.sh.tftpl"),
	} {
		if output, err := exec.Command("bash", "-n", script).CombinedOutput(); err != nil {
			t.Fatalf("bash -n %s: %v\n%s", script, err, output)
		}
	}
	wrapper := readFile(t, filepath.Join("files", "libredashctl-wrapper"))
	requireContains(t, wrapper, "LIBREDASHCTL_ROOT=/opt/libredash")
	requireContains(t, wrapper, "exec /opt/libredash/libredashctl")
	hook := readFile(t, filepath.Join("files", "libredash-backup-hook"))
	for _, fragment := range []string{"restic backup", "--keep-daily 7", "--keep-weekly 4", "--keep-monthly 12", "rm -f"} {
		requireContains(t, hook, fragment)
	}
}

func TestReleaseWorkflowPublishesComposeArchiveAndAttestedImage(t *testing.T) {
	workflow := readFile(t, filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	for _, fragment := range []string{
		"release:", "packages: write", "attestations: write", "id-token: write",
		"docker/build-push-action@", "actions/attest@", "push-to-registry: true",
		"libredash-compose-", "deployment.env.example", ".tar.gz.sha256", "./cmd/libredashctl",
	} {
		requireContains(t, workflow, fragment)
	}
}

func TestSupplyChainInputsArePinned(t *testing.T) {
	for _, name := range []string{"Dockerfile", "Dockerfile.site"} {
		dockerfile := readFile(t, filepath.Join("..", "..", name))
		if !strings.HasPrefix(dockerfile, "# syntax=docker/dockerfile:1.7@sha256:") {
			t.Errorf("%s frontend is not pinned by digest", name)
		}
		assertDockerfileImagesPinned(t, name, dockerfile)
	}
	workflows, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	mutableAction := regexp.MustCompile(`(?m)^\s*uses:\s+[^#\s]+@v[0-9]+(?:\s|$)`)
	for _, workflow := range workflows {
		if match := mutableAction.FindString(readFile(t, workflow)); match != "" {
			t.Errorf("GitHub Action is not pinned by commit in %s: %s", workflow, strings.TrimSpace(match))
		}
	}
}

func assertDockerfileImagesPinned(t *testing.T, name, dockerfile string) {
	t.Helper()
	stages := make(map[string]struct{})
	hexDigest := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, line := range strings.Split(dockerfile, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "FROM" {
			continue
		}
		image := fields[1]
		if _, internal := stages[image]; !internal {
			_, digest, pinned := strings.Cut(image, "@sha256:")
			if !pinned || !hexDigest.MatchString(digest) {
				t.Errorf("%s base image is not pinned by a valid SHA-256 digest: %s", name, line)
			}
		}
		if len(fields) >= 4 && fields[2] == "AS" {
			stages[fields[3]] = struct{}{}
		}
	}
}

func TestEphemeralDeploymentExercisesPublicAndBackupContracts(t *testing.T) {
	workflow := readFile(t, filepath.Join("..", "..", ".github", "workflows", "hetzner-deploy.yml"))
	for _, fragment := range []string{
		"workflow_dispatch:", "environment: hetzner-deployment", "terraform apply",
		"public_ready=false", "--connect-timeout 5", "libredashctl backup", "libredashctl restore",
		`.publisherToken`, "if: always()", "terraform destroy",
	} {
		requireContains(t, workflow, fragment)
	}
	if strings.Contains(workflow, "pull_request:") {
		t.Fatal("cloud deployment must require an explicit, environment-protected dispatch")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func requireContains(t *testing.T, contents, fragment string) {
	t.Helper()
	if !strings.Contains(contents, fragment) {
		t.Fatalf("missing %q", fragment)
	}
}

func requireMatch(t *testing.T, contents, pattern string) {
	t.Helper()
	if !regexp.MustCompile(pattern).MatchString(contents) {
		t.Fatalf("missing pattern %q", pattern)
	}
}
