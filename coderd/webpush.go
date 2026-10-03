package coderd

import (
	"database/sql"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/webpush"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Create user webpush subscription
// @ID create-user-webpush-subscription
// @Security CoderSessionToken
// @Accept json
// @Tags Notifications
// @Param request body codersdk.WebpushSubscription true "Webpush subscription"
// @Param user path string true "User ID, name, or me"
// @Router /api/v2/users/{user}/webpush/subscription [post]
// @Success 204
// @x-apidocgen {"skip": true}
func (api *API) postUserWebpushSubscription(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)
	var req codersdk.WebpushSubscription
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if err := validateWebpushEndpoint(req.Endpoint); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid webpush endpoint.",
			Detail:  err.Error(),
		})
		return
	}

	if err := api.WebpushDispatcher.Test(ctx, req); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to test webpush subscription",
			Detail:  err.Error(),
		})
		return
	}

	if _, err := api.Database.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
		CreatedAt:         dbtime.Now(),
		UserID:            user.ID,
		Endpoint:          req.Endpoint,
		EndpointAuthKey:   req.AuthKey,
		EndpointP256dhKey: req.P256DHKey,
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to insert push notification subscription.",
			Detail:  err.Error(),
		})
		return
	}
	if invalidator, ok := api.WebpushDispatcher.(webpush.SubscriptionCacheInvalidator); ok {
		invalidator.InvalidateUser(user.ID)
	}

	rw.WriteHeader(http.StatusNoContent)
}

func validateWebpushEndpoint(rawEndpoint string) error {
	endpoint, err := url.Parse(rawEndpoint)
	if err != nil {
		return xerrors.Errorf("parse endpoint URL: %w", err)
	}
	if !endpoint.IsAbs() {
		return xerrors.New("endpoint must be an absolute URL")
	}
	if endpoint.Scheme != "https" {
		return xerrors.New("endpoint URL scheme must be https")
	}
	if endpoint.Host == "" {
		return xerrors.New("endpoint host is required")
	}
	if endpoint.User != nil {
		return xerrors.New("endpoint URL must not include userinfo")
	}

	hostname := strings.ToLower(endpoint.Hostname())
	if hostname == "" {
		return xerrors.New("endpoint hostname is required")
	}
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") {
		return xerrors.New("endpoint hostname must not be localhost")
	}

	if ip, err := netip.ParseAddr(hostname); err == nil &&
		(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsMulticast() ||
			ip.IsUnspecified()) {
		return xerrors.New("endpoint IP must not be private, loopback, link-local, multicast, or unspecified")
	}

	return nil
}

// @Summary Delete user webpush subscription
// @ID delete-user-webpush-subscription
// @Security CoderSessionToken
// @Accept json
// @Tags Notifications
// @Param request body codersdk.DeleteWebpushSubscription true "Webpush subscription"
// @Param user path string true "User ID, name, or me"
// @Router /api/v2/users/{user}/webpush/subscription [delete]
// @Success 204
// @x-apidocgen {"skip": true}
func (api *API) deleteUserWebpushSubscription(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	var req codersdk.DeleteWebpushSubscription
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Return NotFound if the subscription does not exist.
	existing, err := api.Database.GetWebpushSubscriptionsByUserID(ctx, user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get webpush subscriptions.",
			Detail:  err.Error(),
		})
		return
	}
	if idx := slices.IndexFunc(existing, func(s database.WebpushSubscription) bool {
		return s.Endpoint == req.Endpoint
	}); idx == -1 {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Webpush subscription not found.",
		})
		return
	}

	if err := api.Database.DeleteWebpushSubscriptionByUserIDAndEndpoint(ctx, database.DeleteWebpushSubscriptionByUserIDAndEndpointParams{
		UserID:   user.ID,
		Endpoint: req.Endpoint,
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Webpush subscription not found.",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete push notification subscription.",
			Detail:  err.Error(),
		})
		return
	}
	if invalidator, ok := api.WebpushDispatcher.(webpush.SubscriptionCacheInvalidator); ok {
		invalidator.InvalidateUser(user.ID)
	}

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Send a test push notification
// @ID send-a-test-push-notification
// @Security CoderSessionToken
// @Tags Notifications
// @Param user path string true "User ID, name, or me"
// @Success 204
// @Router /api/v2/users/{user}/webpush/test [post]
// @x-apidocgen {"skip": true}
func (api *API) postUserPushNotificationTest(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := httpmw.UserParam(r)

	// We need to authorize the user to send a push notification to themselves.
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceNotificationMessage.WithOwner(user.ID.String())) {
		httpapi.Forbidden(rw)
		return
	}

	if err := api.WebpushDispatcher.Dispatch(ctx, user.ID, codersdk.WebpushMessage{
		Title: "It's working!",
		Body:  "You've subscribed to push notifications.",
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to send test notification",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
