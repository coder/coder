package coderd

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get appearance
// @ID get-appearance
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.AppearanceConfig
// @Router /appearance [get]
func (api *API) appearance(rw http.ResponseWriter, r *http.Request) {
	af := *api.AGPL.AppearanceFetcher.Load()
	cfg, err := af.Fetch(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch appearance config.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, cfg)
}

type appearanceFetcher struct {
	database     database.Store
	supportLinks []codersdk.LinkConfig
}

func newAppearanceFetcher(store database.Store, links []codersdk.LinkConfig) agpl.Fetcher {
	return &appearanceFetcher{
		database:     store,
		supportLinks: links,
	}
}

func (f *appearanceFetcher) Fetch(ctx context.Context) (codersdk.AppearanceConfig, error) {
	var eg errgroup.Group
	var applicationName string
	var logoURL string
	var serviceBannerJSON string
	eg.Go(func() (err error) {
		applicationName, err = f.database.GetApplicationName(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get application name: %w", err)
		}
		return nil
	})
	eg.Go(func() (err error) {
		logoURL, err = f.database.GetLogoURL(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get logo url: %w", err)
		}
		return nil
	})
	eg.Go(func() (err error) {
		serviceBannerJSON, err = f.database.GetServiceBanner(ctx)
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
		ApplicationName: applicationName,
		LogoURL:         logoURL,
	}
	if serviceBannerJSON != "" {
		err = json.Unmarshal([]byte(serviceBannerJSON), &cfg.ServiceBanner)
		if err != nil {
			return codersdk.AppearanceConfig{}, xerrors.Errorf(
				"unmarshal json: %w, raw: %s", err, serviceBannerJSON,
			)
		}
	}

	if len(f.supportLinks) == 0 {
		cfg.SupportLinks = agpl.DefaultSupportLinks
	} else {
		cfg.SupportLinks = f.supportLinks
	}

	return cfg, nil
}

func validateHexColor(color string) error {
	if len(color) != 7 {
		return xerrors.New("expected # prefix and 6 characters")
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
				Message: "Invalid color format",
				Detail:  err.Error(),
			})
			return
		}
	}

	serviceBannerJSON, err := json.Marshal(appearance.ServiceBanner)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unable to marshal service banner",
			Detail:  err.Error(),
		})
		return
	}

	err = api.Database.UpsertServiceBanner(ctx, string(serviceBannerJSON))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Unable to set service banner",
			Detail:  err.Error(),
		})
		return
	}

	err = api.Database.UpsertApplicationName(ctx, appearance.ApplicationName)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Unable to set application name",
			Detail:  err.Error(),
		})
		return
	}

	err = api.Database.UpsertLogoURL(ctx, appearance.LogoURL)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Unable to set logo URL",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, appearance)
}
