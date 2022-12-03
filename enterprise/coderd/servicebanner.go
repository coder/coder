package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (api *API) serviceBanner(rw http.ResponseWriter, r *http.Request) {
	api.entitlementsMu.RLock()
	isEntitled := api.entitlements.Features[codersdk.FeatureServiceBanners].Entitlement == codersdk.EntitlementEntitled
	api.entitlementsMu.RUnlock()

	ctx := r.Context()

	if !isEntitled {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.ServiceBanner{
			Enabled: false,
		})
		return
	}

	serviceBannerJSON, err := api.Database.GetServiceBanner(r.Context())
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.ServiceBanner{
			Enabled: false,
		})
		return
	} else if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("database error: %+v", err),
		})
		return
	}

	var serviceBanner codersdk.ServiceBanner
	err = json.Unmarshal([]byte(serviceBannerJSON), &serviceBanner)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("unmarshal json: %+v", err),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, serviceBanner)
}

func (api *API) putServiceBanner(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var serviceBanner codersdk.ServiceBanner
	if !httpapi.Read(ctx, rw, r, &serviceBanner) {
		return
	}

	serviceBannerJSON, err := json.Marshal(serviceBanner)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("marshal banner: %+v", err),
		})
		return
	}

	err = api.Database.InsertOrUpdateServiceBanner(ctx, string(serviceBannerJSON))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("database error: %+v", err),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
		Message: "ok",
	})
}
