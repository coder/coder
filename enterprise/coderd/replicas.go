package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// replicas returns the number of replicas that are active in Coder.
func (api *API) replicas(rw http.ResponseWriter, r *http.Request) {
	if !api.AGPL.Authorize(r, rbac.ActionRead, rbac.ResourceReplicas) {
		httpapi.ResourceNotFound(rw)
		return
	}

	replicas := api.replicaManager.All()
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
