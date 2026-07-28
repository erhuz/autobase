package storage

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
)

const (
	BackupEvidenceMarker  = "AUTOBASE_BACKUP_EVIDENCE="
	RestoreEvidenceMarker = "AUTOBASE_RESTORE_EVIDENCE="
)

type backupEvidencePayload struct {
	ObservedAt          time.Time      `json:"observed_at"`
	RepositoryReachable bool           `json:"repository_reachable"`
	LatestFull          *time.Time     `json:"latest_full"`
	LatestDifferential  *time.Time     `json:"latest_differential"`
	Retention           map[string]any `json:"retention"`
	WalContinuous       *bool          `json:"wal_continuous"`
	Locks               []string       `json:"locks"`
	SchedulerOwners     []string       `json:"scheduler_owners"`
	FreshnessSeconds    int64          `json:"freshness_seconds"`
}

type RestoreEvidence struct {
	VerifiedAt      time.Time `json:"verified_at"`
	SourceCluster   string    `json:"source_cluster"`
	RecoveryCluster string    `json:"recovery_cluster"`
	Operation       string    `json:"operation"`
}

func DecodeBackupEvidence(message string, clusterID int64) (*BackupEvidence, error) {
	var payload backupEvidencePayload
	if err := decodeEvidence(message, BackupEvidenceMarker, &payload); err != nil {
		return nil, err
	}
	if payload.ObservedAt.IsZero() || payload.FreshnessSeconds <= 0 {
		return nil, errors.New("backup evidence is incomplete")
	}
	retention, err := json.Marshal(payload.Retention)
	if err != nil {
		return nil, err
	}
	locks, err := json.Marshal(payload.Locks)
	if err != nil {
		return nil, err
	}
	owners, err := json.Marshal(payload.SchedulerOwners)
	if err != nil {
		return nil, err
	}
	return &BackupEvidence{
		ClusterID: clusterID, ObservedAt: payload.ObservedAt,
		RepositoryReachable: payload.RepositoryReachable,
		LatestFull:          payload.LatestFull, LatestDifferential: payload.LatestDifferential,
		Retention: retention, WalContinuous: payload.WalContinuous,
		Locks: locks, SchedulerOwners: owners, FreshnessSeconds: payload.FreshnessSeconds,
	}, nil
}

func DecodeRestoreEvidence(message string) (*RestoreEvidence, error) {
	var evidence RestoreEvidence
	if err := decodeEvidence(message, RestoreEvidenceMarker, &evidence); err != nil {
		return nil, err
	}
	if evidence.VerifiedAt.IsZero() || evidence.SourceCluster == "" || evidence.RecoveryCluster == "" ||
		evidence.SourceCluster == evidence.RecoveryCluster ||
		(evidence.Operation != OperationTypeRestore && evidence.Operation != OperationTypePITR) {
		return nil, errors.New("restore evidence is incomplete")
	}
	return &evidence, nil
}

func decodeEvidence(message, marker string, destination any) error {
	start := strings.Index(message, marker)
	if start < 0 {
		return errors.New("evidence marker not found")
	}
	encoded := message[start+len(marker):]
	if end := strings.IndexFunc(encoded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '/' && r != '='
	}); end >= 0 {
		encoded = encoded[:end]
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, destination)
}
