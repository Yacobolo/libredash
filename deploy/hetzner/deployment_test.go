package hetzner_test

import (
	"bytes"
	"fmt"
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
	requireContains(t, cloudInit, "libredashctl_b64")
	if strings.Contains(cloudInit, "git clone") || strings.Contains(cloudInit, "docker build") {
		t.Fatal("cloud-init must not clone or build application source")
	}
	if strings.Contains(cloudInit, "libredash-bootstrap-token") {
		t.Fatal("bootstrap administrator tokens must not persist on disk")
	}
}

func TestComposeSecurityContracts(t *testing.T) {
	compose := readFile(t, filepath.Join("files", "compose.yaml"))

	for _, fragment := range []string{
		"image: ${LIBREDASH_IMAGE}",
		"image: ${CADDY_IMAGE}",
		"127.0.0.1:8080:8080",
		"read_only: true",
		"no-new-privileges:true",
		"cap_drop:",
		"pids_limit:",
		"max-size:",
		"stop_grace_period:",
	} {
		requireContains(t, compose, fragment)
	}
}

func TestReleaseWorkflowPublishesAttestedImage(t *testing.T) {
	workflow := readFile(t, filepath.Join("..", "..", ".github", "workflows", "release.yml"))

	for _, fragment := range []string{
		"release:",
		"packages: write",
		"attestations: write",
		"id-token: write",
		"docker/build-push-action@",
		"actions/attest@",
		"push-to-registry: true",
	} {
		requireContains(t, workflow, fragment)
	}
}

func TestSupplyChainInputsArePinned(t *testing.T) {
	dockerfile := readFile(t, filepath.Join("..", "..", "Dockerfile"))
	if !strings.HasPrefix(dockerfile, "# syntax=docker/dockerfile:1.7@sha256:") {
		t.Error("Dockerfile frontend is not pinned by digest")
	}
	for _, line := range strings.Split(dockerfile, "\n") {
		if strings.HasPrefix(line, "FROM ") && !strings.Contains(line, "@sha256:") {
			t.Errorf("Docker base image is not pinned by digest: %s", line)
		}
	}

	workflows, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	mutableAction := regexp.MustCompile(`(?m)^\s*uses:\s+[^#\s]+@v[0-9]+(?:\s|$)`)
	for _, workflow := range workflows {
		contents := readFile(t, workflow)
		if match := mutableAction.FindString(contents); match != "" {
			t.Errorf("GitHub Action is not pinned by commit in %s: %s", workflow, strings.TrimSpace(match))
		}
	}
}

func TestEphemeralDeploymentWorkflowAlwaysDestroysInfrastructure(t *testing.T) {
	workflow := readFile(t, filepath.Join("..", "..", ".github", "workflows", "hetzner-deploy.yml"))

	for _, fragment := range []string{
		"workflow_dispatch:",
		"environment: hetzner-deployment",
		"terraform apply",
		"libredashctl backup",
		"libredashctl restore",
		`.principal.email`,
		"if: always()",
		"terraform destroy",
	} {
		requireContains(t, workflow, fragment)
	}
	if strings.Contains(workflow, "pull_request:") {
		t.Fatal("cloud deployment must require an explicit, environment-protected dispatch")
	}
}

func TestOperationsScriptSyntaxAndCommands(t *testing.T) {
	script := filepath.Join("files", "libredashctl")
	contents := readFile(t, script)

	for _, command := range []string{"first-login", "status", "logs", "backup", "restore", "upgrade", "rollback", "restic.env"} {
		requireContains(t, contents, command)
	}

	cmd := exec.Command("bash", "-n", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n %s: %v\n%s", script, err, output)
	}
}

func TestProvisionCreatesPublisherBeforeMandatoryPassword(t *testing.T) {
	script := readFile(t, filepath.Join("files", "provision.sh.tftpl"))
	publisher := strings.Index(script, `http://127.0.0.1:8080/api/v1/me/api-tokens`)
	localUser := strings.Index(script, `http://127.0.0.1:8080/api/v1/principals`)
	if publisher < 0 || localUser < 0 {
		t.Fatal("provisioning endpoints are missing")
	}
	if publisher > localUser {
		t.Fatal("publisher token must be created before the local credential enforces a password change")
	}
}

func TestFirstLoginCredentialsAreConsumed(t *testing.T) {
	script := filepath.Join("files", "libredashctl")
	_ = readFile(t, script)

	configDir := t.TempDir()
	credentials := filepath.Join(configDir, "initial-local-user.json")
	want := `{"email":"admin@example.com","temporaryPassword":"temporary-secret"}`
	if err := os.WriteFile(credentials, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", script, "first-login")
	cmd.Env = append(os.Environ(), "LIBREDASHCTL_CONFIG_DIR="+configDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("first-login: %v\n%s", err, output)
	}
	if strings.TrimSpace(string(output)) != want {
		t.Fatalf("first-login output = %q, want %q", output, want)
	}
	if _, err := os.Stat(credentials); !os.IsNotExist(err) {
		t.Fatalf("credentials still exist after retrieval: %v", err)
	}

	cmd = exec.Command("bash", script, "first-login")
	cmd.Env = append(os.Environ(), "LIBREDASHCTL_CONFIG_DIR="+configDir)
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("second first-login succeeded unexpectedly: %s", output)
	}
}

func TestUpgradeRejectsMutableImageReference(t *testing.T) {
	script := filepath.Join("files", "libredashctl")
	_ = readFile(t, script)

	cmd := exec.Command("bash", script, "upgrade", "ghcr.io/yacobolo/libredash:latest")
	cmd.Env = append(os.Environ(), "LIBREDASHCTL_CONFIG_DIR="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("upgrade accepted mutable image reference: %s", output)
	}
	if !strings.Contains(string(output), "digest") {
		t.Fatalf("upgrade diagnostic does not explain digest requirement: %s", output)
	}
}

func TestUpgradeAndRollbackUseImmutableImages(t *testing.T) {
	env := newOperationsEnvironment(t)
	current := imageDigest('a')
	target := imageDigest('b')
	env.writeDeployment(current)

	output := env.run(t, "upgrade", target)
	if !strings.Contains(output, "upgraded to "+target) {
		t.Fatalf("upgrade output = %q", output)
	}
	if got := env.deployedImage(t); got != target {
		t.Fatalf("deployed image = %q, want %q", got, target)
	}
	if got := strings.TrimSpace(readFile(t, env.previousImage)); got != current {
		t.Fatalf("previous image = %q, want %q", got, current)
	}

	env.run(t, "rollback")
	if got := env.deployedImage(t); got != current {
		t.Fatalf("rolled-back image = %q, want %q", got, current)
	}
}

func TestFailedUpgradeRestoresPreviousImage(t *testing.T) {
	env := newOperationsEnvironment(t)
	current := imageDigest('a')
	target := imageDigest('b')
	env.writeDeployment(current)
	env.extra = append(env.extra, "FAKE_HEALTH_FAIL_IMAGE="+target)

	output, err := env.command("upgrade", target).CombinedOutput()
	if err == nil {
		t.Fatalf("failed-health upgrade succeeded: %s", output)
	}
	if !strings.Contains(string(output), "rolled back") {
		t.Fatalf("upgrade output does not report rollback: %s", output)
	}
	if got := env.deployedImage(t); got != current {
		t.Fatalf("image after failed upgrade = %q, want %q", got, current)
	}
}

func TestBackupAndRestoreRoundTrip(t *testing.T) {
	env := newOperationsEnvironment(t)
	env.writeDeployment(imageDigest('a'))
	stateFile := filepath.Join(env.stateDir, "platform.db")
	if err := os.WriteFile(stateFile, []byte("before-backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(env.backupDir, "round-trip.tar.gz")
	cmd := env.command("backup", archive)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("backup: %v\n%s", err, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != archive {
		t.Fatalf("backup stdout = %q, want only archive path", stdout.String())
	}
	if _, err := os.Stat(archive + ".sha256"); err != nil {
		t.Fatalf("backup checksum: %v", err)
	}
	if err := os.WriteFile(stateFile, []byte("after-backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	env.run(t, "restore", archive)
	contents, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "before-backup" {
		t.Fatalf("restored contents = %q", contents)
	}
}

func TestFailedRestoreReinstatesCurrentState(t *testing.T) {
	env := newOperationsEnvironment(t)
	env.writeDeployment(imageDigest('a'))
	stateFile := filepath.Join(env.stateDir, "platform.db")
	if err := os.WriteFile(stateFile, []byte("backup-state"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(env.backupDir, "failed-restore.tar.gz")
	env.run(t, "backup", archive)
	if err := os.WriteFile(stateFile, []byte("current-state"), 0o600); err != nil {
		t.Fatal(err)
	}

	failOnce := filepath.Join(env.configDir, "fail-health-once")
	env.extra = append(env.extra, "FAKE_HEALTH_FAIL_ONCE_FILE="+failOnce)
	output, err := env.command("restore", archive).CombinedOutput()
	if err == nil {
		t.Fatalf("failed-health restore succeeded: %s", output)
	}
	contents, readErr := os.ReadFile(stateFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "current-state" {
		t.Fatalf("state after failed restore = %q, want current state", contents)
	}
}

type operationsEnvironment struct {
	script        string
	configDir     string
	deployDir     string
	stateDir      string
	backupDir     string
	previousImage string
	fakeBinDir    string
	extra         []string
}

func newOperationsEnvironment(t *testing.T) *operationsEnvironment {
	t.Helper()
	root := t.TempDir()
	env := &operationsEnvironment{
		script:        filepath.Join("files", "libredashctl"),
		configDir:     filepath.Join(root, "config"),
		deployDir:     filepath.Join(root, "deploy"),
		stateDir:      filepath.Join(root, "state"),
		backupDir:     filepath.Join(root, "backups"),
		previousImage: filepath.Join(root, "config", "previous-image"),
		fakeBinDir:    filepath.Join(root, "bin"),
	}
	for _, dir := range []string{env.configDir, env.deployDir, env.stateDir, env.backupDir, env.fakeBinDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(env.deployDir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.configDir, "libredash.env"), []byte("LIBREDASH_PRODUCTION=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fakeDocker := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$FAKE_DOCKER_LOG"
case "$*" in
  compose*) echo "compose progress" ;;
  "inspect --format {{.State.Running}} libredash") echo true ;;
  "inspect --format {{.State.Health.Status}} libredash")
    image="$(awk -F= '$1 == "LIBREDASH_IMAGE" {print $2}' "$FAKE_DEPLOYMENT_ENV")"
    if [[ -n "${FAKE_HEALTH_FAIL_ONCE_FILE:-}" && ! -e "$FAKE_HEALTH_FAIL_ONCE_FILE" ]]; then
      touch "$FAKE_HEALTH_FAIL_ONCE_FILE"
      echo unhealthy
    elif [[ -n "${FAKE_HEALTH_FAIL_IMAGE:-}" && "$image" == "$FAKE_HEALTH_FAIL_IMAGE" ]]; then
      echo unhealthy
    else
      echo healthy
    fi
    ;;
  *"id -u libredash"*) id -u ;;
  *"id -g libredash"*) id -g ;;
esac
`
	writeExecutable(t, filepath.Join(env.fakeBinDir, "docker"), fakeDocker)
	writeExecutable(t, filepath.Join(env.fakeBinDir, "flock"), "#!/usr/bin/env bash\nexit 0\n")
	writeExecutable(t, filepath.Join(env.fakeBinDir, "sha256sum"), `#!/usr/bin/env bash
set -euo pipefail
for file in "$@"; do
  hash="$(shasum -a 256 "$file" | awk '{print $1}')"
  printf '%s  %s\n' "$hash" "$file"
done
`)
	return env
}

func (e *operationsEnvironment) writeDeployment(image string) {
	contents := fmt.Sprintf("LIBREDASH_IMAGE=%s\nCADDY_IMAGE=%s\nCADDY_DOMAIN=example.com\nLIBREDASH_UID=%d\nLIBREDASH_GID=%d\n", image, imageDigest('c'), os.Getuid(), os.Getgid())
	_ = os.WriteFile(filepath.Join(e.configDir, "deployment.env"), []byte(contents), 0o600)
}

func (e *operationsEnvironment) command(args ...string) *exec.Cmd {
	cmd := exec.Command("bash", append([]string{e.script}, args...)...)
	cmd.Env = append(os.Environ(), []string{
		"PATH=" + e.fakeBinDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"LIBREDASHCTL_CONFIG_DIR=" + e.configDir,
		"LIBREDASHCTL_DEPLOY_DIR=" + e.deployDir,
		"LIBREDASHCTL_STATE_DIR=" + e.stateDir,
		"LIBREDASHCTL_BACKUP_DIR=" + e.backupDir,
		"LIBREDASHCTL_DOCKER_BIN=" + filepath.Join(e.fakeBinDir, "docker"),
		"LIBREDASHCTL_LOCK_FILE=" + filepath.Join(e.configDir, "operation.lock"),
		"LIBREDASHCTL_HEALTH_ATTEMPTS=1",
		"FAKE_DOCKER_LOG=" + filepath.Join(e.configDir, "docker.log"),
		"FAKE_DEPLOYMENT_ENV=" + filepath.Join(e.configDir, "deployment.env"),
	}...)
	cmd.Env = append(cmd.Env, e.extra...)
	return cmd
}

func (e *operationsEnvironment) run(t *testing.T, args ...string) string {
	t.Helper()
	output, err := e.command(args...).CombinedOutput()
	if err != nil {
		t.Fatalf("libredashctl %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func (e *operationsEnvironment) deployedImage(t *testing.T) string {
	t.Helper()
	contents := readFile(t, filepath.Join(e.configDir, "deployment.env"))
	for _, line := range strings.Split(contents, "\n") {
		if strings.HasPrefix(line, "LIBREDASH_IMAGE=") {
			return strings.TrimPrefix(line, "LIBREDASH_IMAGE=")
		}
	}
	t.Fatal("deployment environment has no LIBREDASH_IMAGE")
	return ""
}

func imageDigest(fill rune) string {
	return "ghcr.io/yacobolo/libredash@sha256:" + strings.Repeat(string(fill), 64)
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
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
