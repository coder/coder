package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// Me is used as a replacement for your own ID.
var Me = "me"

type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusDormant   UserStatus = "dormant"
	UserStatusSuspended UserStatus = "suspended"
)

type UsersRequest struct {
	Search string `json:"search,omitempty" typescript:"-"`
	// Filter users by status.
	Status UserStatus `json:"status,omitempty" typescript:"-"`
	// Filter users that have the given role.
	Role string `json:"role,omitempty" typescript:"-"`

	SearchQuery string `json:"q,omitempty"`
	Pagination
}

// MinimalUser is the minimal information needed to identify a user and show
// them on the UI.
type MinimalUser struct {
	ID        uuid.UUID `json:"id" validate:"required" table:"id" format:"uuid"`
	Username  string    `json:"username" validate:"required" table:"username,default_sort"`
	AvatarURL string    `json:"avatar_url" format:"uri"`
}

// ReducedUser omits role and organization information. Roles are deduced from
// the user's site and organization roles. This requires fetching the user's
// organizational memberships. Fetching that is more expensive, and not usually
// required by the frontend.
type ReducedUser struct {
	MinimalUser `table:"m,recursive_inline"`
	Name        string    `json:"name"`
	Email       string    `json:"email" validate:"required" table:"email" format:"email"`
	CreatedAt   time.Time `json:"created_at" validate:"required" table:"created at" format:"date-time"`
	LastSeenAt  time.Time `json:"last_seen_at" format:"date-time"`

	Status          UserStatus `json:"status" table:"status" enums:"active,suspended"`
	LoginType       LoginType  `json:"login_type"`
	ThemePreference string     `json:"theme_preference"`
}

// User represents a user in Coder.
type User struct {
	ReducedUser `table:"r,recursive_inline"`

	OrganizationIDs []uuid.UUID `json:"organization_ids" format:"uuid"`
	Roles           []SlimRole  `json:"roles"`
}

type GetUsersResponse struct {
	Users []User `json:"users"`
	Count int    `json:"count"`
}

// @typescript-ignore LicensorTrialRequest
type LicensorTrialRequest struct {
	DeploymentID string `json:"deployment_id"`
	Email        string `json:"email"`
	Source       string `json:"source"`

	// Personal details.
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	PhoneNumber string `json:"phone_number"`
	JobTitle    string `json:"job_title"`
	CompanyName string `json:"company_name"`
	Country     string `json:"country"`
	Developers  string `json:"developers"`
}

type CreateFirstUserRequest struct {
	Email     string                   `json:"email" validate:"required,email"`
	Username  string                   `json:"username" validate:"required,username"`
	Password  string                   `json:"password" validate:"required"`
	Trial     bool                     `json:"trial"`
	TrialInfo CreateFirstUserTrialInfo `json:"trial_info"`
}

type CreateFirstUserTrialInfo struct {
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	PhoneNumber string `json:"phone_number"`
	JobTitle    string `json:"job_title"`
	CompanyName string `json:"company_name"`
	Country     string `json:"country"`
	Developers  string `json:"developers"`
}

// CreateFirstUserResponse contains IDs for newly created user info.
type CreateFirstUserResponse struct {
	UserID         uuid.UUID `json:"user_id" format:"uuid"`
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid"`
}

type CreateUserRequest struct {
	Email    string `json:"email" validate:"required,email" format:"email"`
	Username string `json:"username" validate:"required,username"`
	Password string `json:"password"`
	// UserLoginType defaults to LoginTypePassword.
	UserLoginType LoginType `json:"login_type"`
	// DisableLogin sets the user's login type to 'none'. This prevents the user
	// from being able to use a password or any other authentication method to login.
	// Deprecated: Set UserLoginType=LoginTypeDisabled instead.
	DisableLogin   bool      `json:"disable_login"`
	OrganizationID uuid.UUID `json:"organization_id" validate:"" format:"uuid"`
}

type UpdateUserProfileRequest struct {
	Username string `json:"username" validate:"required,username"`
	Name     string `json:"name" validate:"user_real_name"`
}

type UpdateUserAppearanceSettingsRequest struct {
	ThemePreference string `json:"theme_preference" validate:"required"`
}

type UpdateUserPasswordRequest struct {
	OldPassword string `json:"old_password" validate:""`
	Password    string `json:"password" validate:"required"`
}

type UserQuietHoursScheduleResponse struct {
	RawSchedule string `json:"raw_schedule"`
	// UserSet is true if the user has set their own quiet hours schedule. If
	// false, the user is using the default schedule.
	UserSet bool `json:"user_set"`
	// UserCanSet is true if the user is allowed to set their own quiet hours
	// schedule. If false, the user cannot set a custom schedule and the default
	// schedule will always be used.
	UserCanSet bool `json:"user_can_set"`
	// Time is the time of day that the quiet hours window starts in the given
	// Timezone each day.
	Time     string `json:"time"`     // HH:mm (24-hour)
	Timezone string `json:"timezone"` // raw format from the cron expression, UTC if unspecified
	// Next is the next time that the quiet hours window will start.
	Next time.Time `json:"next" format:"date-time"`
}

type UpdateUserQuietHoursScheduleRequest struct {
	// Schedule is a cron expression that defines when the user's quiet hours
	// window is. Schedule must not be empty. For new users, the schedule is set
	// to 2am in their browser or computer's timezone. The schedule denotes the
	// beginning of a 4 hour window where the workspace is allowed to
	// automatically stop or restart due to maintenance or template schedule.
	//
	// The schedule must be daily with a single time, and should have a timezone
	// specified via a CRON_TZ prefix (otherwise UTC will be used).
	//
	// If the schedule is empty, the user will be updated to use the default
	// schedule.
	Schedule string `json:"schedule" validate:"required"`
}

type UpdateRoles struct {
	Roles []string `json:"roles" validate:""`
}

type UserRoles struct {
	Roles             []string               `json:"roles"`
	OrganizationRoles map[uuid.UUID][]string `json:"organization_roles"`
}

type ConvertLoginRequest struct {
	// ToType is the login type to convert to.
	ToType   LoginType `json:"to_type" validate:"required"`
	Password string    `json:"password" validate:"required"`
}

// LoginWithPasswordRequest enables callers to authenticate with email and password.
type LoginWithPasswordRequest struct {
	Email    string `json:"email" validate:"required,email" format:"email"`
	Password string `json:"password" validate:"required"`
}

// LoginWithPasswordResponse contains a session token for the newly authenticated user.
type LoginWithPasswordResponse struct {
	SessionToken string `json:"session_token" validate:"required"`
}

type OAuthConversionResponse struct {
	StateString string    `json:"state_string"`
	ExpiresAt   time.Time `json:"expires_at" format:"date-time"`
	ToType      LoginType `json:"to_type"`
	UserID      uuid.UUID `json:"user_id" format:"uuid"`
}

// AuthMethods contains authentication method information like whether they are enabled or not or custom text, etc.
type AuthMethods struct {
	TermsOfServiceURL string         `json:"terms_of_service_url,omitempty"`
	Password          AuthMethod     `json:"password"`
	Github            AuthMethod     `json:"github"`
	OIDC              OIDCAuthMethod `json:"oidc"`
}

type AuthMethod struct {
	Enabled bool `json:"enabled"`
}

type UserLoginType struct {
	LoginType LoginType `json:"login_type"`
}

type OIDCAuthMethod struct {
	AuthMethod
	SignInText string `json:"signInText"`
	IconURL    string `json:"iconUrl"`
}

type UserParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// UserAutofillParameters returns all recently used parameters for the given user.
func (c *Client) UserAutofillParameters(ctx context.Context, user string, templateID uuid.UUID) ([]UserParameter, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/autofill-parameters?template_id=%s", user, templateID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var params []UserParameter
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// HasFirstUser returns whether the first user has been created.
func (c *Client) HasFirstUser(ctx context.Context) (bool, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/users/first", nil)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		// ensure we are talking to coder and not
		// some other service that returns 404
		v := res.Header.Get(BuildVersionHeader)
		if v == "" {
			return false, xerrors.Errorf("missing build version header, not a coder instance")
		}

		return false, nil
	}
	if res.StatusCode != http.StatusOK {
		return false, ReadBodyAsError(res)
	}
	return true, nil
}

// CreateFirstUser attempts to create the first user on a Coder deployment.
// This initial user has superadmin privileges. If >0 users exist, this request will fail.
func (c *Client) CreateFirstUser(ctx context.Context, req CreateFirstUserRequest) (CreateFirstUserResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/users/first", req)
	if err != nil {
		return CreateFirstUserResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return CreateFirstUserResponse{}, ReadBodyAsError(res)
	}
	var resp CreateFirstUserResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// CreateUser creates a new user.
func (c *Client) CreateUser(ctx context.Context, req CreateUserRequest) (User, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/users", req)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return User{}, ReadBodyAsError(res)
	}
	var user User
	return user, json.NewDecoder(res.Body).Decode(&user)
}

// DeleteUser deletes a user.
func (c *Client) DeleteUser(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/users/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// UpdateUserProfile updates the username of a user.
func (c *Client) UpdateUserProfile(ctx context.Context, user string, req UpdateUserProfileRequest) (User, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/profile", user), req)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return User{}, ReadBodyAsError(res)
	}
	var resp User
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpdateUserStatus sets the user status to the given status
func (c *Client) UpdateUserStatus(ctx context.Context, user string, status UserStatus) (User, error) {
	path := fmt.Sprintf("/api/v2/users/%s/status/", user)
	switch status {
	case UserStatusActive:
		path += "activate"
	case UserStatusSuspended:
		path += "suspend"
	default:
		return User{}, xerrors.Errorf("status %q is not supported", status)
	}

	res, err := c.Request(ctx, http.MethodPut, path, nil)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return User{}, ReadBodyAsError(res)
	}

	var resp User
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpdateUserAppearanceSettings updates the appearance settings for a user.
func (c *Client) UpdateUserAppearanceSettings(ctx context.Context, user string, req UpdateUserAppearanceSettingsRequest) (User, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/appearance", user), req)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return User{}, ReadBodyAsError(res)
	}
	var resp User
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpdateUserPassword updates a user password.
// It calls PUT /users/{user}/password
func (c *Client) UpdateUserPassword(ctx context.Context, user string, req UpdateUserPasswordRequest) error {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/password", user), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// PostOrganizationMember adds a user to an organization
func (c *Client) PostOrganizationMember(ctx context.Context, organizationID uuid.UUID, user string) (OrganizationMember, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/members/%s", organizationID, user), nil)
	if err != nil {
		return OrganizationMember{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OrganizationMember{}, ReadBodyAsError(res)
	}
	var member OrganizationMember
	return member, json.NewDecoder(res.Body).Decode(&member)
}

// OrganizationMembers lists all members in an organization
func (c *Client) OrganizationMembers(ctx context.Context, organizationID uuid.UUID) ([]OrganizationMemberWithName, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/members/", organizationID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var members []OrganizationMemberWithName
	return members, json.NewDecoder(res.Body).Decode(&members)
}

// UpdateUserRoles grants the userID the specified roles.
// Include ALL roles the user has.
func (c *Client) UpdateUserRoles(ctx context.Context, user string, req UpdateRoles) (User, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/roles", user), req)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return User{}, ReadBodyAsError(res)
	}
	var resp User
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpdateOrganizationMemberRoles grants the userID the specified roles in an org.
// Include ALL roles the user has.
func (c *Client) UpdateOrganizationMemberRoles(ctx context.Context, organizationID uuid.UUID, user string, req UpdateRoles) (OrganizationMember, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/organizations/%s/members/%s/roles", organizationID, user), req)
	if err != nil {
		return OrganizationMember{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OrganizationMember{}, ReadBodyAsError(res)
	}
	var member OrganizationMember
	return member, json.NewDecoder(res.Body).Decode(&member)
}

// UserRoles returns all roles the user has
func (c *Client) UserRoles(ctx context.Context, user string) (UserRoles, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/roles", user), nil)
	if err != nil {
		return UserRoles{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserRoles{}, ReadBodyAsError(res)
	}
	var roles UserRoles
	return roles, json.NewDecoder(res.Body).Decode(&roles)
}

// LoginWithPassword creates a session token authenticating with an email and password.
// Call `SetSessionToken()` to apply the newly acquired token to the client.
func (c *Client) LoginWithPassword(ctx context.Context, req LoginWithPasswordRequest) (LoginWithPasswordResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/users/login", req)
	if err != nil {
		return LoginWithPasswordResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return LoginWithPasswordResponse{}, ReadBodyAsError(res)
	}
	var resp LoginWithPasswordResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return LoginWithPasswordResponse{}, err
	}
	return resp, nil
}

// ConvertLoginType will send a request to convert the user from password
// based authentication to oauth based. The response has the oauth state code
// to use in the oauth flow.
func (c *Client) ConvertLoginType(ctx context.Context, req ConvertLoginRequest) (OAuthConversionResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/users/me/convert-login", req)
	if err != nil {
		return OAuthConversionResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return OAuthConversionResponse{}, ReadBodyAsError(res)
	}
	var resp OAuthConversionResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return OAuthConversionResponse{}, err
	}
	return resp, nil
}

// Logout calls the /logout API
// Call `ClearSessionToken()` to clear the session token of the client.
func (c *Client) Logout(ctx context.Context) error {
	// Since `LoginWithPassword` doesn't actually set a SessionToken
	// (it requires a call to SetSessionToken), this is essentially a no-op
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/users/logout", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

// User returns a user for the ID/username provided.
func (c *Client) User(ctx context.Context, userIdent string) (User, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s", userIdent), nil)
	if err != nil {
		return User{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return User{}, ReadBodyAsError(res)
	}
	var user User
	return user, json.NewDecoder(res.Body).Decode(&user)
}

// UserQuietHoursSchedule returns the quiet hours settings for the user. This
// endpoint only exists in enterprise editions.
func (c *Client) UserQuietHoursSchedule(ctx context.Context, userIdent string) (UserQuietHoursScheduleResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/quiet-hours", userIdent), nil)
	if err != nil {
		return UserQuietHoursScheduleResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserQuietHoursScheduleResponse{}, ReadBodyAsError(res)
	}
	var resp UserQuietHoursScheduleResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpdateUserQuietHoursSchedule updates the quiet hours settings for the user.
// This endpoint only exists in enterprise editions.
func (c *Client) UpdateUserQuietHoursSchedule(ctx context.Context, userIdent string, req UpdateUserQuietHoursScheduleRequest) (UserQuietHoursScheduleResponse, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/users/%s/quiet-hours", userIdent), req)
	if err != nil {
		return UserQuietHoursScheduleResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserQuietHoursScheduleResponse{}, ReadBodyAsError(res)
	}
	var resp UserQuietHoursScheduleResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// Users returns all users according to the request parameters. If no parameters are set,
// the default behavior is to return all users in a single page.
func (c *Client) Users(ctx context.Context, req UsersRequest) (GetUsersResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/users", nil,
		req.Pagination.asRequestOption(),
		func(r *http.Request) {
			q := r.URL.Query()
			var params []string
			if req.Search != "" {
				params = append(params, req.Search)
			}
			if req.Status != "" {
				params = append(params, "status:"+string(req.Status))
			}
			if req.Role != "" {
				params = append(params, "role:"+req.Role)
			}
			if req.SearchQuery != "" {
				params = append(params, req.SearchQuery)
			}
			q.Set("q", strings.Join(params, " "))
			r.URL.RawQuery = q.Encode()
		},
	)
	if err != nil {
		return GetUsersResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GetUsersResponse{}, ReadBodyAsError(res)
	}

	var usersRes GetUsersResponse
	return usersRes, json.NewDecoder(res.Body).Decode(&usersRes)
}

// OrganizationsByUser returns all organizations the user is a member of.
func (c *Client) OrganizationsByUser(ctx context.Context, user string) ([]Organization, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/organizations", user), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode > http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var orgs []Organization
	return orgs, json.NewDecoder(res.Body).Decode(&orgs)
}

func (c *Client) OrganizationByUserAndName(ctx context.Context, user string, name string) (Organization, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/organizations/%s", user, name), nil)
	if err != nil {
		return Organization{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Organization{}, ReadBodyAsError(res)
	}
	var org Organization
	return org, json.NewDecoder(res.Body).Decode(&org)
}

// AuthMethods returns types of authentication available to the user.
func (c *Client) AuthMethods(ctx context.Context) (AuthMethods, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/users/authmethods", nil)
	if err != nil {
		return AuthMethods{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return AuthMethods{}, ReadBodyAsError(res)
	}

	var userAuth AuthMethods
	return userAuth, json.NewDecoder(res.Body).Decode(&userAuth)
}
