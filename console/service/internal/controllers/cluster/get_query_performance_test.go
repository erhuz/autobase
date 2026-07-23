package cluster

import (
	"testing"
	"time"

	"postgresql-cluster-console/internal/storage"

	"github.com/go-openapi/strfmt"
)

func TestQueryPerformanceFilterBounds(t *testing.T) {
	filter, err := queryPerformanceFilter(nil, nil, nil, nil, nil, nil)
	if err != nil || filter.To.Sub(filter.From) != time.Hour {
		t.Fatalf("default filter = %+v, %v", filter, err)
	}

	now := time.Now().UTC()
	tooOld, end := strfmt.DateTime(now.Add(-30*24*time.Hour)), strfmt.DateTime(now.Add(time.Hour))
	serverID := int64(0)
	if _, err = queryPerformanceFilter(&tooOld, &end, &serverID, nil, nil, nil); err == nil {
		t.Fatal("non-positive server accepted")
	}

	from, to := strfmt.DateTime(now), strfmt.DateTime(now.Add(-time.Minute))
	if _, err = queryPerformanceFilter(&from, &to, nil, nil, nil, nil); err == nil {
		t.Fatal("reversed range accepted")
	}
}

func TestQueryPerformanceOverviewReportsCapabilityDrift(t *testing.T) {
	version, errorCode := "2.2.0", "privacy_drift"
	model := queryPerformanceOverviewModel(&storage.QueryAnalyticsOverview{
		Status: storage.QueryAnalyticsStatus{
			State: "rollout_required", Managed: false, Desired: false, PostgresVersion: 16,
		},
		Coverage: []storage.QueryAnalyticsCoverage{{
			ServerID: 1, ServerName: "pg-1", ServerStatus: "running", CollectionStatus: "unsupported",
			ExtensionVersion: &version, LastErrorCode: &errorCode,
		}},
	})

	if model.Status.State != "rollout_required" || model.Status.Managed || model.Status.Desired {
		t.Fatalf("status = %+v", model.Status)
	}
	if len(model.Coverage) != 1 || *model.Coverage[0].ExtensionVersion != version || *model.Coverage[0].LastErrorCode != errorCode {
		t.Fatalf("coverage = %+v", model.Coverage)
	}
}
