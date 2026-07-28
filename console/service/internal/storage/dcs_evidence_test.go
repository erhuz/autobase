package storage

import (
	"testing"
	"time"
)

func TestV60DCSLastSeenMetadataIsReadableAndInternal(t *testing.T) {
	lastSeen := time.Now().UTC().Truncate(time.Second)
	tags := WithDCSLastSeen(map[string]any{"nofailover": true}, lastSeen)
	got, ok := ServerDCSLastSeen(tags)
	if !ok || !got.Equal(lastSeen) {
		t.Fatalf("last seen = %v, %t", got, ok)
	}
	visible := VisibleServerTags(tags).(map[string]any)
	if visible["nofailover"] != true {
		t.Fatalf("visible tags = %v", visible)
	}
	if _, leaked := visible[dcsLastSeenTag]; leaked {
		t.Fatalf("internal DCS evidence leaked: %v", visible)
	}
}
