package xdocker

import (
	"os"
	"strings"
	"testing"
)

func TestDockerClientLogExcludesBodies(t *testing.T) {
	source, err := os.ReadFile("round_tripper_log.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`Str("body"`, "io.ReadAll", "drainBody"} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("Docker logger still reads or emits bodies: %s", forbidden)
		}
	}
}
