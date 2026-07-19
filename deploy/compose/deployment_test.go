package compose

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeSingleInstanceContract(t *testing.T) {
	compose := read(t, "compose.yaml")
	for _, required := range []string{
		"libredash-state:/var/lib/libredash",
		"${COMPOSE_APP_BIND:-127.0.0.1:8080}:8080",
		"read_only: true",
		"cap_drop: [ALL]",
		"stop_grace_period: 2m",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("compose.yaml missing %q", required)
		}
	}
	if strings.Contains(compose, "container_name:") {
		t.Fatal("generic Compose must allow independent project names on one host")
	}
	if strings.Contains(compose, "./backups:/backups") {
		t.Fatal("backup archives must cross the container boundary as streams, not through a host bind with incompatible ownership")
	}
	if !strings.Contains(read(t, "libredash.env.example"), "LIBREDASH_HOME=/var/lib/libredash/home") {
		t.Fatal("libredash.env.example must place LIBREDASH_HOME beneath the mounted volume so restore can replace it")
	}
	https := read(t, "compose.https.yaml")
	if !strings.Contains(https, "CADDY_IMAGE") || !strings.Contains(https, "443:443/udp") {
		t.Fatal("HTTPS overlay is incomplete")
	}
}

func TestPublicImageIsPrimaryOnboardingContract(t *testing.T) {
	release := read(t, filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	for _, required := range []string{
		"IMAGE_NAME: ghcr.io/yacobolo/libredash",
		"docker/setup-qemu-action@",
		"type=raw,value=latest",
		"platforms: linux/amd64,linux/arm64",
		"Verify anonymous image pull",
		"docker logout ghcr.io",
		"docker buildx imagetools inspect",
	} {
		if !strings.Contains(release, required) {
			t.Fatalf("release workflow missing public image contract %q", required)
		}
	}
	if strings.Index(release, "docker/setup-qemu-action@") > strings.Index(release, "docker/setup-buildx-action@") {
		t.Fatal("release workflow must install emulation before creating the multi-platform builder")
	}

	for _, name := range []string{
		filepath.Join("..", "..", "README.md"),
		filepath.Join("..", "..", "docs", "articles", "start", "installation.md"),
	} {
		document := read(t, name)
		image := strings.Index(document, "ghcr.io/yacobolo/libredash:latest")
		pull := strings.Index(document, "docker pull")
		initialize := strings.Index(document, "admin initialize --format json")
		controller := strings.Index(document, "./libredashctl init")
		if image < 0 || pull < 0 || initialize < 0 {
			t.Errorf("%s does not document pull-first public image onboarding", name)
		}
		if controller >= 0 && image > controller {
			t.Errorf("%s presents the operations controller before the public image", name)
		}
	}
}

func TestControllerBuildAndLifecycleCommands(t *testing.T) {
	root := t.TempDir()
	controller := buildController(t, root)
	output, err := exec.Command(controller, "help").CombinedOutput()
	if err != nil {
		t.Fatalf("libredashctl help: %v\n%s", err, output)
	}
	for _, command := range []string{"init", "start", "status", "logs", "first-login", "backup", "restore", "upgrade", "rollback"} {
		if !strings.Contains(string(output), command) {
			t.Fatalf("controller help missing %s:\n%s", command, output)
		}
	}
}

func TestControllerReleasePackagingContract(t *testing.T) {
	release := read(t, filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	for _, required := range []string{
		"./cmd/libredashctl",
		"CGO_ENABLED=0",
		"linux amd64",
		"linux arm64",
		"darwin amd64",
		"darwin arm64",
	} {
		if !strings.Contains(release, required) {
			t.Fatalf("release workflow missing Go controller packaging contract %q", required)
		}
	}
	dockerfile := read(t, filepath.Join("..", "..", "Dockerfile"))
	if !strings.Contains(dockerfile, "/usr/local/libexec/libredashctl") {
		t.Fatal("application image must carry the matching Linux controller for provider extraction")
	}
}

func TestControllerLifecycleWithStateAwareUpgradeRollback(t *testing.T) {
	root := t.TempDir()
	buildController(t, root)
	copyDeploymentFile(t, root, "deployment.env.example", 0o600)
	fakeDocker := filepath.Join(root, "fake-docker")
	if err := os.WriteFile(fakeDocker, []byte(`#!/usr/bin/env bash
set -euo pipefail
root="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
printf '%s\n' "$*" >>"$root/docker.log"
if [[ -n "${FAKE_DOCKER_FAIL_COMMAND:-}" && " $* " == *" ${FAKE_DOCKER_FAIL_COMMAND} "* ]]; then exit 42; fi
if [[ "${FAKE_DOCKER_FAIL_RESTORE_ONCE:-}" == 1 && " $* " == *' admin restore '* && ! -e "$root/restore-failed-once" ]]; then
  touch "$root/restore-failed-once"
  exit 42
fi
if [[ "${1:-}" == inspect ]]; then
  template="${3:-}"
  if [[ "$template" == *Running* ]]; then printf 'true\n'; exit 0; fi
  image="$(awk -F= '$1=="LIBREDASH_IMAGE" {sub(/^[^=]*=/, ""); print; exit}' "$root/deployment.env")"
  if [[ -n "${FAKE_DOCKER_FAIL_IMAGE:-}" && "$image" == "$FAKE_DOCKER_FAIL_IMAGE" ]]; then printf 'unhealthy\n'; else printf 'healthy\n'; fi
  exit 0
fi
[[ "${1:-}" == compose ]] || exit 0
shift
while [[ $# -gt 0 ]]; do
  case "$1" in
    --project-directory|--env-file|-f) shift 2 ;;
    *) command="$1"; shift; break ;;
  esac
done
case "${command:-}" in
  ps) [[ " $* " == *' -q '* ]] && printf 'fake-container\n' ;;
  run)
    if [[ " $* " == *' admin initialize --format json '* ]]; then
      printf '{"email":"admin@example.com","temporaryPassword":"temporary","publisherToken":"publisher","publisherTokenExpiresAt":"2026-07-19T00:00:00Z"}\n'
    elif [[ " $* " == *' admin backup '* ]]; then
      output=""
      while [[ $# -gt 0 ]]; do
        if [[ "$1" == --out ]]; then output="$2"; break; fi
        shift
      done
      if [[ "$output" == - ]]; then
        printf 'validated archive\n'
      else
        output="$root/${output#/}"
        mkdir -p "$(dirname -- "$output")"
        printf 'validated archive\n' >"$output"
      fi
    fi
    ;;
esac
`), 0o700); err != nil {
		t.Fatal(err)
	}

	oldImage := "example.com/libredash@sha256:" + strings.Repeat("a", 64)
	newImage := "example.com/libredash@sha256:" + strings.Repeat("b", 64)
	runController(t, root, fakeDocker, "", "init", "--admin-email", "admin@example.com", "--domain", "dash.example.com", "--image", oldImage)
	for _, name := range []string{"deployment.env", "libredash.env", "initial-credentials.json"} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions = %v, %v", name, info.Mode().Perm(), err)
		}
	}
	if output := runController(t, root, fakeDocker, "", "first-login"); !strings.Contains(output, `"temporaryPassword":"temporary"`) {
		t.Fatalf("first-login output = %s", output)
	}
	if _, err := os.Stat(filepath.Join(root, "initial-credentials.json")); !os.IsNotExist(err) {
		t.Fatalf("one-time credentials were not deleted: %v", err)
	}
	runController(t, root, fakeDocker, "", "start")
	t.Setenv("FAKE_DOCKER_FAIL_COMMAND", "admin backup")
	if output, err := runControllerResult(root, fakeDocker, "", "backup"); err == nil || !strings.Contains(output, "previous service state was restored") {
		t.Fatalf("failed backup result = %v, %s", err, output)
	}
	t.Setenv("FAKE_DOCKER_FAIL_COMMAND", "")
	backupOutput := runController(t, root, fakeDocker, "", "backup")
	backupPath := strings.TrimSpace(backupOutput)
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup missing: %v (%s)", err, backupOutput)
	}
	runController(t, root, fakeDocker, "", "restore", backupPath)
	t.Setenv("FAKE_DOCKER_FAIL_COMMAND", "pull libredash")
	if output, err := runControllerResult(root, fakeDocker, "", "upgrade", newImage); err == nil || !strings.Contains(output, "previous image and service state were restored") {
		t.Fatalf("failed pull result = %v, %s", err, output)
	}
	requireDeploymentImage(t, root, oldImage)
	t.Setenv("FAKE_DOCKER_FAIL_COMMAND", "")

	output, err := runControllerResult(root, fakeDocker, newImage, "upgrade", newImage)
	if err == nil || !strings.Contains(output, "previous image and state were restored") {
		t.Fatalf("failed upgrade result = %v, %s", err, output)
	}
	requireDeploymentImage(t, root, oldImage)
	runController(t, root, fakeDocker, "", "upgrade", newImage)
	requireDeploymentImage(t, root, newImage)
	t.Setenv("FAKE_DOCKER_FAIL_RESTORE_ONCE", "1")
	if output, err := runControllerResult(root, fakeDocker, "", "rollback", "--confirm"); err == nil || !strings.Contains(output, "pre-rollback image and state were reinstated") {
		t.Fatalf("failed rollback result = %v, %s", err, output)
	}
	requireDeploymentImage(t, root, newImage)
	t.Setenv("FAKE_DOCKER_FAIL_RESTORE_ONCE", "")
	runController(t, root, fakeDocker, "", "rollback", "--confirm")
	requireDeploymentImage(t, root, oldImage)
	log, err := os.ReadFile(filepath.Join(root, "docker.log"))
	if err != nil || !strings.Contains(string(log), "admin restore") {
		t.Fatalf("controller did not restore paired state: %v\n%s", err, log)
	}
}

func TestControllerInitializationIsRetryableAndRequiresPinnedProxy(t *testing.T) {
	image := "example.com/libredash@sha256:" + strings.Repeat("a", 64)
	setup := func(t *testing.T) (string, string) {
		t.Helper()
		root := t.TempDir()
		buildController(t, root)
		copyDeploymentFile(t, root, "deployment.env.example", 0o600)
		fakeDocker := filepath.Join(root, "fake-docker")
		if err := os.WriteFile(fakeDocker, []byte(`#!/usr/bin/env bash
set -euo pipefail
root="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
if [[ -f "$root/fail-validation" && " $* " == *" config validate "* ]]; then exit 42; fi
if [[ " $* " == *" admin initialize --format json "* ]]; then
  printf '{"email":"admin@example.com","temporaryPassword":"temporary","publisherToken":"publisher","publisherTokenExpiresAt":"2026-07-19T00:00:00Z"}\n'
fi
`), 0o700); err != nil {
			t.Fatal(err)
		}
		return root, fakeDocker
	}

	t.Run("retry after validation failure", func(t *testing.T) {
		root, fakeDocker := setup(t)
		if err := os.WriteFile(filepath.Join(root, "fail-validation"), []byte("fail\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := runControllerResult(root, fakeDocker, "", "init", "--admin-email", "admin@example.com", "--domain", "dash.example.com", "--image", image); err == nil || !strings.Contains(output, "initialization can be retried") {
			t.Fatalf("failed initialization = %v, %s", err, output)
		}
		for _, name := range []string{"libredash.env", "initial-credentials.json"} {
			if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
				t.Fatalf("partial initialization retained %s: %v", name, err)
			}
		}
		if err := os.Remove(filepath.Join(root, "fail-validation")); err != nil {
			t.Fatal(err)
		}
		runController(t, root, fakeDocker, "", "init", "--admin-email", "admin@example.com", "--domain", "dash.example.com", "--image", image)
	})

	t.Run("mutable proxy image", func(t *testing.T) {
		root, fakeDocker := setup(t)
		examplePath := filepath.Join(root, "deployment.env.example")
		contents, err := os.ReadFile(examplePath)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(contents), "\n")
		for i := range lines {
			if strings.HasPrefix(lines[i], "CADDY_IMAGE=") {
				lines[i] = "CADDY_IMAGE=caddy:latest"
			}
		}
		if err := os.WriteFile(examplePath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := runControllerResult(root, fakeDocker, "", "init", "--admin-email", "admin@example.com", "--domain", "dash.example.com", "--image", image); err == nil || !strings.Contains(output, "image must be pinned by digest") {
			t.Fatalf("mutable proxy result = %v, %s", err, output)
		}
	})
}

func copyDeploymentFile(t *testing.T, targetDir, name string, mode os.FileMode) {
	t.Helper()
	contents, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, name), contents, mode); err != nil {
		t.Fatal(err)
	}
}

func buildController(t *testing.T, targetDir string) string {
	t.Helper()
	target := filepath.Join(targetDir, "libredashctl")
	command := exec.Command("go", "build", "-o", target, "./cmd/libredashctl")
	command.Dir = filepath.Join("..", "..")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build libredashctl: %v\n%s", err, output)
	}
	return target
}

func runController(t *testing.T, root, docker, failImage string, args ...string) string {
	t.Helper()
	output, err := runControllerResult(root, docker, failImage, args...)
	if err != nil {
		t.Fatalf("libredashctl %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}

func runControllerResult(root, docker, failImage string, args ...string) (string, error) {
	command := exec.Command(filepath.Join(root, "libredashctl"), args...)
	command.Dir = root
	command.Env = append(os.Environ(), "LIBREDASHCTL_ROOT="+root, "LIBREDASHCTL_DOCKER_BIN="+docker, "FAKE_DOCKER_FAIL_IMAGE="+failImage)
	output, err := command.CombinedOutput()
	return string(output), err
}

func requireDeploymentImage(t *testing.T, root, image string) {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, "deployment.env"))
	if err != nil || !strings.Contains(string(contents), "LIBREDASH_IMAGE="+image+"\n") {
		t.Fatalf("deployment image is not %s: %v\n%s", image, err, contents)
	}
}

func read(t *testing.T, name string) string {
	t.Helper()
	value, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}
