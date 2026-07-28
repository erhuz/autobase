package db

import (
	"testing"
	"time"
)

func TestV64LogQueryArgsHandlesTypedNilStringer(t *testing.T) {
	var timestamp *time.Time
	if got := logQueryArgs([]any{timestamp}); got != "(<nil>)" {
		t.Fatalf("logQueryArgs() = %q", got)
	}
}
