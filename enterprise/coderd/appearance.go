package coderd

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

var DefaultSupportLinks = []codersdk.LinkConfig{
	{
		Name:   "Documentation",
		Target: "https://coder.com/docs/coder-oss",
		Icon:   "docs",
	},
	{
		Name:   "Report a bug",
		Target: "https://github.com/coder/coder/issues/new?labels=needs+grooming&body={CODER_BUILD_INFO}",
		Icon:   "bug",
	},
	{
		Name:   "Join the Coder Discord",
		Target: "https://coder.com/chat?utm_source=coder&utm_medium=coder&utm_campaign=server-footer",
		Icon:   "chat",
	},
}

// @Summary Get appearance
// @ID get-appearance
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.AppearanceConfig
// @Router /appearance [get]
func (api *API) appearance(rw http.ResponseWriter, r *http.Request) {
	cfg, err := api.fetchAppearanceConfig(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch appearance config.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, cfg)
}

func (api *API) fetchAppearanceConfig(ctx context.Context) (codersdk.AppearanceConfig, error) {
	api.entitlementsMu.RLock()
	isEntitled := api.entitlements.Features[codersdk.FeatureAppearance].Entitlement == codersdk.EntitlementEntitled
	api.entitlementsMu.RUnlock()

	if !isEntitled {
		return codersdk.AppearanceConfig{
			SupportLinks: DefaultSupportLinks,
		}, nil
	}

	var eg errgroup.Group
	var logoURL string
	var serviceBannerJSON string
	eg.Go(func() (err error) {
		logoURL, err = api.Database.GetLogoURL(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get logo url: %w", err)
		}
		return nil
	})
	eg.Go(func() (err error) {
		serviceBannerJSON, err = api.Database.GetServiceBanner(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get service banner: %w", err)
		}
		return nil
	})
	err := eg.Wait()
	if err != nil {
		return codersdk.AppearanceConfig{}, err
	}

	cfg := codersdk.AppearanceConfig{
		LogoURL: logoURL,
	}
	if serviceBannerJSON != "" {
		err = json.Unmarshal([]byte(serviceBannerJSON), &cfg.ServiceBanner)
		if err != nil {
			return codersdk.AppearanceConfig{}, xerrors.Errorf(
				"unmarshal json: %w, raw: %s", err, serviceBannerJSON,
			)
		}
	}

	if len(api.DeploymentValues.Support.Links.Value) == 0 {
		cfg.SupportLinks = DefaultSupportLinks
	} else {
		cfg.SupportLinks = api.DeploymentValues.Support.Links.Value
	}

	return cfg, nil
}

func validateHexColor(color string) error {
	if len(color) != 7 {
		return xerrors.New("expected 7 characters")
	}
	if color[0] != '#' {
		return xerrors.New("no # prefix")
	}
	_, err := hex.DecodeString(color[1:])
	return err
}

// @Summary Update appearance
// @ID update-appearance
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.UpdateAppearanceConfig true "Update appearance request"
// @Success 200 {object} codersdk.UpdateAppearanceConfig
// @Router /appearance [put]
func (api *API) putAppearance(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, rbac.ActionUpdate, rbac.ResourceDeploymentValues) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Insufficient permissions to update appearance",
		})
		return
	}

	var appearance codersdk.UpdateAppearanceConfig
	if !httpapi.Read(ctx, rw, r, &appearance) {
		return
	}

	if appearance.ServiceBanner.Enabled {
		if err := validateHexColor(appearance.ServiceBanner.BackgroundColor); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("parse color: %+v", err),
			})
			return
		}
	}

	serviceBannerJSON, err := json.Marshal(appearance.ServiceBanner)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("marshal banner: %+v", err),
		})
		return
	}

	err = api.Database.UpsertServiceBanner(ctx, string(serviceBannerJSON))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("database error: %+v", err),
		})
		return
	}

	err = api.Database.UpsertLogoURL(ctx, appearance.LogoURL)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("database error: %+v", err),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, appearance)
}
