package idpsync
import (
	"fmt"
	"errors"
	"context"
	"net/http"
	"regexp"
	"strings"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/site"
)
// IDPSync is an interface, so we can implement this as AGPL and as enterprise,
// and just swap the underlying implementation.
// IDPSync exists to contain all the logic for mapping a user's external IDP
// claims to the internal representation of a user in Coder.
// TODO: Move group + role sync into this interface.
type IDPSync interface {
	OrganizationSyncEntitled() bool
	OrganizationSyncSettings(ctx context.Context, db database.Store) (*OrganizationSyncSettings, error)
	UpdateOrganizationSyncSettings(ctx context.Context, db database.Store, settings OrganizationSyncSettings) error
	// OrganizationSyncEnabled returns true if all OIDC users are assigned
	// to organizations via org sync settings.
	// This is used to know when to disable manual org membership assignment.
	OrganizationSyncEnabled(ctx context.Context, db database.Store) bool
	// ParseOrganizationClaims takes claims from an OIDC provider, and returns the
	// organization sync params for assigning users into organizations.
	ParseOrganizationClaims(ctx context.Context, mergedClaims jwt.MapClaims) (OrganizationParams, *HTTPError)
	// SyncOrganizations assigns and removed users from organizations based on the
	// provided params.
	SyncOrganizations(ctx context.Context, tx database.Store, user database.User, params OrganizationParams) error
	GroupSyncEntitled() bool
	// ParseGroupClaims takes claims from an OIDC provider, and returns the params
	// for group syncing. Most of the logic happens in SyncGroups.
	ParseGroupClaims(ctx context.Context, mergedClaims jwt.MapClaims) (GroupParams, *HTTPError)
	// SyncGroups assigns and removes users from groups based on the provided params.
	SyncGroups(ctx context.Context, db database.Store, user database.User, params GroupParams) error
	// GroupSyncSettings is exposed for the API to implement CRUD operations
	// on the settings used by IDPSync. This entry is thread safe and can be
	// accessed concurrently. The settings are stored in the database.
	GroupSyncSettings(ctx context.Context, orgID uuid.UUID, db database.Store) (*GroupSyncSettings, error)
	UpdateGroupSyncSettings(ctx context.Context, orgID uuid.UUID, db database.Store, settings GroupSyncSettings) error
	// RoleSyncEntitled returns true if the deployment is entitled to role syncing.
	RoleSyncEntitled() bool
	// OrganizationRoleSyncEnabled returns true if the organization has role sync
	// enabled.
	OrganizationRoleSyncEnabled(ctx context.Context, db database.Store, org uuid.UUID) (bool, error)
	// SiteRoleSyncEnabled returns true if the deployment has role sync enabled
	// at the site level.
	SiteRoleSyncEnabled() bool
	// RoleSyncSettings is similar to GroupSyncSettings. See GroupSyncSettings for
	// rational.
	RoleSyncSettings(ctx context.Context, orgID uuid.UUID, db database.Store) (*RoleSyncSettings, error)
	UpdateRoleSyncSettings(ctx context.Context, orgID uuid.UUID, db database.Store, settings RoleSyncSettings) error
	// ParseRoleClaims takes claims from an OIDC provider, and returns the params
	// for role syncing. Most of the logic happens in SyncRoles.
	ParseRoleClaims(ctx context.Context, mergedClaims jwt.MapClaims) (RoleParams, *HTTPError)
	// SyncRoles assigns and removes users from roles based on the provided params.
	// Site & org roles are handled in this method.
	SyncRoles(ctx context.Context, db database.Store, user database.User, params RoleParams) error
}
// AGPLIDPSync implements the IDPSync interface
var _ IDPSync = AGPLIDPSync{}
// AGPLIDPSync is the configuration for syncing user information from an external
// IDP. All related code to syncing user information should be in this package.
type AGPLIDPSync struct {
	Logger  slog.Logger
	Manager *runtimeconfig.Manager
	SyncSettings
}
// DeploymentSyncSettings are static and are sourced from the deployment config.
type DeploymentSyncSettings struct {
	// OrganizationField selects the claim field to be used as the created user's
	// organizations. If the field is the empty string, then no organization updates
	// will ever come from the OIDC provider.
	OrganizationField string
	// OrganizationMapping controls how organizations returned by the OIDC provider get mapped
	OrganizationMapping map[string][]uuid.UUID
	// OrganizationAssignDefault will ensure all users that authenticate will be
	// placed into the default organization. This is mostly a hack to support
	// legacy deployments.
	OrganizationAssignDefault bool
	// GroupField at the deployment level is used for deployment level group claim
	// settings.
	GroupField string
	// GroupAllowList (if set) will restrict authentication to only users who
	// have at least one group in this list.
	// A map representation is used for easier lookup.
	GroupAllowList map[string]struct{}
	// Legacy deployment settings that only apply to the default org.
	Legacy DefaultOrgLegacySettings
	// SiteRoleField selects the claim field to be used as the created user's
	// roles. If the field is the empty string, then no site role updates
	// will ever come from the OIDC provider.
	SiteRoleField string
	// SiteRoleMapping controls how groups returned by the OIDC provider get mapped
	// to site roles within Coder.
	// map[oidcRoleName][]coderRoleName
	SiteRoleMapping map[string][]string
	// SiteDefaultRoles is the default set of site roles to assign to a user if role sync
	// is enabled.
	SiteDefaultRoles []string
}
type DefaultOrgLegacySettings struct {
	GroupField          string
	GroupMapping        map[string]string
	GroupFilter         *regexp.Regexp
	CreateMissingGroups bool
}
func FromDeploymentValues(dv *codersdk.DeploymentValues) DeploymentSyncSettings {
	if dv == nil {
		panic("Developer error: DeploymentValues should not be nil")
	}
	return DeploymentSyncSettings{
		OrganizationField:         dv.OIDC.OrganizationField.Value(),
		OrganizationMapping:       dv.OIDC.OrganizationMapping.Value,
		OrganizationAssignDefault: dv.OIDC.OrganizationAssignDefault.Value(),
		SiteRoleField:    dv.OIDC.UserRoleField.Value(),
		SiteRoleMapping:  dv.OIDC.UserRoleMapping.Value,
		SiteDefaultRoles: dv.OIDC.UserRolesDefault.Value(),
		// TODO: Separate group field for allow list from default org.
		// Right now you cannot disable group sync from the default org and
		// configure an allow list.
		GroupField:     dv.OIDC.GroupField.Value(),
		GroupAllowList: ConvertAllowList(dv.OIDC.GroupAllowList.Value()),
		Legacy: DefaultOrgLegacySettings{
			GroupField:          dv.OIDC.GroupField.Value(),
			GroupMapping:        dv.OIDC.GroupMapping.Value,
			GroupFilter:         dv.OIDC.GroupRegexFilter.Value(),
			CreateMissingGroups: dv.OIDC.GroupAutoCreate.Value(),
		},
	}
}
type SyncSettings struct {
	DeploymentSyncSettings
	Group        runtimeconfig.RuntimeEntry[*GroupSyncSettings]
	Role         runtimeconfig.RuntimeEntry[*RoleSyncSettings]
	Organization runtimeconfig.RuntimeEntry[*OrganizationSyncSettings]
}
func NewAGPLSync(logger slog.Logger, manager *runtimeconfig.Manager, settings DeploymentSyncSettings) *AGPLIDPSync {
	return &AGPLIDPSync{
		Logger:  logger.Named("idp-sync"),
		Manager: manager,
		SyncSettings: SyncSettings{
			DeploymentSyncSettings: settings,
			Group:                  runtimeconfig.MustNew[*GroupSyncSettings]("group-sync-settings"),
			Role:                   runtimeconfig.MustNew[*RoleSyncSettings]("role-sync-settings"),
			Organization:           runtimeconfig.MustNew[*OrganizationSyncSettings]("organization-sync-settings"),
		},
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
				return nil, fmt.Errorf("invalid claim type. Element %d expected a string, got: %T", i, item)
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
			return nil, fmt.Errorf("invalid claim type. Got a csv string (%q), change this claim to return an array of strings instead.", asString)
		}
		return []string{asString}, nil
	}
	// Not sure what the user gave us.
	return nil, fmt.Errorf("invalid claim type. Expected an array of strings, got: %T", claim)
}
// IsHTTPError handles us being inconsistent with returning errors as values or
// pointers.
func IsHTTPError(err error) *HTTPError {
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return &httpErr
	}
	var httpErrPtr *HTTPError
	if errors.As(err, &httpErrPtr) {
		return httpErrPtr
	}
	return nil
}
// HTTPError is a helper struct for returning errors from the IDP sync process.
// A regular error is not sufficient because many of these errors are surfaced
// to a user logging in, and the errors should be descriptive.
type HTTPError struct {
	Code                 int
	Msg                  string
	Detail               string
	RenderStaticPage     bool
	RenderDetailMarkdown bool
}
func (e HTTPError) Write(rw http.ResponseWriter, r *http.Request) {
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
func (e HTTPError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return e.Msg
}
