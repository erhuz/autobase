package storage

import (
	"encoding/json"
	"time"
)

const dcsLastSeenTag = "_autobase_dcs_last_seen"

func WithDCSLastSeen(tags any, lastSeen time.Time) any {
	values := serverTagMap(tags)
	delete(values, dcsLastSeenTag)
	if !lastSeen.IsZero() {
		values[dcsLastSeenTag] = lastSeen.UTC().Format(time.RFC3339Nano)
	}
	return values
}

func ServerDCSLastSeen(tags any) (time.Time, bool) {
	value, ok := serverTagMap(tags)[dcsLastSeenTag].(string)
	if !ok {
		return time.Time{}, false
	}
	lastSeen, err := time.Parse(time.RFC3339Nano, value)
	return lastSeen, err == nil
}

func VisibleServerTags(tags any) any {
	if tags == nil {
		return nil
	}
	values := serverTagMap(tags)
	delete(values, dcsLastSeenTag)
	return values
}

func serverTagMap(tags any) map[string]any {
	raw, _ := json.Marshal(tags)
	values := map[string]any{}
	_ = json.Unmarshal(raw, &values)
	if values == nil {
		values = map[string]any{}
	}
	return values
}
