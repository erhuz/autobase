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
	ImageRegistry  string                      `json:"image_registry"`
	Components     map[string]versionComponent `json:"components"`
}

type versionComponent struct {
	Path          string `json:"path"`
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
		set.ImageRegistry != "ghcr.io/erhuz" {
		t.Fatalf("unexpected version set metadata: %+v", set)
	}

	for _, name := range []string{"ui", "api", "console_db", "automation"} {
		component, ok := set.Components[name]
		if !ok {
			t.Errorf("%s component missing", name)
			continue
		}
		if component.VersionSource != "release_tag" {
			t.Errorf("%s version source = %q", name, component.VersionSource)
		}
		if info, statErr := os.Stat(filepath.Join(root, component.Path)); statErr != nil || !info.IsDir() {
			t.Errorf("%s path %q missing", name, component.Path)
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
}

func TestImageRegistryContract(t *testing.T) {
	root := filepath.Clean("../../..")
	contracts := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{".config/make/docker.mak", []string{"DOCKER_REGISTRY ?= ghcr.io/erhuz", "docker login ghcr.io"}, []string{"Dockerhub"}},
		{".github/workflows/docker.yml", []string{"packages: write", "DOCKER_REGISTRY_USER: ${{ github.actor }}", "DOCKER_REGISTRY_PASSWORD: ${{ secrets.GITHUB_TOKEN }}"}, []string{"secrets.DOCKER_USERNAME", "secrets.DOCKER_PASSWORD"}},
		{".github/workflows/release.yml", []string{"packages: write", `IMAGE_TAG_PATTERN="[[:alnum:]_.-]\+"`, "ghcr.io/erhuz/automation:${IMAGE_TAG_PATTERN}", "DOCKER_REGISTRY_PASSWORD: ${{ secrets.GITHUB_TOKEN }}"}, []string{"secrets.DOCKER_USERNAME", "secrets.DOCKER_PASSWORD", `autobase\/console_db:`}},
		{"console/docker-compose.yml", []string{"ghcr.io/erhuz/console_api:", "ghcr.io/erhuz/console_ui:", "ghcr.io/erhuz/console_db:"}, []string{"autobase/console_api:", "autobase/console_ui:", "autobase/console_db:"}},
		{"console/docker-compose.caddy.yml", []string{"ghcr.io/erhuz/console_api:", "ghcr.io/erhuz/console_ui:", "ghcr.io/erhuz/console_db:"}, []string{"autobase/console_api:", "autobase/console_ui:", "autobase/console_db:"}},
		{"console/docker-compose.enterprise.yml", []string{"ghcr.io/erhuz/console_db:"}, []string{"autobase/console_db:"}},
		{"console/docker-compose.enterprise.ssl.yml", []string{"ghcr.io/erhuz/console_db:"}, []string{"autobase/console_db:"}},
		{"console/README.md", []string{"ghcr.io/erhuz/automation:latest", "ghcr.io/erhuz/console:latest"}, []string{"autobase/automation:", "autobase/console:"}},
		{"console/service/README.md", []string{"ghcr.io/erhuz/automation:"}, []string{"autobase/automation:"}},
		{"console/service/internal/configuration/config.go", []string{"ghcr.io/erhuz/automation:"}, []string{"autobase/automation:"}},
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
