package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// replicas returns the number of replicas that are active in Coder.
//
// @Summary Get active replicas
// @ID get-active-replicas
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {array} codersdk.Replica
// @Router /replicas [get]
func (api *API) replicas(rw http.ResponseWriter, r *http.Request) {
	if !api.AGPL.Authorize(r, policy.ActionRead, rbac.ResourceReplicas) {
		httpapi.ResourceNotFound(rw)
		return
	}

	replicas := api.replicaManager.AllPrimary()
	res := make([]codersdk.Replica, 0, len(replicas))
	for _, replica := range replicas {
		res = append(res, convertReplica(replica))
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, res)
}

func convertReplica(replica database.Replica) codersdk.Replica {
	return codersdk.Replica{
		ID:              replica.ID,
		Hostname:        replica.Hostname,
		CreatedAt:       replica.CreatedAt,
		RelayAddress:    replica.RelayAddress,
		RegionID:        replica.RegionID,
		Error:           replica.Error,
		DatabaseLatency: replica.DatabaseLatency,
	}
}
