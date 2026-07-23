package storage

import "testing"

func TestV50OperationTerminalStatus(t *testing.T) {
	for status, terminal := range map[string]bool{
		OperationStatusQueued: false, OperationStatusRunning: false, OperationStatusSucceeded: true,
		OperationStatusFailed: true, OperationStatusCancelled: true,
	} {
		if IsTerminalOperationStatus(status) != terminal {
			t.Fatalf("%s terminal = %t", status, !terminal)
		}
	}
}
