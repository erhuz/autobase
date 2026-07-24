package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const databaseAdminPlaybook = "database_admin.yml"

const (
	databasePresent = "database_present"
	databaseAbsent  = "database_absent"
	userPresent     = "user_present"
	userAbsent      = "user_absent"
	rolePresent     = "role_present"
	roleAbsent      = "role_absent"
	grantPresent    = "grant_present"
	grantAbsent     = "grant_absent"
)

type databaseAdminParams struct {
	Action          string   `json:"database_action"`
	Database        string   `json:"database,omitempty"`
	Owner           string   `json:"owner,omitempty"`
	Role            string   `json:"role,omitempty"`
	ConnectionLimit *int32   `json:"connection_limit,omitempty"`
	ObjectType      string   `json:"object_type,omitempty"`
	Objects         []string `json:"objects,omitempty"`
	Schema          string   `json:"schema,omitempty"`
	Privileges      []string `json:"privileges,omitempty"`
}

type databaseAdminDesired struct {
	Change  databaseAdminParams `json:"change"`
	Primary string              `json:"primary"`
}

var databaseAdminIdentifier = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

var databaseAdminPrivileges = map[string]map[string]bool{
	"database": {"CONNECT": true, "CREATE": true, "TEMPORARY": true},
	"schema":   {"CREATE": true, "USAGE": true},
	"table": {
		"DELETE": true, "INSERT": true, "REFERENCES": true, "SELECT": true,
		"TRIGGER": true, "TRUNCATE": true, "UPDATE": true,
	},
	"sequence": {"SELECT": true, "UPDATE": true, "USAGE": true},
}

func (h *guardedOperationsHandler) databaseAdminPreflightState(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	target string,
	params []byte,
) (*guardedPreflight, error) {
	refreshStarted := time.Now().UTC().Add(-2 * time.Second)
	h.clusterWatcher.HandleCluster(ctx, clusterInfo)

	clusterInfo, err := h.db.GetCluster(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}
	servers, err := h.db.GetClusterServers(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}
	active, err := h.db.HasActiveOperation(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}

	var request databaseAdminParams
	paramsOK := len(params) > 0 && json.Unmarshal(params, &request) == nil && validDatabaseAdminParams(request)
	nodes, leaderCount, healthyCount := operationTopology(servers)
	leader := topologyNode{}
	healthyLeader, allNamed := false, true
	for _, node := range nodes {
		allNamed = allNamed && node.Name != ""
		if leaderRole(node.Role) {
			leader = node
			healthyLeader = healthyStatus(node.Status)
		}
	}
	inventory, inventoryOK := lifecycleInventory(clusterInfo.Inventory)
	primary, primaryOK := lifecycleInventoryHost{}, false
	if leader.Name != "" {
		primary, primaryOK = lifecycleInventoryTarget(inventory, leader.Name)
	}

	checks := []preflightCheck{
		{Name: "target omitted for database administration", OK: target == ""},
		{Name: "supported fixed database change", OK: paramsOK},
		{Name: "PostgreSQL 14-18", OK: clusterInfo.PostgreVersion >= 14 && clusterInfo.PostgreVersion <= 18},
		{Name: "healthy leader", OK: leaderCount == 1 && healthyLeader},
		{Name: "all current members healthy", OK: len(nodes) > 0 && healthyCount == len(nodes)},
		{Name: "topology names resolved", OK: allNamed},
		{Name: "stored inventory valid", OK: inventoryOK},
		{Name: "primary inventory resolved", OK: primaryOK},
		{Name: "topology refreshed now", OK: topologyFresh(servers, refreshStarted)},
		{Name: "no active cluster mutation", OK: !active},
	}
	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			blockers = append(blockers, check.Name)
		}
	}

	desired := databaseAdminDesired{Change: request}
	affected := []string{}
	if primaryOK {
		desired.Primary = primary.Name
		affected = append(affected, primary.Name)
	}
	plan, confirmation := databaseAdminPlan(request)
	hash, err := topologyHash(nodes)
	if err != nil {
		return nil, err
	}
	return &guardedPreflight{
		observed: map[string]any{
			"topology": nodes, "primary": leader.Name, "primary_inventory": desired.Primary,
		},
		desired: desired, checks: checks, blockers: blockers, plan: plan, affectedNodes: affected,
		confirmation: confirmation, topologyHash: hash,
	}, nil
}

func (h *guardedOperationsHandler) databaseAdminOperationInputs(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	desired []byte,
) ([]string, []byte, error) {
	var state databaseAdminDesired
	if json.Unmarshal(desired, &state) != nil || state.Primary == "" || !validDatabaseAdminParams(state.Change) {
		return nil, nil, errors.New("database administration desired state is invalid")
	}

	envs, extraVars, err := h.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		return nil, nil, err
	}
	change := state.Change
	extraVars["database_admin_action"] = change.Action
	extraVars["database_admin_database"] = change.Database
	extraVars["database_admin_owner"] = change.Owner
	extraVars["database_admin_role"] = change.Role
	extraVars["database_admin_object_type"] = change.ObjectType
	extraVars["database_admin_objects"] = change.Objects
	extraVars["database_admin_schema"] = change.Schema
	extraVars["database_admin_privileges"] = change.Privileges
	extraVars["database_admin_primary"] = state.Primary
	extraVars["database_admin_connection_limit"] = int32(-2)
	if change.ConnectionLimit != nil {
		extraVars["database_admin_connection_limit"] = *change.ConnectionLimit
	}
	extraVars["patroni_cluster_name"] = clusterInfo.Name
	extraVars["cloud_provider"] = ""

	tag := ""
	switch change.Action {
	case databasePresent, databaseAbsent:
		item := map[string]any{"db": change.Database, "state": actionState(change.Action)}
		if change.Action == databasePresent {
			item["owner"] = change.Owner
			if change.ConnectionLimit != nil {
				item["conn_limit"] = *change.ConnectionLimit
			}
		}
		extraVars["postgresql_databases"] = []map[string]any{item}
		tag = "postgresql_databases"
	case userPresent, userAbsent, rolePresent, roleAbsent:
		item := map[string]any{
			"name": change.Role, "state": actionState(change.Action),
			"flags": map[bool]string{true: "LOGIN", false: "NOLOGIN"}[strings.HasPrefix(change.Action, "user_")],
		}
		if change.ConnectionLimit != nil {
			item["conn_limit"] = *change.ConnectionLimit
		}
		extraVars["postgresql_users"] = []map[string]any{item}
		extraVars["enable_pg_stat_monitor"] = false
		tag = "postgresql_users"
	case grantPresent, grantAbsent:
		objects := change.Objects
		if change.ObjectType == "database" {
			objects = []string{change.Database}
		}
		item := map[string]any{
			"role": change.Role, "db": change.Database, "type": change.ObjectType,
			"objs": strings.Join(objects, ","), "privs": strings.Join(change.Privileges, ","),
			"state": actionState(change.Action),
		}
		if change.Schema != "" {
			item["schema"] = change.Schema
		}
		extraVars["postgresql_privs"] = []map[string]any{item}
		tag = "postgresql_privs"
	}
	envs = append(envs, "ANSIBLE_RUN_TAGS="+tag+",database_admin")
	payload, err := json.Marshal(extraVars)
	return envs, payload, err
}

func databaseAdminOperationParams(desired []byte) ([]byte, error) {
	var state databaseAdminDesired
	if json.Unmarshal(desired, &state) != nil || state.Primary == "" || !validDatabaseAdminParams(state.Change) {
		return nil, errors.New("database administration parameters are invalid")
	}
	return mustJSON(state.Change), nil
}

func validDatabaseAdminParams(change databaseAdminParams) bool {
	connectionLimitOK := change.ConnectionLimit == nil || *change.ConnectionLimit >= -1
	switch change.Action {
	case databasePresent:
		return databaseAdminIdentifier.MatchString(change.Database) &&
			databaseAdminIdentifier.MatchString(change.Owner) && change.Role == "" &&
			change.ObjectType == "" && len(change.Objects) == 0 && change.Schema == "" &&
			len(change.Privileges) == 0 && connectionLimitOK
	case databaseAbsent:
		return databaseAdminIdentifier.MatchString(change.Database) && change.Owner == "" && change.Role == "" &&
			change.ConnectionLimit == nil && change.ObjectType == "" && len(change.Objects) == 0 &&
			change.Schema == "" && len(change.Privileges) == 0
	case userPresent, rolePresent:
		return change.Database == "" && change.Owner == "" && databaseAdminIdentifier.MatchString(change.Role) &&
			change.ObjectType == "" && len(change.Objects) == 0 && change.Schema == "" &&
			len(change.Privileges) == 0 && connectionLimitOK
	case userAbsent, roleAbsent:
		return change.Database == "" && change.Owner == "" && databaseAdminIdentifier.MatchString(change.Role) &&
			change.ConnectionLimit == nil && change.ObjectType == "" && len(change.Objects) == 0 &&
			change.Schema == "" && len(change.Privileges) == 0
	case grantPresent, grantAbsent:
		return validDatabaseAdminGrant(change)
	default:
		return false
	}
}

func validDatabaseAdminGrant(change databaseAdminParams) bool {
	allowed, ok := databaseAdminPrivileges[change.ObjectType]
	if !ok || !databaseAdminIdentifier.MatchString(change.Database) ||
		!databaseAdminIdentifier.MatchString(change.Role) || change.Owner != "" || change.ConnectionLimit != nil ||
		len(change.Privileges) == 0 || len(change.Privileges) > len(allowed) {
		return false
	}
	if change.ObjectType == "database" {
		if len(change.Objects) != 0 || change.Schema != "" {
			return false
		}
	} else if change.ObjectType == "schema" {
		if change.Schema != "" || !validDatabaseAdminNames(change.Objects) {
			return false
		}
	} else if !databaseAdminIdentifier.MatchString(change.Schema) || !validDatabaseAdminNames(change.Objects) {
		return false
	}
	seen := make(map[string]bool, len(change.Privileges))
	for _, privilege := range change.Privileges {
		if !allowed[privilege] || seen[privilege] {
			return false
		}
		seen[privilege] = true
	}
	return true
}

func validDatabaseAdminNames(names []string) bool {
	if len(names) == 0 || len(names) > 64 {
		return false
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if !databaseAdminIdentifier.MatchString(name) || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

func databaseAdminPlan(change databaseAdminParams) ([]string, string) {
	description, confirmation := "reject unsupported database administration request", "DATABASE ADMIN"
	switch change.Action {
	case databasePresent:
		description, confirmation = "create or reconcile database "+change.Database+" owned by "+change.Owner,
			"DATABASE "+change.Database+" OWNER "+change.Owner
	case databaseAbsent:
		description, confirmation = "drop database "+change.Database,
			"DROP DATABASE "+change.Database+" DELETE DATA"
	case userPresent:
		description, confirmation = "create or reconcile login user "+change.Role, "USER "+change.Role+" LOGIN"
	case userAbsent:
		description, confirmation = "drop login user "+change.Role, "DROP USER "+change.Role
	case rolePresent:
		description, confirmation = "create or reconcile no-login role "+change.Role, "ROLE "+change.Role+" NOLOGIN"
	case roleAbsent:
		description, confirmation = "drop no-login role "+change.Role, "DROP ROLE "+change.Role
	case grantPresent, grantAbsent:
		verb := "grant"
		if change.Action == grantAbsent {
			verb = "revoke"
		}
		description = verb + " " + strings.Join(change.Privileges, ",") + " on " +
			change.ObjectType + " in " + change.Database + " for " + change.Role
		confirmation = strings.ToUpper(verb) + " " + change.Role + " " + change.Database
	}
	return []string{
		"recheck healthy primary and cluster mutation lock",
		description,
		"verify live PostgreSQL database, role, or privilege state",
	}, confirmation
}

func actionState(action string) string {
	if strings.HasSuffix(action, "_absent") {
		return "absent"
	}
	return "present"
}
