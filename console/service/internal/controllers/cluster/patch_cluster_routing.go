package cluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"postgresql-cluster-console/internal/controllers"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
)

var editableRoutingRoles = map[string]bool{
	"primary": true, "replica": true, "replica_sync": true, "replica_async": true,
}

type routingPatchTarget struct {
	Addresses []string
	Port      int64
}

type patchClusterRoutingHandler struct {
	db storage.IStorage
}

func NewPatchClusterRoutingHandler(db storage.IStorage) clusterapi.PatchClustersIDRoutingHandler {
	return &patchClusterRoutingHandler{db: db}
}

func (h *patchClusterRoutingHandler) Handle(
	param clusterapi.PatchClustersIDRoutingParams,
) middleware.Responder {
	patch, err := decodeRoutingPatch(param.Body)
	if err != nil {
		return routingBadRequest(err)
	}

	ctx := param.HTTPRequest.Context()
	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return routingBadRequest(errors.New("cluster is unavailable"))
	}

	routing := storedRouting(clusterInfo.ConnectionInfo)
	for role, target := range patch {
		if target == nil {
			delete(routing, role)
		} else {
			routing[role] = *target
		}
	}
	if primary, ok := routing["primary"]; !ok || len(primary.Addresses) == 0 || primary.Port == 0 {
		return routingBadRequest(errors.New("primary routing target is required"))
	}

	connectionInfo := healthObject(clusterInfo.ConnectionInfo)
	if connectionInfo == nil {
		connectionInfo = make(map[string]any)
	}
	addresses, ports := map[string]any{}, map[string]any{}
	for _, role := range []string{"primary", "replica", "replica_sync", "replica_async"} {
		target, ok := routing[role]
		if !ok {
			continue
		}
		addresses[role] = strings.Join(target.Addresses, ", ")
		ports[role] = target.Port
	}
	connectionInfo["address"] = addresses
	connectionInfo["port"] = ports

	clusterInfo, err = h.db.UpdateCluster(ctx, &storage.UpdateClusterReq{
		ID:             clusterInfo.ID,
		ConnectionInfo: connectionInfo,
	})
	if err != nil {
		return routingBadRequest(errors.New("routing metadata could not be saved"))
	}
	response, err := getClusterInfo(ctx, h.db, clusterInfo)
	if err != nil {
		return routingBadRequest(errors.New("cluster is unavailable"))
	}
	return clusterapi.NewPatchClustersIDRoutingOK().WithPayload(response)
}

func decodeRoutingPatch(body any) (map[string]*routingPatchTarget, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, errors.New("routing update is invalid")
	}
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(data, &raw); err != nil || len(raw) == 0 {
		return nil, errors.New("at least one routing role is required")
	}

	patch := make(map[string]*routingPatchTarget, len(raw))
	for role, value := range raw {
		if !editableRoutingRoles[role] {
			return nil, fmt.Errorf("routing role %q is unsupported", role)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			patch[role] = nil
			continue
		}

		var target models.RequestClusterRoutingTarget
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&target); err != nil || target.Validate(strfmt.Default) != nil {
			return nil, fmt.Errorf("routing target %q is invalid", role)
		}
		addresses, normalizeErr := normalizeRoutingAddresses(target.Addresses)
		if normalizeErr != nil {
			return nil, fmt.Errorf("routing target %q: %w", role, normalizeErr)
		}
		patch[role] = &routingPatchTarget{Addresses: addresses, Port: *target.Port}
	}
	return patch, nil
}

func normalizeRoutingAddresses(values []string) ([]string, error) {
	addresses := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		address, ok := normalizeRoutingAddress(value)
		if !ok {
			return nil, fmt.Errorf("address %q is invalid", value)
		}
		if !seen[address] {
			seen[address] = true
			addresses = append(addresses, address)
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("at least one address is required")
	}
	return addresses, nil
}

func normalizeRoutingAddress(value string) (string, bool) {
	address := strings.TrimSpace(value)
	ipAddress := address
	if strings.HasPrefix(address, "[") || strings.HasSuffix(address, "]") {
		if !strings.HasPrefix(address, "[") || !strings.HasSuffix(address, "]") {
			return "", false
		}
		ipAddress = strings.TrimSuffix(strings.TrimPrefix(address, "["), "]")
	}
	if ip := net.ParseIP(ipAddress); ip != nil {
		return ip.String(), true
	}
	address = strings.TrimSuffix(strings.ToLower(address), ".")
	if address == "" || address == "n/a" || len(address) > 253 {
		return "", false
	}
	for _, label := range strings.Split(address, ".") {
		if len(label) == 0 || len(label) > 63 || !isRoutingAlphaNumeric(label[0]) ||
			!isRoutingAlphaNumeric(label[len(label)-1]) {
			return "", false
		}
		for index := 1; index < len(label)-1; index++ {
			if !isRoutingAlphaNumeric(label[index]) && label[index] != '-' {
				return "", false
			}
		}
	}
	return address, true
}

func isRoutingAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func storedRouting(connectionInfo any) map[string]routingPatchTarget {
	routing := make(map[string]routingPatchTarget)
	for _, target := range healthRouting(connectionInfo).Targets {
		if target == nil || !editableRoutingRoles[target.Role] || target.Address == "" || target.Port == nil {
			continue
		}
		current := routing[target.Role]
		current.Port = *target.Port
		if !routingContainsAddress(current.Addresses, target.Address) {
			current.Addresses = append(current.Addresses, target.Address)
		}
		routing[target.Role] = current
	}
	return routing
}

func routingContainsAddress(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func routingBadRequest(err error) middleware.Responder {
	return clusterapi.NewPatchClustersIDRoutingBadRequest().WithPayload(
		controllers.MakeErrorPayload(err, controllers.BaseError),
	)
}
