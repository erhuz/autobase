package cluster

import (
	"context"
	"encoding/json"
	"testing"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"

	"github.com/rs/zerolog"
)

func TestAutomationCredentialRequirementsAreDeclared(t *testing.T) {
	tests := map[string][]automationCredentialPurpose{
		storage.OperationTypeBackupFull:            nil,
		storage.OperationTypeBackupDiff:            nil,
		storage.OperationTypeQueryAnalyticsEnable:  {credentialPostgresSuperuser, credentialPatroniRestapi},
		storage.OperationTypeQueryAnalyticsDisable: {credentialPostgresSuperuser, credentialPatroniRestapi},
		storage.OperationTypeNodeAdd:               {credentialPostgresSuperuser, credentialPostgresReplication, credentialPatroniRestapi},
		storage.OperationTypeConfigUpdate:          {credentialPostgresSuperuser, credentialPostgresReplication, credentialPatroniRestapi},
		storage.OperationTypePostgreSQLUpgrade:     {credentialPostgresSuperuser, credentialPostgresReplication, credentialPatroniRestapi},
		storage.OperationTypeRestore:               {credentialPostgresSuperuser, credentialPostgresReplication, credentialPatroniRestapi},
		storage.OperationTypePITR:                  {credentialPostgresSuperuser, credentialPostgresReplication, credentialPatroniRestapi},
		storage.OperationTypeSwitchover:            {credentialPostgresSuperuser},
		storage.OperationTypeReload:                {credentialPostgresSuperuser},
		storage.OperationTypeRollingRestart:        {credentialPostgresSuperuser},
		storage.OperationTypeReplicaReinit:         {credentialPostgresSuperuser},
		storage.OperationTypeNodeRemove:            {credentialPostgresSuperuser},
		storage.OperationTypeRollingUpdate:         {credentialPostgresSuperuser},
		storage.OperationTypeEmergencyFailover:     {credentialPostgresSuperuser},
		storage.OperationTypeDatabaseAdmin:         {credentialPostgresSuperuser},
		storage.OperationTypeExtensionAdmin:        {credentialPostgresSuperuser},
		storage.OperationTypePgBouncerAdmin:        {credentialPostgresSuperuser},
	}
	for operationType, expected := range tests {
		if !supportedOperationType(operationType) {
			t.Fatalf("%s is not a supported guarded operation", operationType)
		}
		actual, err := requiredAutomationCredentials(operationType)
		if err != nil || !sameJSON(mustJSON(actual), mustJSON(expected)) {
			t.Fatalf("%s credentials=%v expected=%v err=%v", operationType, actual, expected, err)
		}
	}
	if _, err := requiredAutomationCredentials("undeclared"); err == nil {
		t.Fatal("undeclared operation did not fail closed")
	}
}

func TestAutomationCredentialsAreInjectedOnlyAtLaunch(t *testing.T) {
	store, _ := switchoverFixture()
	handler := NewGuardedOperationsHandler(
		store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop(),
	)
	payload, err := handler.injectAutomationCredentials(
		context.Background(), store.cluster, storage.OperationTypePostgreSQLUpgrade, []byte(`{"fixed":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]any
	if json.Unmarshal(payload, &inputs) != nil ||
		inputs["patroni_superuser_password"] != "service-secret" ||
		inputs["patroni_replication_password"] != "service-secret" ||
		inputs["patroni_restapi_password"] != "service-secret" ||
		inputs["fixed"] != true {
		t.Fatalf("inputs=%v", inputs)
	}

	restore, err := handler.injectAutomationCredentials(
		context.Background(), store.cluster, storage.OperationTypeRestore, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(restore, &inputs) != nil || inputs["operation_credentials_validate_only"] != true {
		t.Fatalf("restore inputs=%v", inputs)
	}
}

func TestAutomationCredentialMetadataInvalidatesPreflightHash(t *testing.T) {
	before, err := automationCredentialHash("topology", []automationCredentialFingerprint{{
		Purpose: credentialPostgresSuperuser, SecretID: 8, UpdatedAt: "before",
	}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := automationCredentialHash("topology", []automationCredentialFingerprint{{
		Purpose: credentialPostgresSuperuser, SecretID: 8, UpdatedAt: "after",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("credential metadata change did not invalidate the preflight")
	}
}
