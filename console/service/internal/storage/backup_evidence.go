package storage

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
)

const BackupEvidenceMarker = "AUTOBASE_BACKUP_EVIDENCE="

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
	RestoreTestedAt     *time.Time     `json:"restore_tested_at"`
}

func DecodeBackupEvidence(message string, clusterID int64) (*BackupEvidence, error) {
	start := strings.Index(message, BackupEvidenceMarker)
	if start < 0 {
		return nil, errors.New("backup evidence marker not found")
	}
	encoded := message[start+len(BackupEvidenceMarker):]
	if end := strings.IndexFunc(encoded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '/' && r != '='
	}); end >= 0 {
		encoded = encoded[:end]
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var payload backupEvidencePayload
	if err = json.Unmarshal(raw, &payload); err != nil {
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
		RestoreTestedAt: payload.RestoreTestedAt,
	}, nil
}
