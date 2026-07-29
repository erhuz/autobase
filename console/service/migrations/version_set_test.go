package migrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type versionSet struct {
	Schema         int                         `json:"schema"`
	Name           string                      `json:"name"`
	SourceBaseline string                      `json:"source_baseline"`
	ReleaseVersion string                      `json:"release_version"`
	Platform       string                      `json:"platform"`
	ImageRegistry  string                      `json:"image_registry"`
	Components     map[string]versionComponent `json:"components"`
}

type versionComponent struct {
	Path          string `json:"path"`
	Image         string `json:"image"`
	Digest        string `json:"digest"`
	VersionSource string `json:"version_source"`
	MigrationHead int64  `json:"migration_head"`
}

func TestManagementVersionSet(t *testing.T) {
	root := filepath.Clean("../../..")
	data, err := os.ReadFile(filepath.Join(root, "MANAGEMENT_VERSION_SET.json"))
	if err != nil {
		t.Fatal(err)
	}

	var set versionSet
	if err = json.Unmarshal(data, &set); err != nil {
		t.Fatal(err)
	}
	if set.Schema != 1 || set.Name != "management-v1" || set.SourceBaseline != "2.9.0" ||
		set.ReleaseVersion != "2.9.0-management.8" || set.Platform != "linux/amd64" ||
		set.ImageRegistry != "ghcr.io/erhuz" {
		t.Fatalf("unexpected version set metadata: %+v", set)
	}

	for _, name := range []string{"ui", "api", "console_db", "automation"} {
		component, ok := set.Components[name]
		if !ok {
			t.Errorf("%s component missing", name)
			continue
		}
		if info, statErr := os.Stat(filepath.Join(root, component.Path)); statErr != nil || !info.IsDir() {
			t.Errorf("%s path %q missing", name, component.Path)
		}
		if name == "console_db" {
			if component.VersionSource != "official_2.9.0_digest" || component.Image != "autobase/console_db" ||
				!strings.HasPrefix(component.Digest, "sha256:") {
				t.Errorf("console_db version contract = %+v", component)
			}
		} else if component.VersionSource != "release_version" ||
			component.Image != set.ImageRegistry+"/"+map[string]string{"ui": "console_ui", "api": "console_api", "automation": "automation"}[name] {
			t.Errorf("%s version contract = %+v", name, component)
		}
	}

	entries, err := os.ReadDir(filepath.Join(root, set.Components["console_db"].Path, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var migrationHead int64
	for _, entry := range entries {
		prefix, _, found := strings.Cut(entry.Name(), "_")
		if !found {
			continue
		}
		version, parseErr := strconv.ParseInt(prefix, 10, 64)
		if parseErr == nil && version > migrationHead {
			migrationHead = version
		}
	}
	if want := set.Components["console_db"].MigrationHead; migrationHead != want {
		t.Fatalf("migration head = %d, version set = %d", migrationHead, want)
	}

	serviceVersion, err := os.ReadFile(filepath.Join(root, "console/service/VERSION"))
	if err != nil || strings.TrimSpace(string(serviceVersion)) != set.ReleaseVersion {
		t.Fatalf("service version = %q, %v", serviceVersion, err)
	}
	var uiPackage struct {
		Version string `json:"version"`
	}
	uiData, err := os.ReadFile(filepath.Join(root, "console/ui/package.json"))
	if err != nil || json.Unmarshal(uiData, &uiPackage) != nil || uiPackage.Version != set.ReleaseVersion {
		t.Fatalf("UI version = %q, %v", uiPackage.Version, err)
	}
}

func TestImageRegistryContract(t *testing.T) {
	root := filepath.Clean("../../..")
	contracts := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{".config/make/docker.mak", []string{"DOCKER_REGISTRY ?= ghcr.io/erhuz", "DOCKER_PLATFORMS ?= linux/amd64", "docker-push-management", "docker login ghcr.io"}, []string{"Dockerhub"}},
		{".github/workflows/docker.yml", []string{"packages: write", "make docker-push-management", "DOCKER_REGISTRY_PASSWORD: ${{ secrets.GITHUB_TOKEN }}"}, []string{"make docker-push\n", "secrets.DOCKER_PASSWORD"}},
		{".github/workflows/release.yml", []string{"2.9.0-management.*", "git diff --exit-code", "make docker-push-management", "release-manifest.json", "docker logout ghcr.io", "gh release create"}, []string{"sed -i", "git commit", "git push", "ansible-galaxy collection publish", "docker-push-console-db"}},
		{"console/docker-compose.yml", []string{"ghcr.io/erhuz/console_api:2.9.0-management.8", "ghcr.io/erhuz/console_ui:2.9.0-management.8", "autobase/console_db:2.9.0"}, []string{"ghcr.io/erhuz/console_db:"}},
		{"console/docker-compose.caddy.yml", []string{"ghcr.io/erhuz/console_api:2.9.0-management.8", "ghcr.io/erhuz/console_ui:2.9.0-management.8", "autobase/console_db:2.9.0"}, []string{"ghcr.io/erhuz/console_db:"}},
		{"console/docker-compose.enterprise.yml", []string{"autobase/console_db:2.9.0"}, []string{"ghcr.io/erhuz/console_db:"}},
		{"console/docker-compose.enterprise.ssl.yml", []string{"autobase/console_db:2.9.0"}, []string{"ghcr.io/erhuz/console_db:"}},
		{"console/README.md", []string{"### Docker Compose"}, []string{"ghcr.io/erhuz/console:"}},
		{"console/service/README.md", []string{"ghcr.io/erhuz/automation:2.9.0-management.8"}, []string{"ghcr.io/erhuz/automation:latest"}},
		{"console/service/internal/configuration/config.go", []string{"ghcr.io/erhuz/automation:2.9.0-management.8"}, []string{"ghcr.io/erhuz/automation:latest"}},
	}

	for _, contract := range contracts {
		data, err := os.ReadFile(filepath.Join(root, contract.path))
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, required := range contract.required {
			if !strings.Contains(content, required) {
				t.Errorf("%s missing %q", contract.path, required)
			}
		}
		for _, forbidden := range contract.forbidden {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s contains %q", contract.path, forbidden)
			}
		}
	}
}
