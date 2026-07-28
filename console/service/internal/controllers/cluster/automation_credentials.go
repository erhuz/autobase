package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
)

type automationCredentialPurpose string

const (
	credentialPostgresSuperuser   automationCredentialPurpose = "postgres_superuser"
	credentialPostgresReplication automationCredentialPurpose = "postgres_replication"
	credentialPatroniRestapi      automationCredentialPurpose = "patroni_restapi"
)

type passwordCredential struct {
	Username string `json:"USERNAME"`
	Password string `json:"PASSWORD"`
}

type automationCredentialFingerprint struct {
	Purpose   automationCredentialPurpose `json:"purpose"`
	SecretID  int64                       `json:"secret_id"`
	UpdatedAt string                      `json:"updated_at"`
}

func automationEncryptionKey(cfg *configuration.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.EncryptionKey
}

func requiredAutomationCredentials(operationType string) ([]automationCredentialPurpose, error) {
	switch operationType {
	case storage.OperationTypeBackupFull, storage.OperationTypeBackupDiff, storage.OperationTypeBackupSchedulerReconcile:
		return nil, nil
	case storage.OperationTypeQueryAnalyticsEnable, storage.OperationTypeQueryAnalyticsDisable:
		return []automationCredentialPurpose{credentialPostgresSuperuser, credentialPatroniRestapi}, nil
	case storage.OperationTypeNodeAdd, storage.OperationTypeConfigUpdate, storage.OperationTypePostgreSQLUpgrade,
		storage.OperationTypeRestore, storage.OperationTypePITR:
		return []automationCredentialPurpose{
			credentialPostgresSuperuser,
			credentialPostgresReplication,
			credentialPatroniRestapi,
		}, nil
	case storage.OperationTypeSwitchover, storage.OperationTypeReload, storage.OperationTypeRollingRestart,
		storage.OperationTypeReplicaReinit, storage.OperationTypeNodeRemove, storage.OperationTypeRollingUpdate,
		storage.OperationTypeEmergencyFailover, storage.OperationTypeDatabaseAdmin,
		storage.OperationTypeExtensionAdmin, storage.OperationTypePgBouncerAdmin:
		return []automationCredentialPurpose{credentialPostgresSuperuser}, nil
	default:
		return nil, errors.New("operation credential requirements are undeclared")
	}
}

func automationCredentialID(clusterInfo *storage.Cluster, purpose automationCredentialPurpose) *int64 {
	switch purpose {
	case credentialPostgresSuperuser:
		return clusterInfo.PostgresSuperuserSecretID
	case credentialPostgresReplication:
		return clusterInfo.PostgresReplicationSecretID
	case credentialPatroniRestapi:
		return clusterInfo.PatroniRestapiSecretID
	default:
		return nil
	}
}

func automationCredentialLabel(purpose automationCredentialPurpose) string {
	switch purpose {
	case credentialPostgresSuperuser:
		return "PostgreSQL superuser credential attached and decryptable"
	case credentialPostgresReplication:
		return "PostgreSQL replication credential attached and decryptable"
	case credentialPatroniRestapi:
		return "Patroni REST API credential attached and decryptable"
	default:
		return "automation credential requirement declared"
	}
}

func loadAutomationCredential(
	ctx context.Context,
	db storage.IStorage,
	encryptionKey string,
	projectID, secretID int64,
) (passwordCredential, *storage.SecretView, error) {
	secret, err := db.GetSecret(ctx, secretID)
	if err != nil || secret == nil {
		return passwordCredential{}, nil, errors.New("automation credential is unavailable")
	}
	if secret.ProjectID != projectID || secret.Type != string(models.SecretTypePassword) {
		return passwordCredential{}, nil, errors.New("automation credential must be a same-project password secret")
	}
	value, err := db.GetSecretVal(ctx, secretID, encryptionKey)
	if err != nil {
		return passwordCredential{}, nil, errors.New("automation credential cannot be decrypted")
	}
	var credential passwordCredential
	if json.Unmarshal(value, &credential) != nil ||
		strings.TrimSpace(credential.Username) == "" || credential.Password == "" {
		return passwordCredential{}, nil, errors.New("automation credential must contain username and password")
	}
	return credential, secret, nil
}

func (h *guardedOperationsHandler) bindAutomationCredentialPreflight(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	operationType string,
	state *guardedPreflight,
) error {
	required, err := requiredAutomationCredentials(operationType)
	if err != nil {
		return err
	}
	fingerprints := make([]automationCredentialFingerprint, 0, len(required))
	for _, purpose := range required {
		secretID := automationCredentialID(clusterInfo, purpose)
		check := preflightCheck{Name: automationCredentialLabel(purpose)}
		fingerprint := automationCredentialFingerprint{Purpose: purpose}
		if secretID != nil {
			fingerprint.SecretID = *secretID
			_, secret, credentialErr := loadAutomationCredential(
				ctx, h.db, automationEncryptionKey(h.cfg), clusterInfo.ProjectID, *secretID,
			)
			check.OK = credentialErr == nil
			if secret != nil {
				updatedAt := secret.CreatedAt
				if secret.UpdatedAt != nil {
					updatedAt = *secret.UpdatedAt
				}
				fingerprint.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
			}
		}
		state.checks = append(state.checks, check)
		if !check.OK {
			state.blockers = append(state.blockers, check.Name)
		}
		fingerprints = append(fingerprints, fingerprint)
	}
	state.topologyHash, err = automationCredentialHash(state.topologyHash, fingerprints)
	return err
}

func automationCredentialHash(topologyHash string, credentials []automationCredentialFingerprint) (string, error) {
	payload, err := json.Marshal(struct {
		TopologyHash string                            `json:"topology_hash"`
		Credentials  []automationCredentialFingerprint `json:"credentials"`
	}{TopologyHash: topologyHash, Credentials: credentials})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (h *guardedOperationsHandler) injectAutomationCredentials(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	operationType string,
	extraVars []byte,
) ([]byte, error) {
	required, err := requiredAutomationCredentials(operationType)
	if err != nil {
		return nil, err
	}
	var values map[string]any
	if err = json.Unmarshal(extraVars, &values); err != nil {
		return nil, err
	}
	purposes := make([]string, 0, len(required))
	for _, purpose := range required {
		secretID := automationCredentialID(clusterInfo, purpose)
		if secretID == nil {
			return nil, errors.New("required automation credential is not attached")
		}
		credential, _, credentialErr := loadAutomationCredential(
			ctx, h.db, automationEncryptionKey(h.cfg), clusterInfo.ProjectID, *secretID,
		)
		if credentialErr != nil {
			return nil, credentialErr
		}
		switch purpose {
		case credentialPostgresSuperuser:
			values["patroni_superuser_username"] = credential.Username
			values["patroni_superuser_password"] = credential.Password
		case credentialPostgresReplication:
			values["patroni_replication_username"] = credential.Username
			values["patroni_replication_password"] = credential.Password
		case credentialPatroniRestapi:
			values["patroni_restapi_username"] = credential.Username
			values["patroni_restapi_password"] = credential.Password
		}
		purposes = append(purposes, string(purpose))
	}
	values["operation_required_credentials"] = purposes
	if operationType == storage.OperationTypeRestore || operationType == storage.OperationTypePITR {
		values["operation_credentials_validate_only"] = true
	}
	return json.Marshal(values)
}
