package watcher

import (
	"strings"
	"testing"

	"postgresql-cluster-console/internal/storage"
)

func TestQueryAnalyticsCollectionPrivacyContract(t *testing.T) {
	for _, required := range []string{"bucket_done = true", "application_name is distinct from $1", "pgsm_query_id", "left(query, 2048)"} {
		if !strings.Contains(queryAnalyticsCollectionSQL, required) {
			t.Errorf("collector SQL missing %q", required)
		}
	}
	for _, forbidden := range []string{"client_ip", "comments", "query_plan", "message", "sqlcode"} {
		if strings.Contains(queryAnalyticsCollectionSQL, forbidden) {
			t.Errorf("collector SQL selects forbidden field %q", forbidden)
		}
	}
	if !queryAnalyticsPrivacySafe("on", "on", "on", "off", "off", "off", "off") {
		t.Fatal("safe PGSM configuration rejected")
	}
	if queryAnalyticsPrivacySafe("on", "on", "on", "off", "off", "on", "off") {
		t.Fatal("comment capture drift accepted")
	}
}

func TestQueryAnalyticsMergeAndTopUnion(t *testing.T) {
	bucket := storage.QueryAnalyticsBucket{}
	indexes := map[string]int{}
	first := storage.QueryAnalyticsSample{
		FingerprintID: "same", DatabaseName: "db", RoleName: "role", ApplicationName: "app",
		Calls: 1, TotalExecTimeMs: 3, MinExecTimeMs: 3, MaxExecTimeMs: 3, LatencyHistogram: []string{"1", "2"},
	}
	second := first
	second.Calls, second.TotalExecTimeMs, second.MinExecTimeMs, second.MaxExecTimeMs = 2, 8, 2, 5
	second.LatencyHistogram = []string{"3", "4"}
	mergeQueryAnalyticsSample(&bucket, indexes, first)
	mergeQueryAnalyticsSample(&bucket, indexes, second)
	finalizeQueryAnalyticsBucket(&bucket)
	if len(bucket.Samples) != 1 || bucket.Calls != 3 || bucket.TotalExecTimeMs != 11 {
		t.Fatalf("merged bucket = %#v", bucket)
	}
	if got := strings.Join(bucket.Samples[0].LatencyHistogram, ","); got != "4,6" {
		t.Fatalf("merged histogram = %s", got)
	}

	bucket.Samples = nil
	for i := 0; i < 201; i++ {
		bucket.Samples = append(bucket.Samples, storage.QueryAnalyticsSample{
			FingerprintID: string(rune(i + 1)), TotalExecTimeMs: float64(i), MaxExecTimeMs: float64(200 - i), Calls: 1,
		})
	}
	finalizeQueryAnalyticsBucket(&bucket)
	if len(bucket.Samples) != 200 {
		t.Fatalf("top union retained %d samples, want 200", len(bucket.Samples))
	}
}
