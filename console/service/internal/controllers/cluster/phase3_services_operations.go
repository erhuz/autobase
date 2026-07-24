package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const phase3ServicesPlaybook = "phase3_services.yml"

const (
	extensionPresent  = "extension_present"
	extensionAbsent   = "extension_absent"
	poolPresent       = "pool_present"
	poolAbsent        = "pool_absent"
	poolLimitsUpdate  = "limits_update"
	defaultMaxClients = 100000
	defaultMaxDBConns = 10000
	defaultPoolSize   = 100
	defaultQueryWait  = 120
)

type extensionSpec struct {
	Name   string
	Tag    string
	Enable string
}

var supportedExtensions = map[string]extensionSpec{
	"pg_repack": {Name: "pg_repack", Tag: "pg_repack", Enable: "enable_pg_repack"},
	"pgvector":  {Name: "vector", Tag: "pgvector", Enable: "enable_pgvector"},
	"pgrouting": {Name: "pgrouting", Tag: "pgrouting", Enable: "enable_pgrouting"},
	"postgis":   {Name: "postgis", Tag: "postgis", Enable: "enable_postgis"},
}

var poolSizePattern = regexp.MustCompile(`(?:^|\s)pool_size\s*=?\s*(\S+)`)

type phase3ServicesParams struct {
	ExtensionAction        string `json:"extension_action,omitempty"`
	Extension              string `json:"extension,omitempty"`
	Database               string `json:"database,omitempty"`
	Schema                 string `json:"schema,omitempty"`
	PgBouncerAction        string `json:"pgbouncer_action,omitempty"`
	PoolName               string `json:"pool_name,omitempty"`
	PoolSize               *int32 `json:"pool_size,omitempty"`
	PoolMode               string `json:"pool_mode,omitempty"`
	MaxClientConnections   *int32 `json:"max_client_connections,omitempty"`
	MaxDatabaseConnections *int32 `json:"max_database_connections,omitempty"`
	DefaultPoolSize        *int32 `json:"default_pool_size,omitempty"`
	QueryWaitTimeout       *int32 `json:"query_wait_timeout,omitempty"`
}

type pgbouncerPool struct {
	Name           string `json:"name"`
	DBName         string `json:"dbname"`
	PoolParameters any    `json:"pool_parameters"`
}

type pgbouncerConfig struct {
	Pools                  []pgbouncerPool `json:"pools"`
	MaxClientConnections   int32           `json:"max_client_connections"`
	MaxDatabaseConnections int32           `json:"max_database_connections"`
	DefaultPoolSize        int32           `json:"default_pool_size"`
	QueryWaitTimeout       int32           `json:"query_wait_timeout"`
}

type phase3ServicesDesired struct {
	Operation                string               `json:"operation"`
	Change                   phase3ServicesParams `json:"change"`
	Primary                  string               `json:"primary"`
	Nodes                    []string             `json:"nodes"`
	PgBouncer                *pgbouncerConfig     `json:"pgbouncer,omitempty"`
	PgBouncerEnabled         bool                 `json:"pgbouncer_enabled,omitempty"`
	PostgreSQLMaxConnections int32                `json:"postgresql_max_connections,omitempty"`
	PgBouncerProcesses       int32                `json:"pgbouncer_processes,omitempty"`
	Databases                []string             `json:"databases,omitempty"`
}

func (h *guardedOperationsHandler) phase3ServicesPreflightState(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	operationType, target string,
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

	var request phase3ServicesParams
	paramsOK := len(params) > 0 && json.Unmarshal(params, &request) == nil &&
		validPhase3ServicesParams(operationType, request)
	nodes, leaderCount, healthyCount := operationTopology(servers)
	inventory, inventoryOK := lifecycleInventory(clusterInfo.Inventory)
	primary, resolvedNodes, nodesResolved := phase3InventoryNodes(nodes, inventory)

	checks := []preflightCheck{
		{Name: "target omitted for phase 3 service administration", OK: target == ""},
		{Name: "supported fixed phase 3 service change", OK: paramsOK},
		{Name: "PostgreSQL 14-18", OK: clusterInfo.PostgreVersion >= 14 && clusterInfo.PostgreVersion <= 18},
		{Name: "healthy leader", OK: leaderCount == 1 && primary != ""},
		{Name: "all current members healthy", OK: len(nodes) > 0 && healthyCount == len(nodes)},
		{Name: "topology names resolved", OK: namedTopology(nodes)},
		{Name: "stored inventory valid", OK: inventoryOK},
		{Name: "all PostgreSQL inventory nodes resolved", OK: nodesResolved && len(resolvedNodes) == len(nodes)},
		{Name: "topology refreshed now", OK: topologyFresh(servers, refreshStarted)},
		{Name: "no active cluster mutation", OK: !active},
	}

	desired := phase3ServicesDesired{
		Operation: operationType, Change: request, Primary: primary, Nodes: resolvedNodes,
	}
	observed := map[string]any{
		"topology": nodes, "primary_inventory": primary, "inventory_nodes": resolvedNodes,
	}
	if operationType == storage.OperationTypeExtensionAdmin {
		observed["extension"] = configuredExtension(clusterInfo.ExtraVars, request)
	} else {
		current, enabled, maxConnections, processes, databases, configOK :=
			pgbouncerConfigFromExtraVars(clusterInfo.ExtraVars)
		next := current
		if paramsOK && configOK {
			next = mergePgBouncerConfig(current, request)
		}
		capacity, capacityOK := pgbouncerCapacity(next, processes, databases)
		checks = append(checks,
			preflightCheck{Name: "PgBouncer configured", OK: enabled},
			preflightCheck{Name: "stored PgBouncer configuration valid", OK: configOK},
			preflightCheck{Name: "desired pool capacity within PostgreSQL max_connections",
				OK: configOK && capacityOK && capacity <= maxConnections},
		)
		observed["pgbouncer"] = current
		observed["postgresql_max_connections"] = maxConnections
		observed["desired_pool_capacity"] = capacity
		desired.PgBouncer = &next
		desired.PgBouncerEnabled = enabled
		desired.PostgreSQLMaxConnections = maxConnections
		desired.PgBouncerProcesses = processes
		desired.Databases = databases
	}

	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			blockers = append(blockers, check.Name)
		}
	}
	plan, confirmation := phase3ServicesPlan(operationType, request)
	hash, err := topologyHash(nodes)
	if err != nil {
		return nil, err
	}
	return &guardedPreflight{
		observed: observed, desired: desired, checks: checks, blockers: blockers,
		plan: plan, affectedNodes: resolvedNodes, confirmation: confirmation, topologyHash: hash,
	}, nil
}

func (h *guardedOperationsHandler) phase3ServicesOperationInputs(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	operationType string,
	desired []byte,
) ([]string, []byte, error) {
	var state phase3ServicesDesired
	if json.Unmarshal(desired, &state) != nil || state.Operation != operationType ||
		state.Primary == "" || len(state.Nodes) == 0 || !validPhase3ServicesParams(operationType, state.Change) {
		return nil, nil, errors.New("phase 3 service desired state is invalid")
	}
	if operationType == storage.OperationTypePgBouncerAdmin {
		if state.PgBouncer == nil || !state.PgBouncerEnabled {
			return nil, nil, errors.New("PgBouncer desired state is invalid")
		}
		capacity, ok := pgbouncerCapacity(*state.PgBouncer, state.PgBouncerProcesses, state.Databases)
		if !ok ||
			state.PostgreSQLMaxConnections < 1 || capacity > state.PostgreSQLMaxConnections {
			return nil, nil, errors.New("PgBouncer desired state is invalid")
		}
	}

	envs, extraVars, err := h.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		return nil, nil, err
	}
	change := state.Change
	extraVars["phase3_operation"] = operationType
	extraVars["phase3_primary"] = state.Primary
	extraVars["phase3_nodes"] = state.Nodes
	extraVars["cloud_provider"] = ""
	tags := []string{"phase3_services"}

	switch operationType {
	case storage.OperationTypeExtensionAdmin:
		spec := supportedExtensions[change.Extension]
		extraVars["phase3_extension_action"] = change.ExtensionAction
		extraVars["phase3_extension"] = spec.Name
		extraVars["phase3_extension_database"] = change.Database
		extraVars["phase3_extension_schema"] = change.Schema
		extraVars[spec.Enable] = change.ExtensionAction == extensionPresent
		item := map[string]any{
			"ext": change.Extension, "db": change.Database,
			"state": actionState(change.ExtensionAction), "cascade": false,
		}
		if change.Schema != "" {
			item["schema"] = change.Schema
		}
		extraVars["postgresql_extensions"] = []map[string]any{item}
		tags = append(tags, "postgresql_extensions")
		if change.ExtensionAction == extensionPresent {
			tags = append(tags, spec.Tag)
		}
	case storage.OperationTypePgBouncerAdmin:
		config := *state.PgBouncer
		extraVars["phase3_pgbouncer_action"] = change.PgBouncerAction
		extraVars["phase3_pool_name"] = change.PoolName
		extraVars["phase3_pool_database"] = change.Database
		extraVars["phase3_pool_size"] = optionalInt(change.PoolSize)
		extraVars["phase3_pool_mode"] = change.PoolMode
		extraVars["phase3_max_client_connections"] = optionalInt(change.MaxClientConnections)
		extraVars["phase3_max_database_connections"] = optionalInt(change.MaxDatabaseConnections)
		extraVars["phase3_default_pool_size"] = optionalInt(change.DefaultPoolSize)
		extraVars["phase3_query_wait_timeout"] = optionalInt(change.QueryWaitTimeout)
		extraVars["pgbouncer_install"] = true
		extraVars["pgbouncer_pools"] = config.Pools
		extraVars["pgbouncer_max_client_conn"] = config.MaxClientConnections
		extraVars["pgbouncer_max_db_connections"] = config.MaxDatabaseConnections
		extraVars["pgbouncer_default_pool_size"] = config.DefaultPoolSize
		extraVars["pgbouncer_query_wait_timeout"] = config.QueryWaitTimeout
		tags = append(tags, "pgbouncer_conf")
	}
	envs = append(envs, "ANSIBLE_RUN_TAGS="+strings.Join(tags, ","))
	payload, err := json.Marshal(extraVars)
	return envs, payload, err
}
func phase3ServicesOperationParams(operationType string, desired []byte) ([]byte, error) {
	var state phase3ServicesDesired
	if json.Unmarshal(desired, &state) != nil || state.Operation != operationType ||
		state.Primary == "" || len(state.Nodes) == 0 || !validPhase3ServicesParams(operationType, state.Change) {
		return nil, errors.New("phase 3 service parameters are invalid")
	}
	return mustJSON(state.Change), nil
}

func validPhase3ServicesParams(operationType string, change phase3ServicesParams) bool {
	switch operationType {
	case storage.OperationTypeExtensionAdmin:
		_, supported := supportedExtensions[change.Extension]
		common := supported && databaseAdminIdentifier.MatchString(change.Database) &&
			change.PgBouncerAction == "" && change.PoolName == "" && change.PoolSize == nil &&
			change.PoolMode == "" && noPgBouncerLimits(change)
		if change.ExtensionAction == extensionPresent {
			return common && databaseAdminIdentifier.MatchString(change.Schema)
		}
		return common && change.ExtensionAction == extensionAbsent && change.Schema == ""
	case storage.OperationTypePgBouncerAdmin:
		if change.ExtensionAction != "" || change.Extension != "" || change.Schema != "" {
			return false
		}
		switch change.PgBouncerAction {
		case poolPresent:
			return databaseAdminIdentifier.MatchString(change.PoolName) &&
				databaseAdminIdentifier.MatchString(change.Database) && validRange(change.PoolSize, 1, 10000) &&
				(change.PoolMode == "session" || change.PoolMode == "transaction") && noPgBouncerLimits(change)
		case poolAbsent:
			return databaseAdminIdentifier.MatchString(change.PoolName) && change.Database == "" &&
				change.PoolSize == nil && change.PoolMode == "" && noPgBouncerLimits(change)
		case poolLimitsUpdate:
			return change.PoolName == "" && change.Database == "" && change.PoolSize == nil &&
				change.PoolMode == "" && somePgBouncerLimit(change) &&
				validOptionalRange(change.MaxClientConnections, 1, 1000000) &&
				validOptionalRange(change.MaxDatabaseConnections, 1, 100000) &&
				validOptionalRange(change.DefaultPoolSize, 1, 10000) &&
				validOptionalRange(change.QueryWaitTimeout, 1, 86400)
		default:
			return false
		}
	default:
		return false
	}
}

func phase3InventoryNodes(
	nodes []topologyNode,
	inventory map[string]lifecycleInventoryHost,
) (string, []string, bool) {
	primary, resolved := "", make([]string, 0, len(nodes))
	seen := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		host, ok := lifecycleInventoryTarget(inventory, node.Name)
		if !ok || seen[host.Name] {
			return "", nil, false
		}
		seen[host.Name] = true
		resolved = append(resolved, host.Name)
		if leaderRole(node.Role) && healthyStatus(node.Status) {
			primary = host.Name
		}
	}
	sort.Strings(resolved)
	return primary, resolved, len(resolved) > 0
}

func namedTopology(nodes []topologyNode) bool {
	if len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		if node.Name == "" {
			return false
		}
	}
	return true
}

func pgbouncerConfigFromExtraVars(
	raw []byte,
) (pgbouncerConfig, bool, int32, int32, []string, bool) {
	config := pgbouncerConfig{
		Pools: []pgbouncerPool{
			{Name: "postgres", DBName: "postgres", PoolParameters: ""},
		},
		MaxClientConnections: defaultMaxClients, MaxDatabaseConnections: defaultMaxDBConns,
		DefaultPoolSize: defaultPoolSize, QueryWaitTimeout: defaultQueryWait,
	}
	values := map[string]any{}
	if len(raw) != 0 && json.Unmarshal(raw, &values) != nil {
		return config, false, 0, 0, nil, false
	}
	enabled, ok := boolValue(values["pgbouncer_install"], true)
	if !ok {
		return config, false, 0, 0, nil, false
	}
	if pools, exists := values["pgbouncer_pools"]; exists {
		encoded, err := json.Marshal(pools)
		if err != nil || json.Unmarshal(encoded, &config.Pools) != nil {
			return config, enabled, 0, 0, nil, false
		}
	}
	var valid bool
	if config.MaxClientConnections, valid = intConfig(values, "pgbouncer_max_client_conn", defaultMaxClients); !valid {
		return config, enabled, 0, 0, nil, false
	}
	if config.MaxDatabaseConnections, valid = intConfig(values, "pgbouncer_max_db_connections", defaultMaxDBConns); !valid {
		return config, enabled, 0, 0, nil, false
	}
	if config.DefaultPoolSize, valid = intConfig(values, "pgbouncer_default_pool_size", defaultPoolSize); !valid {
		return config, enabled, 0, 0, nil, false
	}
	if config.QueryWaitTimeout, valid = intConfig(values, "pgbouncer_query_wait_timeout", defaultQueryWait); !valid {
		return config, enabled, 0, 0, nil, false
	}
	maxConnections := int32(1000)
	for _, key := range []string{
		"postgresql_parameters", "postgresql_parameters_overrides", "local_postgresql_parameters",
	} {
		if configured, found, configOK := postgresqlOption(values[key], "max_connections"); !configOK {
			return config, enabled, 0, 0, nil, false
		} else if found {
			maxConnections = configured
		}
	}
	processes, valid := intConfig(values, "pgbouncer_processes", 1)
	if !valid || processes < 1 || !validPgBouncerConfig(config) {
		return config, enabled, 0, 0, nil, false
	}
	databases, valid := configuredDatabases(values["postgresql_databases"])
	return config, enabled, maxConnections, processes, databases, valid
}

func mergePgBouncerConfig(current pgbouncerConfig, change phase3ServicesParams) pgbouncerConfig {
	next := current
	next.Pools = append([]pgbouncerPool(nil), current.Pools...)
	switch change.PgBouncerAction {
	case poolPresent:
		pool := pgbouncerPool{
			Name: change.PoolName, DBName: change.Database,
			PoolParameters: map[string]any{"pool_size": *change.PoolSize, "pool_mode": change.PoolMode},
		}
		found := false
		for i := range next.Pools {
			if next.Pools[i].Name == change.PoolName {
				next.Pools[i], found = pool, true
			}
		}
		if !found {
			next.Pools = append(next.Pools, pool)
		}
	case poolAbsent:
		pools := make([]pgbouncerPool, 0, len(next.Pools))
		for _, pool := range next.Pools {
			if pool.Name != change.PoolName {
				pools = append(pools, pool)
			}
		}
		next.Pools = pools
	case poolLimitsUpdate:
		setOptional(&next.MaxClientConnections, change.MaxClientConnections)
		setOptional(&next.MaxDatabaseConnections, change.MaxDatabaseConnections)
		setOptional(&next.DefaultPoolSize, change.DefaultPoolSize)
		setOptional(&next.QueryWaitTimeout, change.QueryWaitTimeout)
	}
	sort.Slice(next.Pools, func(i, j int) bool { return next.Pools[i].Name < next.Pools[j].Name })
	return next
}

func validPgBouncerConfig(config pgbouncerConfig) bool {
	if config.MaxClientConnections < 1 || config.MaxDatabaseConnections < 1 ||
		config.DefaultPoolSize < 1 || config.QueryWaitTimeout < 1 {
		return false
	}
	seen := make(map[string]bool, len(config.Pools))
	for _, pool := range config.Pools {
		if !databaseAdminIdentifier.MatchString(pool.Name) ||
			!databaseAdminIdentifier.MatchString(pool.DBName) || seen[pool.Name] {
			return false
		}
		seen[pool.Name] = true
	}
	return true
}

func pgbouncerCapacity(config pgbouncerConfig, processes int32, databases []string) (int32, bool) {
	if !validPgBouncerConfig(config) || processes < 1 {
		return 0, false
	}
	var total int64
	configured := make(map[string]bool, len(config.Pools))
	for _, pool := range config.Pools {
		size, ok := configuredPoolSize(pool.PoolParameters, config.DefaultPoolSize)
		if !ok {
			return 0, false
		}
		total += int64(size)
		configured[pool.DBName] = true
	}
	for _, database := range databases {
		if !configured[database] {
			total += int64(config.DefaultPoolSize)
		}
	}
	total *= int64(processes)
	return int32(total), total <= math.MaxInt32
}

func configuredPoolSize(parameters any, fallback int32) (int32, bool) {
	switch value := parameters.(type) {
	case nil:
		return fallback, true
	case string:
		match := poolSizePattern.FindStringSubmatch(value)
		if len(match) == 0 {
			return fallback, true
		}
		parsed, err := strconv.ParseInt(match[1], 10, 32)
		return int32(parsed), err == nil && parsed > 0
	case map[string]any:
		if raw, ok := value["pool_size"]; ok {
			size, valid := intValue(raw)
			return size, valid && size > 0
		}
		return fallback, true
	default:
		return 0, false
	}
}

func intConfig(values map[string]any, key string, fallback int32) (int32, bool) {
	value, exists := values[key]
	if !exists {
		return fallback, true
	}
	return intValue(value)
}

func intValue(value any) (int32, bool) {
	switch typed := value.(type) {
	case float64:
		return int32(typed), typed == math.Trunc(typed) && typed >= 0 && typed <= math.MaxInt32
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 32)
		return int32(parsed), err == nil && parsed >= 0
	case int:
		return int32(typed), typed >= 0 && int64(typed) <= math.MaxInt32
	case int32:
		return typed, typed >= 0
	default:
		return 0, false
	}
}

func boolValue(value any, fallback bool) (bool, bool) {
	if value == nil {
		return fallback, true
	}
	parsed, ok := value.(bool)
	return parsed, ok
}

func postgresqlOption(raw any, option string) (int32, bool, bool) {
	if raw == nil {
		return 0, false, true
	}
	items, ok := raw.([]any)
	if !ok {
		return 0, false, false
	}
	var result int32
	found := false
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return 0, false, false
		}
		if entry["option"] == option {
			value, valid := intValue(entry["value"])
			if !valid || value < 1 {
				return 0, false, false
			}
			result, found = value, true
		}
	}
	return result, found, true
}

func configuredDatabases(raw any) ([]string, bool) {
	if raw == nil {
		return nil, true
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	databases := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		name, named := entry["db"].(string)
		if !named || !databaseAdminIdentifier.MatchString(name) {
			return nil, false
		}
		databases = append(databases, name)
	}
	sort.Strings(databases)
	return databases, true
}

func configuredExtension(raw []byte, change phase3ServicesParams) any {
	values := map[string]any{}
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	items, ok := values["postgresql_extensions"].([]any)
	if !ok {
		return nil
	}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if ok && entry["ext"] == change.Extension && entry["db"] == change.Database {
			return entry
		}
	}
	return nil
}

func phase3ServicesPlan(operationType string, change phase3ServicesParams) ([]string, string) {
	if operationType == storage.OperationTypeExtensionAdmin {
		if change.ExtensionAction == extensionPresent {
			return []string{
					"recheck healthy topology and cluster mutation lock",
					"install supported " + change.Extension + " package on every PostgreSQL node",
					"create extension in " + change.Database + " and verify availability on every node",
				},
				"INSTALL EXTENSION " + change.Extension + " IN " + change.Database
		}
		return []string{
				"recheck healthy topology and cluster mutation lock",
				"drop extension " + change.Extension + " from " + change.Database + " without cascade",
				"verify extension absent from live PostgreSQL catalog",
			},
			"DROP EXTENSION " + change.Extension + " FROM " + change.Database + " WITHOUT CASCADE"
	}
	description := "update supported PgBouncer limits"
	confirmation := "UPDATE PGBOUNCER LIMITS"
	if change.PgBouncerAction == poolPresent {
		description = "create or reconcile PgBouncer pool " + change.PoolName + " for " + change.Database
		confirmation = "PGBOUNCER POOL " + change.PoolName + " DATABASE " + change.Database
	} else if change.PgBouncerAction == poolAbsent {
		description = "remove PgBouncer pool " + change.PoolName
		confirmation = "REMOVE PGBOUNCER POOL " + change.PoolName
	}
	return []string{
		"recheck healthy topology, pool capacity, and cluster mutation lock",
		description + " on every PostgreSQL node",
		"reload and verify live PgBouncer configuration on every node",
	}, confirmation
}

func optionalInt(value *int32) int32 {
	if value == nil {
		return -1
	}
	return *value
}

func setOptional(target *int32, value *int32) {
	if value != nil {
		*target = *value
	}
}

func validRange(value *int32, minimum, maximum int32) bool {
	return value != nil && *value >= minimum && *value <= maximum
}

func validOptionalRange(value *int32, minimum, maximum int32) bool {
	return value == nil || validRange(value, minimum, maximum)
}

func noPgBouncerLimits(change phase3ServicesParams) bool {
	return change.MaxClientConnections == nil && change.MaxDatabaseConnections == nil &&
		change.DefaultPoolSize == nil && change.QueryWaitTimeout == nil
}

func somePgBouncerLimit(change phase3ServicesParams) bool {
	return !noPgBouncerLimits(change)
}
