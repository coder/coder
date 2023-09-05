package coderd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

const (
	PubsubEventLicenses = "licenses"
)

// key20220812 is the Coder license public key with id 2022-08-12 used to validate licenses signed
// by our signing infrastructure
//
//go:embed keys/2022-08-12
var key20220812 []byte

var Keys = map[string]ed25519.PublicKey{"2022-08-12": ed25519.PublicKey(key20220812)}

// postLicense adds a new Enterprise license to the cluster.  We allow multiple different licenses
// in the cluster at one time for several reasons:
//
//  1. Upgrades --- if the license format changes from one version of Coder to the next, during a
//     rolling update you will have different Coder servers that need different licenses to function.
//  2. Avoid abrupt feature breakage --- when an admin uploads a new license with different features
//     we generally don't want the old features to immediately break without warning.  With a grace
//     period on the license, features will continue to work from the old license until its grace
//     period, then the users will get a warning allowing them to gracefully stop using the feature.
//
// @Summary Add new license
// @ID add-new-license
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Organizations
// @Param request body codersdk.AddLicenseRequest true "Add license request"
// @Success 201 {object} codersdk.License
// @Router /licenses [post]
func (api *API) postLicense(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.License](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	if !api.AGPL.Authorize(r, rbac.ActionCreate, rbac.ResourceLicense) {
		httpapi.Forbidden(rw)
		return
	}

	var addLicense codersdk.AddLicenseRequest
	if !httpapi.Read(ctx, rw, r, &addLicense) {
		return
	}

	rawClaims, err := license.ParseRaw(addLicense.License, api.Keys)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid license",
			Detail:  err.Error(),
		})
		return
	}
	exp, ok := rawClaims["exp"].(float64)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid license",
			Detail:  "exp claim missing or not parsable",
		})
		return
	}
	expTime := time.Unix(int64(exp), 0)

	claims, err := license.ParseClaims(addLicense.License, api.Keys)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid license",
			Detail:  err.Error(),
		})
		return
	}

	id, err := uuid.Parse(claims.ID)
	if err != nil {
		// If no uuid is in the license, we generate a random uuid.
		// This is not ideal, and this should be fixed to require a uuid
		// for all licenses. We require this patch to support older licenses.
		// TODO: In the future (April 2023?) we should remove this and reissue
		// old licenses with a uuid.
		id = uuid.New()
	}
	dl, err := api.Database.InsertLicense(ctx, database.InsertLicenseParams{
		UploadedAt: dbtime.Now(),
		JWT:        addLicense.License,
		Exp:        expTime,
		UUID:       id,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Unable to add license to database",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = dl

	err = api.updateEntitlements(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update entitlements",
			Detail:  err.Error(),
		})
		return
	}
	err = api.Pubsub.Publish(PubsubEventLicenses, []byte("add"))
	if err != nil {
		api.Logger.Error(context.Background(), "failed to publish license add", slog.Error(err))
		// don't fail the HTTP request, since we did write it successfully to the database
	}

	httpapi.Write(ctx, rw, http.StatusCreated, convertLicense(dl, rawClaims))
}

// postRefreshEntitlements forces an `updateEntitlements` call and publishes
// a message to the PubsubEventLicenses topic to force other replicas
// to update their entitlements.
// Updates happen automatically on a timer, however that time is every 10 minutes,
// and we want to be able to force an update immediately in some cases.
//
// @Summary Update license entitlements
// @ID update-license-entitlements
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Success 201 {object} codersdk.Response
// @Router /licenses/refresh-entitlements [post]
func (api *API) postRefreshEntitlements(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// If the user cannot create a new license, then they cannot refresh entitlements.
	// Refreshing entitlements is a way to force a refresh of the license, so it is
	// equivalent to creating a new license.
	if !api.AGPL.Authorize(r, rbac.ActionCreate, rbac.ResourceLicense) {
		httpapi.Forbidden(rw)
		return
	}

	// Prevent abuse by limiting how often we allow a forced refresh.
	now := time.Now()
	if diff := now.Sub(api.entitlements.RefreshedAt); diff < time.Minute {
		wait := time.Minute - diff
		rw.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Entitlements already recently refreshed, please wait %d seconds to force a new refresh", int(wait.Seconds())),
			Detail:  fmt.Sprintf("Last refresh at %s", now.UTC().String()),
		})
		return
	}

	err := api.replicaManager.UpdateNow(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to sync replicas",
			Detail:  err.Error(),
		})
		return
	}

	err = api.updateEntitlements(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update entitlements",
			Detail:  err.Error(),
		})
		return
	}

	err = api.Pubsub.Publish(PubsubEventLicenses, []byte("refresh"))
	if err != nil {
		api.Logger.Error(context.Background(), "failed to publish forced entitlement update", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to publish forced entitlement update. Other replicas might not be updated.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Entitlements updated",
	})
}

// @Summary Get licenses
// @ID get-licenses
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {array} codersdk.License
// @Router /licenses [get]
func (api *API) licenses(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	licenses, err := api.Database.GetLicenses(ctx)
	if xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusOK, []codersdk.License{})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching licenses.",
			Detail:  err.Error(),
		})
		return
	}

	licenses, err = coderd.AuthorizeFilter(api.AGPL.HTTPAuth, r, rbac.ActionRead, licenses)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching licenses.",
			Detail:  err.Error(),
		})
		return
	}
	sdkLicenses, err := convertLicenses(licenses)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error parsing licenses.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, sdkLicenses)
}

// @Summary Delete license
// @ID delete-license
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param id path string true "License ID" format(number)
// @Success 200
// @Router /licenses/{id} [delete]
func (api *API) deleteLicense(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		auditor = api.AGPL.Auditor.Load()
	)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "License ID must be an integer",
		})
		return
	}

	dl, err := api.Database.GetLicenseByID(ctx, int32(id))
	if err != nil {
		// don't fail the HTTP request simply because we cannot audit
		api.Logger.Warn(context.Background(), "could not retrieve license; cannot audit", slog.Error(err))
	}

	aReq, commitAudit := audit.InitRequest[database.License](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionDelete,
	})
	defer commitAudit()
	aReq.Old = dl

	if !api.AGPL.Authorize(r, rbac.ActionDelete, rbac.ResourceLicense) {
		httpapi.Forbidden(rw)
		return
	}

	_, err = api.Database.DeleteLicense(ctx, int32(id))
	if httpapi.Is404Error(err) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Unknown license ID",
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting license",
			Detail:  err.Error(),
		})
		return
	}
	err = api.updateEntitlements(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update entitlements",
			Detail:  err.Error(),
		})
		return
	}
	err = api.Pubsub.Publish(PubsubEventLicenses, []byte("delete"))
	if err != nil {
		api.Logger.Error(context.Background(), "failed to publish license delete", slog.Error(err))
		// don't fail the HTTP request, since we did write it successfully to the database
	}
	rw.WriteHeader(http.StatusOK)
}

func convertLicense(dl database.License, c jwt.MapClaims) codersdk.License {
	return codersdk.License{
		ID:         dl.ID,
		UUID:       dl.UUID,
		UploadedAt: dl.UploadedAt,
		Claims:     c,
	}
}

func convertLicenses(licenses []database.License) ([]codersdk.License, error) {
	var out []codersdk.License
	for _, l := range licenses {
		c, err := decodeClaims(l)
		if err != nil {
			return nil, err
		}
		out = append(out, convertLicense(l, c))
	}
	return out, nil
}

// decodeClaims decodes the JWT claims from the stored JWT.  Note here we do not validate the JWT
// and just return the claims verbatim.  We want to include all licenses on the GET response, even
// if they are expired, or signed by a key this version of Coder no longer considers valid.
//
// Also, we do not return the whole JWT itself because a signed JWT is a bearer token and we
// want to limit the chance of it being accidentally leaked.
func decodeClaims(l database.License) (jwt.MapClaims, error) {
	parts := strings.Split(l.JWT, ".")
	if len(parts) != 3 {
		return nil, xerrors.Errorf("Unable to parse license %d as JWT", l.ID)
	}
	cb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, xerrors.Errorf("Unable to decode license %d claims: %w", l.ID, err)
	}
	c := make(jwt.MapClaims)
	d := json.NewDecoder(bytes.NewBuffer(cb))
	d.UseNumber()
	err = d.Decode(&c)
	return c, err
}
