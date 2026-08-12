package releasecontract_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseCredentialsAreIsolatedFromDependencyHooks(t *testing.T) {
	t.Parallel()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate release contract test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	workflow := readFile(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	config := readFile(t, filepath.Join(root, ".goreleaser.yaml"))

	for _, hook := range []string{"npm ci --prefix web", "npm run build --prefix web", "git diff --exit-code -- internal/server/webdist"} {
		if !strings.Contains(config, hook) {
			t.Fatalf("manual GoReleaser contract is missing hook %q", hook)
		}
	}
	if !strings.Contains(workflow, "persist-credentials: false") {
		t.Fatal("release checkout must not persist Git credentials")
	}
	snapshot := strings.Index(workflow, "args: release --snapshot --clean")
	publish := strings.Index(workflow, "args: release --clean --skip=before")
	token := strings.Index(workflow, "GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}")
	if snapshot < 0 || publish < 0 || token < 0 || snapshot >= publish || publish >= token {
		t.Fatalf("release workflow must validate hooks without credentials before publishing with --skip=before: snapshot=%d publish=%d token=%d", snapshot, publish, token)
	}
	if strings.Count(workflow, "GITHUB_TOKEN:") != 1 {
		t.Fatalf("release workflow exposes GITHUB_TOKEN %d times, want once", strings.Count(workflow, "GITHUB_TOKEN:"))
	}
}

func TestReleaseWorkflowsProvisionPinnedSyftBeforeGoReleaser(t *testing.T) {
	t.Parallel()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate release contract test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	const syftAction = "anchore/sbom-action/download-syft@e22c389904149dbc22b58101806040fa8d37a610"
	const syftVersion = "syft-version: v1.49.0"
	const goReleaserAction = "uses: goreleaser/goreleaser-action@"

	for _, workflowPath := range []string{
		filepath.Join(root, ".github", "workflows", "ci.yml"),
		filepath.Join(root, ".github", "workflows", "release.yml"),
	} {
		workflow := readFile(t, workflowPath)
		syft := strings.Index(workflow, syftAction)
		version := strings.Index(workflow, syftVersion)
		goReleaser := strings.Index(workflow, goReleaserAction)
		if syft < 0 || version < syft || goReleaser < 0 || version >= goReleaser {
			t.Errorf("%s must provision pinned Syft before its first GoReleaser call: syft=%d version=%d goreleaser=%d", workflowPath, syft, version, goReleaser)
		}
		if strings.Count(workflow, syftAction) != 1 || strings.Count(workflow, syftVersion) != 1 {
			t.Errorf("%s must provision exactly one pinned Syft version", workflowPath)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
