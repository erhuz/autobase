package cluster

import (
	"testing"
	"time"

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
