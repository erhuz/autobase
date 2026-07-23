package cluster

import (
	"slices"
	"testing"
)

func TestBackupEnabled(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want bool
	}{
		{`{"pgbackrest_install":true}`, true},
		{`{"pgbackrest_install":"true"}`, true},
		{`{"pgbackrest_install":false}`, false},
		{`{}`, false},
	} {
		if got := backupEnabled([]byte(test.raw)); got != test.want {
			t.Fatalf("backupEnabled(%s) = %t", test.raw, got)
		}
	}
}

func TestBackupObserverDoesNotShareOperationLogFile(t *testing.T) {
	envs := backupObserverEnvs([]string{"TOKEN=value", "ANSIBLE_JSON_LOG_FILE=/tmp/cluster.json"})
	if !slices.Equal(envs, []string{"TOKEN=value"}) {
		t.Fatalf("envs = %v", envs)
	}
}
