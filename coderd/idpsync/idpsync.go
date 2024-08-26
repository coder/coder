package idpsync

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/entitlements"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/site"
)

// IDPSync is the configuration for syncing user information from an external
// IDP. All related code to syncing user information should be in this package.
type IDPSync struct {
	logger       slog.Logger
	entitlements *entitlements.Set

	// OrganizationField selects the claim field to be used as the created user's
	// organizations. If the field is the empty string, then no organization updates
	// will ever come from the OIDC provider.
	OrganizationField string
	// OrganizationMapping controls how organizations returned by the OIDC provider get mapped
	OrganizationMapping map[string][]uuid.UUID
	// OrganizationAlwaysAssign will ensure all users that authenticate will be
	// placed into the specified organizations.
	OrganizationAlwaysAssign []uuid.UUID
}

func NewSync(logger slog.Logger, set *entitlements.Set) *IDPSync {
	return &IDPSync{
		logger:       logger.Named("idp-sync"),
		entitlements: set,
	}
}

// ParseStringSliceClaim parses the claim for groups and roles, expected []string.
//
// Some providers like ADFS return a single string instead of an array if there
// is only 1 element. So this function handles the edge cases.
func ParseStringSliceClaim(claim interface{}) ([]string, error) {
	groups := make([]string, 0)
	if claim == nil {
		return groups, nil
	}

	// The simple case is the type is exactly what we expected
	asStringArray, ok := claim.([]string)
	if ok {
		return asStringArray, nil
	}

	asArray, ok := claim.([]interface{})
	if ok {
		for i, item := range asArray {
			asString, ok := item.(string)
			if !ok {
				return nil, xerrors.Errorf("invalid claim type. Element %d expected a string, got: %T", i, item)
			}
			groups = append(groups, asString)
		}
		return groups, nil
	}

	asString, ok := claim.(string)
	if ok {
		if asString == "" {
			// Empty string should be 0 groups.
			return []string{}, nil
		}
		// If it is a single string, first check if it is a csv.
		// If a user hits this, it is likely a misconfiguration and they need
		// to reconfigure their IDP to send an array instead.
		if strings.Contains(asString, ",") {
			return nil, xerrors.Errorf("invalid claim type. Got a csv string (%q), change this claim to return an array of strings instead.", asString)
		}
		return []string{asString}, nil
	}

	// Not sure what the user gave us.
	return nil, xerrors.Errorf("invalid claim type. Expected an array of strings, got: %T", claim)
}

// HttpError is a helper struct for returning errors from the IDP sync process.
// A regular error is not sufficient because many of these errors are surfaced
// to a user logging in, and the errors should be descriptive.
type HttpError struct {
	Code                 int
	Msg                  string
	Detail               string
	RenderStaticPage     bool
	RenderDetailMarkdown bool
}

func (e HttpError) Write(rw http.ResponseWriter, r *http.Request) {
	if e.RenderStaticPage {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       e.Code,
			HideStatus:   true,
			Title:        e.Msg,
			Description:  e.Detail,
			RetryEnabled: false,
			DashboardURL: "/login",

			RenderDescriptionMarkdown: e.RenderDetailMarkdown,
		})
		return
	}
	httpapi.Write(r.Context(), rw, e.Code, codersdk.Response{
		Message: e.Msg,
		Detail:  e.Detail,
	})
}

func (e HttpError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}

	return e.Msg
}
