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
	if set.Schema != 1 || set.Name != "management-v1" || set.SourceBaseline != "2.9.0" {
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
