package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/util/ptr"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/render"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

type MergedClaimsSource string

var (
	MergedClaimsSourceNone        MergedClaimsSource = "none"
	MergedClaimsSourceUserInfo    MergedClaimsSource = "user_info"
	MergedClaimsSourceAccessToken MergedClaimsSource = "access_token"
)

const (
	userAuthLoggerName      = "userauth"
	OAuthConvertCookieValue = "coder_oauth_convert_jwt"
	mergeStateStringPrefix  = "convert-"
)

type OAuthConvertStateClaims struct {
	jwtutils.RegisteredClaims

	UserID        uuid.UUID          `json:"user_id"`
	State         string             `json:"state"`
	FromLoginType codersdk.LoginType `json:"from_login_type"`
	ToLoginType   codersdk.LoginType `json:"to_login_type"`
}

func (o *OAuthConvertStateClaims) Validate(e jwt.Expected) error {
	return o.RegisteredClaims.Validate(e)
}

// postConvertLoginType replies with an oauth state token capable of converting
// the user to an oauth user.
//
// @Summary Convert user from password to oauth authentication
// @ID convert-user-from-password-to-oauth-authentication
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Authorization
// @Param request body codersdk.ConvertLoginRequest true "Convert request"
// @Param user path string true "User ID, name, or me"
// @Success 201 {object} codersdk.OAuthConversionResponse
// @Router /users/{user}/convert-login [post]
func (api *API) postConvertLoginType(rw http.ResponseWriter, r *http.Request) {
	var (
		user              = httpmw.UserParam(r)
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditOAuthConvertState](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	aReq.Old = database.AuditOAuthConvertState{}
	defer commitAudit()

	var req codersdk.ConvertLoginRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	switch req.ToType {
	case codersdk.LoginTypeGithub, codersdk.LoginTypeOIDC:
		// Allowed!
	case codersdk.LoginTypeNone, codersdk.LoginTypePassword, codersdk.LoginTypeToken:
		// These login types are not allowed to be converted to at this time.
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Cannot convert to login type %q.", req.ToType),
		})
		return
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Unknown login type %q.", req.ToType),
		})
		return
	}

	// This handles the email/pass checking.
	user, _, ok := api.loginRequest(ctx, rw, codersdk.LoginWithPasswordRequest{
		Email:    user.Email,
		Password: req.Password,
	})
	if !ok {
		return
	}

	// Only support converting from password auth.
	if user.LoginType != database.LoginTypePassword {
		// This is checked in loginRequest, but checked again here in case that shared
		// function changes its checks. Just some defensive programming.
		// This login type is **required** to be password based to prevent
		// users from converting other login types to OIDC.
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "User account must have password based authentication.",
		})
		return
	}

	stateString, err := cryptorand.String(32)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error generating state string.",
			Detail:  err.Error(),
		})
		return
	}
	// The prefix is used to identify this state string as a conversion state
	// without needing to hit the database. The random string is the CSRF protection.
	stateString = fmt.Sprintf("%s%s", mergeStateStringPrefix, stateString)

	// This JWT is the signed payload to authorize the convert to oauth request.
	// When the user does the oauth flow, this jwt will be sent back to coderd.
	// The included information in this payload links it to a state string, so
	// this request is tied 1:1 with an oauth state.
	// This JWT also includes information to tie it 1:1 with a coder deployment
	// and user account. This is mainly to inform the user if they are accidentally
	// switching between coder deployments if the OIDC is misconfigured.
	// Eg: Developers with more than 1 deployment.
	now := time.Now()
	claims := &OAuthConvertStateClaims{
		RegisteredClaims: jwtutils.RegisteredClaims{
			Issuer:    api.DeploymentID,
			Subject:   stateString,
			Audience:  []string{user.ID.String()},
			Expiry:    jwt.NewNumericDate(now.Add(time.Minute * 5)),
			NotBefore: jwt.NewNumericDate(now.Add(time.Second * -1)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
		UserID:        user.ID,
		State:         stateString,
		FromLoginType: codersdk.LoginType(user.LoginType),
		ToLoginType:   req.ToType,
	}

	token, err := jwtutils.Sign(ctx, api.OIDCConvertKeyCache, claims)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error signing state jwt.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = database.AuditOAuthConvertState{
		CreatedAt:     claims.IssuedAt.Time(),
		ExpiresAt:     claims.Expiry.Time(),
		FromLoginType: database.LoginType(claims.FromLoginType),
		ToLoginType:   database.LoginType(claims.ToLoginType),
		UserID:        claims.UserID,
	}

	http.SetCookie(rw, &http.Cookie{
		Name:     OAuthConvertCookieValue,
		Path:     "/",
		Value:    token,
		Expires:  claims.Expiry.Time(),
		Secure:   api.SecureAuthCookie,
		HttpOnly: true,
		// Must be SameSite to work on the redirected auth flow from the
		// oauth provider.
		SameSite: http.SameSiteLaxMode,
	})
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.OAuthConversionResponse{
		StateString: stateString,
		ExpiresAt:   claims.Expiry.Time(),
		ToType:      claims.ToLoginType,
		UserID:      claims.UserID,
	})
}

// Requests a one-time passcode for a user.
//
// @Summary Request one-time passcode
// @ID request-one-time-passcode
// @Accept json
// @Tags Authorization
// @Param request body codersdk.RequestOneTimePasscodeRequest true "One-time passcode request"
// @Success 204
// @Router /users/otp/request [post]
func (api *API) postRequestOneTimePasscode(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		logger            = api.Logger.Named(userAuthLoggerName)
		aReq, commitAudit = audit.InitRequest[database.User](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionRequestPasswordReset,
		})
	)
	defer commitAudit()

	if api.DeploymentValues.DisablePasswordAuth {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Password authentication is disabled.",
		})
		return
	}

	var req codersdk.RequestOneTimePasscodeRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	defer func() {
		// We always send the same response. If we give a more detailed response
		// it would open us up to an enumeration attack.
		rw.WriteHeader(http.StatusNoContent)
	}()

	//nolint:gocritic // In order to request a one-time passcode, we need to get the user first - and can only do that in the system auth context.
	user, err := api.Database.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
		Email: req.Email,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Error(ctx, "unable to get user by email", slog.Error(err))
		return
	}
	// We continue if err == sql.ErrNoRows to help prevent a timing-based attack.
	aReq.Old = user
	aReq.UserID = user.ID

	passcode := uuid.New()
	passcodeExpiresAt := dbtime.Now().Add(api.OneTimePasscodeValidityPeriod)

	hashedPasscode, err := userpassword.Hash(passcode.String())
	if err != nil {
		logger.Error(ctx, "unable to hash passcode", slog.Error(err))
		return
	}

	//nolint:gocritic // We need the system auth context to be able to save the one-time passcode.
	err = api.Database.UpdateUserHashedOneTimePasscode(dbauthz.AsSystemRestricted(ctx), database.UpdateUserHashedOneTimePasscodeParams{
		ID:                       user.ID,
		HashedOneTimePasscode:    []byte(hashedPasscode),
		OneTimePasscodeExpiresAt: sql.NullTime{Time: passcodeExpiresAt, Valid: true},
	})
	if err != nil {
		logger.Error(ctx, "unable to set user hashed one-time passcode", slog.Error(err))
		return
	}

	auditUser := user
	auditUser.HashedOneTimePasscode = []byte(hashedPasscode)
	auditUser.OneTimePasscodeExpiresAt = sql.NullTime{Time: passcodeExpiresAt, Valid: true}
	aReq.New = auditUser

	if user.ID != uuid.Nil {
		// Send the one-time passcode to the user.
		err = api.notifyUserRequestedOneTimePasscode(ctx, user, passcode.String())
		if err != nil {
			logger.Error(ctx, "unable to notify user about one-time passcode request", slog.Error(err))
		}
	} else {
		logger.Warn(ctx, "password reset requested for account that does not exist", slog.F("email", req.Email))
	}
}

func (api *API) notifyUserRequestedOneTimePasscode(ctx context.Context, user database.User, passcode string) error {
	_, err := api.NotificationsEnqueuer.Enqueue(
		//nolint:gocritic // We need the notifier auth context to be able to send the user their one-time passcode.
		dbauthz.AsNotifier(ctx),
		user.ID,
		notifications.TemplateUserRequestedOneTimePasscode,
		map[string]string{"one_time_passcode": passcode},
		"change-password-with-one-time-passcode",
		user.ID,
	)
	if err != nil {
		return xerrors.Errorf("enqueue notification: %w", err)
	}

	return nil
}

// Change a users password with a one-time passcode.
//
// @Summary Change password with a one-time passcode
// @ID change-password-with-a-one-time-passcode
// @Accept json
// @Tags Authorization
// @Param request body codersdk.ChangePasswordWithOneTimePasscodeRequest true "Change password request"
// @Success 204
// @Router /users/otp/change-password [post]
func (api *API) postChangePasswordWithOneTimePasscode(rw http.ResponseWriter, r *http.Request) {
	var (
		err               error
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		logger            = api.Logger.Named(userAuthLoggerName)
		aReq, commitAudit = audit.InitRequest[database.User](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	if api.DeploymentValues.DisablePasswordAuth {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Password authentication is disabled.",
		})
		return
	}

	var req codersdk.ChangePasswordWithOneTimePasscodeRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if err := userpassword.Validate(req.Password); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid password.",
			Validations: []codersdk.ValidationError{
				{
					Field:  "password",
					Detail: err.Error(),
				},
			},
		})
		return
	}

	err = api.Database.InTx(func(tx database.Store) error {
		//nolint:gocritic // In order to change a user's password, we need to get the user first - and can only do that in the system auth context.
		user, err := tx.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
			Email: req.Email,
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			logger.Error(ctx, "unable to fetch user by email", slog.F("email", req.Email), slog.Error(err))
			return xerrors.Errorf("get user by email: %w", err)
		}
		// We continue if err == sql.ErrNoRows to help prevent a timing-based attack.
		aReq.Old = user
		aReq.UserID = user.ID

		equal, err := userpassword.Compare(string(user.HashedOneTimePasscode), req.OneTimePasscode)
		if err != nil {
			logger.Error(ctx, "unable to compare one-time passcode", slog.Error(err))
			return xerrors.Errorf("compare one-time passcode: %w", err)
		}

		now := dbtime.Now()
		if !equal || now.After(user.OneTimePasscodeExpiresAt.Time) {
			logger.Warn(ctx, "password reset attempted with invalid or expired one-time passcode", slog.F("email", req.Email))
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Incorrect email or one-time passcode.",
			})
			return nil
		}

		equal, err = userpassword.Compare(string(user.HashedPassword), req.Password)
		if err != nil {
			logger.Error(ctx, "unable to compare password", slog.Error(err))
			return xerrors.Errorf("compare password: %w", err)
		}

		if equal {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "New password cannot match old password.",
			})
			return nil
		}

		newHashedPassword, err := userpassword.Hash(req.Password)
		if err != nil {
			logger.Error(ctx, "unable to hash user's password", slog.Error(err))
			return xerrors.Errorf("hash user password: %w", err)
		}

		//nolint:gocritic // We need the system auth context to be able to update the user's password.
		err = tx.UpdateUserHashedPassword(dbauthz.AsSystemRestricted(ctx), database.UpdateUserHashedPasswordParams{
			ID:             user.ID,
			HashedPassword: []byte(newHashedPassword),
		})
		if err != nil {
			logger.Error(ctx, "unable to delete user's hashed password", slog.Error(err))
			return xerrors.Errorf("update user hashed password: %w", err)
		}

		//nolint:gocritic // We need the system auth context to be able to delete all API keys for the user.
		err = tx.DeleteAPIKeysByUserID(dbauthz.AsSystemRestricted(ctx), user.ID)
		if err != nil {
			logger.Error(ctx, "unable to delete user's api keys", slog.Error(err))
			return xerrors.Errorf("delete api keys for user: %w", err)
		}

		auditUser := user
		auditUser.HashedPassword = []byte(newHashedPassword)
		auditUser.OneTimePasscodeExpiresAt = sql.NullTime{}
		auditUser.HashedOneTimePasscode = nil
		aReq.New = auditUser

		rw.WriteHeader(http.StatusNoContent)

		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
			Detail:  err.Error(),
		})
		return
	}
}

// ValidateUserPassword validates the complexity of a user password and that it is secured enough.
//
// @Summary Validate user password
// @ID validate-user-password
// @Security CoderSessionToken
// @Produce json
// @Accept json
// @Tags Authorization
// @Param request body codersdk.ValidateUserPasswordRequest true "Validate user password request"
// @Success 200 {object} codersdk.ValidateUserPasswordResponse
// @Router /users/validate-password [post]
func (*API) validateUserPassword(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		valid   = true
		details = ""
	)

	var req codersdk.ValidateUserPasswordRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	err := userpassword.Validate(req.Password)
	if err != nil {
		valid = false
		details = err.Error()
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ValidateUserPasswordResponse{
		Valid:   valid,
		Details: details,
	})
}

// Authenticates the user with an email and password.
//
// @Summary Log in user
// @ID log-in-user
// @Accept json
// @Produce json
// @Tags Authorization
// @Param request body codersdk.LoginWithPasswordRequest true "Login request"
// @Success 201 {object} codersdk.LoginWithPasswordResponse
// @Router /users/login [post]
func (api *API) postLogin(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		logger            = api.Logger.Named(userAuthLoggerName)
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionLogin,
		})
	)
	aReq.Old = database.APIKey{}
	defer commitAudit()

	var loginWithPassword codersdk.LoginWithPasswordRequest
	if !httpapi.Read(ctx, rw, r, &loginWithPassword) {
		return
	}

	user, actor, ok := api.loginRequest(ctx, rw, loginWithPassword)
	// 'user.ID' will be empty, or will be an actual value. Either is correct
	// here.
	aReq.UserID = user.ID
	if !ok {
		// user failed to login
		return
	}

	//nolint:gocritic // Creating the API key as the user instead of as system.
	cookie, key, err := api.createAPIKey(dbauthz.As(ctx, actor), apikey.CreateParams{
		UserID:          user.ID,
		LoginType:       database.LoginTypePassword,
		RemoteAddr:      r.RemoteAddr,
		DefaultLifetime: api.DeploymentValues.Sessions.DefaultDuration.Value(),
	})
	if err != nil {
		logger.Error(ctx, "unable to create API key", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = *key

	http.SetCookie(rw, cookie)

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.LoginWithPasswordResponse{
		SessionToken: cookie.Value,
	})
}

// loginRequest will process a LoginWithPasswordRequest and return the user if
// the credentials are correct. If 'false' is returned, the authentication failed
// and the appropriate error will be written to the ResponseWriter.
//
// The user struct is always returned, even if authentication failed. This is
// to support knowing what user attempted to login.
func (api *API) loginRequest(ctx context.Context, rw http.ResponseWriter, req codersdk.LoginWithPasswordRequest) (database.User, rbac.Subject, bool) {
	logger := api.Logger.Named(userAuthLoggerName)

	//nolint:gocritic // In order to login, we need to get the user first!
	user, err := api.Database.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
		Email: req.Email,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		logger.Error(ctx, "unable to fetch user by email", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return user, rbac.Subject{}, false
	}

	// If the user doesn't exist, it will be a default struct.
	equal, err := userpassword.Compare(string(user.HashedPassword), req.Password)
	if err != nil {
		logger.Error(ctx, "unable to compare passwords", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return user, rbac.Subject{}, false
	}

	if !equal {
		// This message is the same as above to remove ease in detecting whether
		// users are registered or not. Attackers still could with a timing attack.
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Incorrect email or password.",
		})
		return user, rbac.Subject{}, false
	}

	// If password authentication is disabled and the user does not have the
	// owner role, block the request.
	if api.DeploymentValues.DisablePasswordAuth {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Password authentication is disabled.",
		})
		return user, rbac.Subject{}, false
	}

	if user.LoginType != database.LoginTypePassword {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("Incorrect login type, attempting to use %q but user is of login type %q", database.LoginTypePassword, user.LoginType),
		})
		return user, rbac.Subject{}, false
	}

	user, err = ActivateDormantUser(api.Logger, &api.Auditor, api.Database)(ctx, user)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
			Detail:  err.Error(),
		})
		return user, rbac.Subject{}, false
	}

	subject, userStatus, err := httpmw.UserRBACSubject(ctx, api.Database, user.ID, rbac.ScopeAll)
	if err != nil {
		logger.Error(ctx, "unable to fetch authorization user roles", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return user, rbac.Subject{}, false
	}

	// If the user logged into a suspended account, reject the login request.
	if userStatus != database.UserStatusActive {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: fmt.Sprintf("Your account is %s. Contact an admin to reactivate your account.", userStatus),
		})
		return user, rbac.Subject{}, false
	}

	return user, subject, true
}

func ActivateDormantUser(logger slog.Logger, auditor *atomic.Pointer[audit.Auditor], db database.Store) func(ctx context.Context, user database.User) (database.User, error) {
	return func(ctx context.Context, user database.User) (database.User, error) {
		if user.ID == uuid.Nil || user.Status != database.UserStatusDormant {
			return user, nil
		}

		//nolint:gocritic // System needs to update status of the user account (dormant -> active).
		newUser, err := db.UpdateUserStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateUserStatusParams{
			ID:        user.ID,
			Status:    database.UserStatusActive,
			UpdatedAt: dbtime.Now(),
		})
		if err != nil {
			logger.Error(ctx, "unable to update user status to active", slog.Error(err))
			return user, xerrors.Errorf("update user status: %w", err)
		}

		oldAuditUser := user
		newAuditUser := user
		newAuditUser.Status = database.UserStatusActive

		audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.User]{
			Audit:            *auditor.Load(),
			Log:              logger,
			UserID:           user.ID,
			Action:           database.AuditActionWrite,
			Old:              oldAuditUser,
			New:              newAuditUser,
			Status:           http.StatusOK,
			AdditionalFields: audit.BackgroundTaskFieldsBytes(ctx, logger, audit.BackgroundSubsystemDormancy),
		})

		return newUser, nil
	}
}

// Clear the user's session cookie.
//
// @Summary Log out user
// @ID log-out-user
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Success 200 {object} codersdk.Response
// @Router /users/logout [post]
func (api *API) postLogout(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionLogout,
		})
	)
	defer commitAudit()

	// Get a blank token cookie.
	cookie := &http.Cookie{
		// MaxAge < 0 means to delete the cookie now.
		MaxAge: -1,
		Name:   codersdk.SessionTokenCookie,
		Path:   "/",
	}
	http.SetCookie(rw, cookie)

	// Delete the session token from database.
	apiKey := httpmw.APIKey(r)
	aReq.Old = apiKey

	logger := api.Logger.Named(userAuthLoggerName)

	err := api.Database.DeleteAPIKeyByID(ctx, apiKey.ID)
	if err != nil {
		logger.Error(ctx, "unable to delete API key", slog.F("api_key", apiKey.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting API key.",
			Detail:  err.Error(),
		})
		return
	}

	// Invalidate all subdomain app tokens. This saves us from having to
	// track which app tokens are associated which this browser session and
	// doesn't inconvenience the user as they'll just get redirected if they try
	// to access the app again.
	err = api.Database.DeleteApplicationConnectAPIKeysByUserID(ctx, apiKey.UserID)
	if err != nil {
		logger.Error(ctx, "unable to invalidate subdomain app tokens", slog.F("user_id", apiKey.UserID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting app tokens.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = database.APIKey{}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Logged out!",
	})
}

// GithubOAuth2Team represents a team scoped to an organization.
type GithubOAuth2Team struct {
	Organization string
	Slug         string
}

// GithubOAuth2Provider exposes required functions for the Github authentication flow.
type GithubOAuth2Config struct {
	promoauth.OAuth2Config
	AuthenticatedUser           func(ctx context.Context, client *http.Client) (*github.User, error)
	ListEmails                  func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error)
	ListOrganizationMemberships func(ctx context.Context, client *http.Client) ([]*github.Membership, error)
	TeamMembership              func(ctx context.Context, client *http.Client, org, team, username string) (*github.Membership, error)

	DeviceFlowEnabled  bool
	ExchangeDeviceCode func(ctx context.Context, deviceCode string) (*oauth2.Token, error)
	AuthorizeDevice    func(ctx context.Context) (*codersdk.ExternalAuthDevice, error)

	AllowSignups       bool
	AllowEveryone      bool
	AllowOrganizations []string
	AllowTeams         []GithubOAuth2Team

	DefaultProviderConfigured bool
}

func (c *GithubOAuth2Config) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	if !c.DeviceFlowEnabled {
		return c.OAuth2Config.Exchange(ctx, code, opts...)
	}
	return c.ExchangeDeviceCode(ctx, code)
}

func (c *GithubOAuth2Config) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	if !c.DeviceFlowEnabled {
		return c.OAuth2Config.AuthCodeURL(state, opts...)
	}
	// This is an absolute path in the Coder app. The device flow is orchestrated
	// by the Coder frontend, so we need to redirect the user to the device flow page.
	return "/login/device?state=" + state
}

// @Summary Get authentication methods
// @ID get-authentication-methods
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Success 200 {object} codersdk.AuthMethods
// @Router /users/authmethods [get]
func (api *API) userAuthMethods(rw http.ResponseWriter, r *http.Request) {
	var signInText string
	var iconURL string

	if api.OIDCConfig != nil {
		signInText = api.OIDCConfig.SignInText
	}
	if api.OIDCConfig != nil {
		iconURL = api.OIDCConfig.IconURL
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.AuthMethods{
		TermsOfServiceURL: api.DeploymentValues.TermsOfServiceURL.Value(),
		Password: codersdk.AuthMethod{
			Enabled: !api.DeploymentValues.DisablePasswordAuth.Value(),
		},
		Github: codersdk.GithubAuthMethod{
			Enabled:                   api.GithubOAuth2Config != nil,
			DefaultProviderConfigured: api.GithubOAuth2Config != nil && api.GithubOAuth2Config.DefaultProviderConfigured,
		},
		OIDC: codersdk.OIDCAuthMethod{
			AuthMethod: codersdk.AuthMethod{Enabled: api.OIDCConfig != nil},
			SignInText: signInText,
			IconURL:    iconURL,
		},
	})
}

// @Summary Get Github device auth.
// @ID get-github-device-auth
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Success 200 {object} codersdk.ExternalAuthDevice
// @Router /users/oauth2/github/device [get]
func (api *API) userOAuth2GithubDevice(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionLogin,
		})
	)
	aReq.Old = database.APIKey{}
	defer commitAudit()

	if api.GithubOAuth2Config == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Github OAuth2 is not enabled.",
		})
		return
	}

	if !api.GithubOAuth2Config.DeviceFlowEnabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Device flow is not enabled for Github OAuth2.",
		})
		return
	}

	deviceAuth, err := api.GithubOAuth2Config.AuthorizeDevice(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to authorize device.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, deviceAuth)
}

// @Summary OAuth 2.0 GitHub Callback
// @ID oauth-20-github-callback
// @Security CoderSessionToken
// @Tags Users
// @Success 307
// @Router /users/oauth2/github/callback [get]
func (api *API) userOAuth2Github(rw http.ResponseWriter, r *http.Request) {
	var (
		// userOAuth2Github is a system function.
		//nolint:gocritic
		ctx               = dbauthz.AsSystemRestricted(r.Context())
		state             = httpmw.OAuth2(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionLogin,
		})
	)
	aReq.Old = database.APIKey{}
	defer commitAudit()

	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(state.Token))

	logger := api.Logger.Named(userAuthLoggerName)

	var selectedMemberships []*github.Membership
	var organizationNames []string
	redirect := state.Redirect
	if !api.GithubOAuth2Config.AllowEveryone {
		memberships, err := api.GithubOAuth2Config.ListOrganizationMemberships(ctx, oauthClient)
		if err != nil {
			logger.Error(ctx, "unable to list organization members", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching authenticated Github user organizations.",
				Detail:  err.Error(),
			})
			return
		}

		for _, membership := range memberships {
			if membership.GetState() != "active" {
				continue
			}
			for _, allowed := range api.GithubOAuth2Config.AllowOrganizations {
				if *membership.Organization.Login != allowed {
					continue
				}
				selectedMemberships = append(selectedMemberships, membership)
				organizationNames = append(organizationNames, membership.Organization.GetLogin())
				break
			}
		}
		if len(selectedMemberships) == 0 {
			status := http.StatusUnauthorized
			msg := "You aren't a member of the authorized Github organizations!"
			if api.GithubOAuth2Config.DeviceFlowEnabled {
				// In the device flow, the error is rendered client-side.
				httpapi.Write(ctx, rw, status, codersdk.Response{
					Message: "Unauthorized",
					Detail:  msg,
				})
			} else {
				httpmw.CustomRedirectToLogin(rw, r, redirect, msg, status)
			}
			return
		}
	}

	ghUser, err := api.GithubOAuth2Config.AuthenticatedUser(ctx, oauthClient)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to fetch authenticated user", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching authenticated Github user.",
			Detail:  err.Error(),
		})
		return
	}

	// The default if no teams are specified is to allow all.
	if !api.GithubOAuth2Config.AllowEveryone && len(api.GithubOAuth2Config.AllowTeams) > 0 {
		var allowedTeam *github.Membership
		for _, allowTeam := range api.GithubOAuth2Config.AllowTeams {
			if allowedTeam != nil {
				break
			}
			for _, selectedMembership := range selectedMemberships {
				if allowTeam.Organization != *selectedMembership.Organization.Login {
					// This needs to continue because multiple organizations
					// could exist in the allow/team listings.
					continue
				}

				allowedTeam, err = api.GithubOAuth2Config.TeamMembership(ctx, oauthClient, allowTeam.Organization, allowTeam.Slug, *ghUser.Login)
				// The calling user may not have permission to the requested team!
				if err != nil {
					continue
				}
			}
		}
		if allowedTeam == nil {
			msg := fmt.Sprintf("You aren't a member of an authorized team in the %v Github organization(s)!", organizationNames)
			status := http.StatusUnauthorized
			if api.GithubOAuth2Config.DeviceFlowEnabled {
				// In the device flow, the error is rendered client-side.
				httpapi.Write(ctx, rw, status, codersdk.Response{
					Message: "Unauthorized",
					Detail:  msg,
				})
			} else {
				httpmw.CustomRedirectToLogin(rw, r, redirect, msg, status)
			}
			return
		}
	}

	emails, err := api.GithubOAuth2Config.ListEmails(ctx, oauthClient)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to list emails", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching personal Github user.",
			Detail:  err.Error(),
		})
		return
	}

	var verifiedEmail *github.UserEmail
	for _, email := range emails {
		if email.GetVerified() && email.GetPrimary() {
			verifiedEmail = email
			break
		}
	}

	if verifiedEmail == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Your primary email must be verified on GitHub!",
		})
		return
	}

	ghName := ghUser.GetName()
	normName := codersdk.NormalizeRealUsername(ghName)

	// If we have a nil GitHub ID, that is a big problem. That would mean we link
	// this user and all other users with this bug to the same uuid.
	// We should instead throw an error. This should never occur in production.
	//
	// Verified that the lowest ID on GitHub is "1", so 0 should never occur.
	if ghUser.GetID() == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "The GitHub user ID is missing, this should never happen. Please report this error.",
			// If this happens, the User could either be:
			//  - Empty, in which case all these fields would also be empty.
			//  - Not a user, in which case the "Type" would be something other than "User"
			Detail: fmt.Sprintf("Other user fields: name=%q, email=%q, type=%q",
				ghUser.GetName(),
				ghUser.GetEmail(),
				ghUser.GetType(),
			),
		})
		return
	}
	user, link, err := findLinkedUser(ctx, api.Database, githubLinkedID(ghUser), verifiedEmail.GetEmail())
	if err != nil {
		logger.Error(ctx, "oauth2: unable to find linked user", slog.F("gh_user", ghUser.Name), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to find linked user.",
			Detail:  err.Error(),
		})
		return
	}

	// If a new user is authenticating for the first time
	// the audit action is 'register', not 'login'
	if user.ID == uuid.Nil {
		aReq.Action = database.AuditActionRegister
	}
	// See: https://github.com/coder/coder/discussions/13340
	// In GitHub Enterprise, admins are permitted to have `_`
	// in their usernames. This is janky, but much better
	// than changing the username format globally.
	username := ghUser.GetLogin()
	if strings.Contains(username, "_") {
		api.Logger.Warn(ctx, "login associates a github username that contains underscores. underscores are not permitted in usernames, replacing with `-`", slog.F("username", username))
		username = strings.ReplaceAll(username, "_", "-")
	}
	params := (&oauthLoginParams{
		User:         user,
		Link:         link,
		State:        state,
		LinkedID:     githubLinkedID(ghUser),
		LoginType:    database.LoginTypeGithub,
		AllowSignups: api.GithubOAuth2Config.AllowSignups,
		Email:        verifiedEmail.GetEmail(),
		Username:     username,
		AvatarURL:    ghUser.GetAvatarURL(),
		Name:         normName,
		UserClaims:   database.UserLinkClaims{},
		GroupSync: idpsync.GroupParams{
			SyncEntitled: false,
		},
		OrganizationSync: idpsync.OrganizationParams{
			SyncEntitled: false,
		},
	}).SetInitAuditRequest(func(params *audit.RequestParams) (*audit.Request[database.User], func()) {
		return audit.InitRequest[database.User](rw, params)
	})
	cookies, user, key, err := api.oauthLogin(r, params)
	defer params.CommitAuditLogs()
	if err != nil {
		if httpErr := idpsync.IsHTTPError(err); httpErr != nil {
			// In the device flow, the error page is rendered client-side.
			if api.GithubOAuth2Config.DeviceFlowEnabled && httpErr.RenderStaticPage {
				httpErr.RenderStaticPage = false
			}
			httpErr.Write(rw, r)
			return
		}
		logger.Error(ctx, "oauth2: login failed", slog.F("user", user.Username), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to process OAuth login.",
			Detail:  err.Error(),
		})
		return
	}
	// If the user is logging in with github.com we update their associated
	// GitHub user ID to the new one.
	// We use AuthCodeURL from the OAuth2Config field instead of the one on
	// GithubOAuth2Config because when device flow is configured, AuthCodeURL
	// is overridden and returns a value that doesn't pass the URL check.
	if externalauth.IsGithubDotComURL(api.GithubOAuth2Config.OAuth2Config.AuthCodeURL("")) && user.GithubComUserID.Int64 != ghUser.GetID() {
		err = api.Database.UpdateUserGithubComUserID(ctx, database.UpdateUserGithubComUserIDParams{
			ID: user.ID,
			GithubComUserID: sql.NullInt64{
				Int64: ghUser.GetID(),
				Valid: true,
			},
		})
		if err != nil {
			logger.Error(ctx, "oauth2: unable to update user github id", slog.F("user", user.Username), slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update user GitHub ID.",
				Detail:  err.Error(),
			})
			return
		}
	}
	aReq.New = key
	aReq.UserID = key.UserID

	for _, cookie := range cookies {
		http.SetCookie(rw, cookie)
	}

	redirect = uriFromURL(redirect)
	if api.GithubOAuth2Config.DeviceFlowEnabled {
		// In the device flow, the redirect is handled client-side.
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.OAuth2DeviceFlowCallbackResponse{
			RedirectURL: redirect,
		})
	} else {
		http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
	}
}

type OIDCConfig struct {
	promoauth.OAuth2Config

	Provider *oidc.Provider
	Verifier *oidc.IDTokenVerifier
	// EmailDomains are the domains to enforce when a user authenticates.
	EmailDomain  []string
	AllowSignups bool
	// IgnoreEmailVerified allows ignoring the email_verified claim
	// from an upstream OIDC provider. See #5065 for context.
	IgnoreEmailVerified bool
	// UsernameField selects the claim field to be used as the created user's
	// username.
	UsernameField string
	// EmailField selects the claim field to be used as the created user's
	// email.
	EmailField string
	// NameField selects the claim field to be used as the created user's
	// full / given name.
	NameField string
	// AuthURLParams are additional parameters to be passed to the OIDC provider
	// when requesting an access token.
	AuthURLParams map[string]string
	// SecondaryClaims indicates where to source additional claim information from.
	// The standard is either 'MergedClaimsSourceNone' or 'MergedClaimsSourceUserInfo'.
	//
	// The OIDC compliant way is to use the userinfo endpoint. This option
	// is useful when the userinfo endpoint does not exist or causes undesirable
	// behavior.
	SecondaryClaims MergedClaimsSource
	// SignInText is the text to display on the OIDC login button
	SignInText string
	// IconURL points to the URL of an icon to display on the OIDC login button
	IconURL string
	// SignupsDisabledText is the text do display on the static error page.
	SignupsDisabledText string
}

// @Summary OpenID Connect Callback
// @ID openid-connect-callback
// @Security CoderSessionToken
// @Tags Users
// @Success 307
// @Router /users/oidc/callback [get]
func (api *API) userOIDC(rw http.ResponseWriter, r *http.Request) {
	var (
		// userOIDC is a system function.
		//nolint:gocritic
		ctx               = dbauthz.AsSystemRestricted(r.Context())
		state             = httpmw.OAuth2(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionLogin,
		})
	)
	aReq.Old = database.APIKey{}
	defer commitAudit()

	// See the example here: https://github.com/coreos/go-oidc
	rawIDToken, ok := state.Token.Extra("id_token").(string)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "id_token not found in response payload. Ensure your OIDC callback is configured correctly!",
		})
		return
	}

	idToken, err := api.OIDCConfig.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to verify OIDC token.",
			Detail:  err.Error(),
		})
		return
	}

	logger := api.Logger.Named(userAuthLoggerName)

	// "email_verified" is an optional claim that changes the behavior
	// of our OIDC handler, so each property must be pulled manually out
	// of the claim mapping.
	idtokenClaims := map[string]interface{}{}
	err = idToken.Claims(&idtokenClaims)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to extract OIDC claims", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to extract OIDC claims.",
			Detail:  err.Error(),
		})
		return
	}

	if idToken.Subject == "" {
		logger.Error(ctx, "oauth2: missing 'sub' claim field in OIDC token",
			slog.F("source", "id_token"),
			slog.F("claim_fields", claimFields(idtokenClaims)),
			slog.F("blank", blankFields(idtokenClaims)),
		)
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "OIDC token missing 'sub' claim field or 'sub' claim field is empty.",
			Detail: "'sub' claim field is required to be unique for all users by a given issue, " +
				"an empty field is invalid and this authentication attempt is rejected.",
		})
		return
	}

	logger.Debug(ctx, "got oidc claims",
		slog.F("source", "id_token"),
		slog.F("claim_fields", claimFields(idtokenClaims)),
		slog.F("blank", blankFields(idtokenClaims)),
	)

	// Not all claims are necessarily embedded in the `id_token`.
	// In GitLab, the username is left empty and must be fetched in UserInfo.
	//
	// The OIDC specification says claims can be in either place, so we fetch
	// user info if required and merge the two claim sets to be sure we have
	// all of the correct data.
	//
	// Some providers (e.g. ADFS) do not support custom OIDC claims in the
	// UserInfo endpoint, so we allow users to disable it and only rely on the
	// ID token.
	//
	// If user info is skipped, the idtokenClaims are the claims.
	mergedClaims := idtokenClaims
	supplementaryClaims := make(map[string]interface{})
	switch api.OIDCConfig.SecondaryClaims {
	case MergedClaimsSourceUserInfo:
		supplementaryClaims, ok = api.userInfoClaims(ctx, rw, state, logger)
		if !ok {
			return
		}

		// The precedence ordering is userInfoClaims > idTokenClaims.
		// Note: Unsure why exactly this is the case. idTokenClaims feels more
		// important?
		mergedClaims = mergeClaims(idtokenClaims, supplementaryClaims)
	case MergedClaimsSourceAccessToken:
		supplementaryClaims, ok = api.accessTokenClaims(ctx, rw, state, logger)
		if !ok {
			return
		}
		// idTokenClaims take priority over accessTokenClaims. The order should
		// not matter. It is just safer to assume idTokenClaims is the truth,
		// and accessTokenClaims are supplemental.
		mergedClaims = mergeClaims(supplementaryClaims, idtokenClaims)
	case MergedClaimsSourceNone:
		// noop, keep the userInfoClaims empty
	default:
		// This should never happen and is a developer error
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Invalid source for secondary user claims.",
			Detail:  fmt.Sprintf("invalid source: %q", api.OIDCConfig.SecondaryClaims),
		})
		return // Invalid MergedClaimsSource
	}

	usernameRaw, ok := mergedClaims[api.OIDCConfig.UsernameField]
	var username string
	if ok {
		username, _ = usernameRaw.(string)
	}

	emailRaw, ok := mergedClaims[api.OIDCConfig.EmailField]
	if !ok {
		// Email is an optional claim in OIDC and
		// instead the email is frequently sent in
		// "preferred_username". See:
		// https://github.com/coder/coder/issues/4472
		_, err = mail.ParseAddress(username)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "No email found in OIDC payload!",
			})
			return
		}
		emailRaw = username
	}

	email, ok := emailRaw.(string)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Email in OIDC payload isn't a string. Got: %t", emailRaw),
		})
		return
	}

	verifiedRaw, ok := mergedClaims["email_verified"]
	if ok {
		verified, ok := verifiedRaw.(bool)
		if ok && !verified {
			if !api.OIDCConfig.IgnoreEmailVerified {
				httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
					Message: fmt.Sprintf("Verify the %q email address on your OIDC provider to authenticate!", email),
				})
				return
			}
			logger.Warn(ctx, "allowing unverified oidc email %q")
		}
	}

	// The username is a required property in Coder. We make a best-effort
	// attempt at using what the claims provide, but if that fails we will
	// generate a random username.
	usernameValid := codersdk.NameValid(username)
	if usernameValid != nil {
		// If no username is provided, we can default to use the email address.
		// This will be converted in the from function below, so it's safe
		// to keep the domain.
		if username == "" {
			username = email
		}
		username = codersdk.UsernameFrom(username)
	}

	if len(api.OIDCConfig.EmailDomain) > 0 {
		ok = false
		emailSp := strings.Split(email, "@")
		if len(emailSp) == 1 {
			httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
				Message: fmt.Sprintf("Your email %q is not in domains %q!", email, api.OIDCConfig.EmailDomain),
			})
			return
		}
		userEmailDomain := emailSp[len(emailSp)-1]
		for _, domain := range api.OIDCConfig.EmailDomain {
			// Folks sometimes enter EmailDomain with a leading '@'.
			domain = strings.TrimPrefix(domain, "@")
			if strings.EqualFold(userEmailDomain, domain) {
				ok = true
				break
			}
		}
		if !ok {
			httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
				Message: fmt.Sprintf("Your email %q is not in domains %q!", email, api.OIDCConfig.EmailDomain),
			})
			return
		}
	}

	// The 'name' is an optional property in Coder. If not specified,
	// it will be left blank.
	var name string
	nameRaw, ok := mergedClaims[api.OIDCConfig.NameField]
	if ok {
		name, _ = nameRaw.(string)
		name = codersdk.NormalizeRealUsername(name)
	}

	var picture string
	pictureRaw, ok := mergedClaims["picture"]
	if ok {
		picture, _ = pictureRaw.(string)
	}

	ctx = slog.With(ctx, slog.F("email", email), slog.F("username", username), slog.F("name", name))

	user, link, err := findLinkedUser(ctx, api.Database, oidcLinkedID(idToken), email)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to find linked user", slog.F("email", email), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to find linked user.",
			Detail:  err.Error(),
		})
		return
	}

	orgSync, orgSyncErr := api.IDPSync.ParseOrganizationClaims(ctx, mergedClaims)
	if orgSyncErr != nil {
		orgSyncErr.Write(rw, r)
		return
	}

	groupSync, groupSyncErr := api.IDPSync.ParseGroupClaims(ctx, mergedClaims)
	if groupSyncErr != nil {
		groupSyncErr.Write(rw, r)
		return
	}

	roleSync, roleSyncErr := api.IDPSync.ParseRoleClaims(ctx, mergedClaims)
	if roleSyncErr != nil {
		roleSyncErr.Write(rw, r)
		return
	}

	// If a new user is authenticating for the first time
	// the audit action is 'register', not 'login'
	if user.ID == uuid.Nil {
		aReq.Action = database.AuditActionRegister
	}

	params := (&oauthLoginParams{
		User:             user,
		Link:             link,
		State:            state,
		LinkedID:         oidcLinkedID(idToken),
		LoginType:        database.LoginTypeOIDC,
		AllowSignups:     api.OIDCConfig.AllowSignups,
		Email:            email,
		Username:         username,
		Name:             name,
		AvatarURL:        picture,
		OrganizationSync: orgSync,
		GroupSync:        groupSync,
		RoleSync:         roleSync,
		UserClaims: database.UserLinkClaims{
			IDTokenClaims:  idtokenClaims,
			UserInfoClaims: supplementaryClaims,
			MergedClaims:   mergedClaims,
		},
	}).SetInitAuditRequest(func(params *audit.RequestParams) (*audit.Request[database.User], func()) {
		return audit.InitRequest[database.User](rw, params)
	})
	cookies, user, key, err := api.oauthLogin(r, params)
	defer params.CommitAuditLogs()
	if err != nil {
		if hErr := idpsync.IsHTTPError(err); hErr != nil {
			hErr.Write(rw, r)
			return
		}
		logger.Error(ctx, "oauth2: login failed", slog.F("user", user.Username), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to process OAuth login.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = key
	aReq.UserID = key.UserID

	for i := range cookies {
		http.SetCookie(rw, cookies[i])
	}

	redirect := state.Redirect
	// Strip the host if it exists on the URL to prevent
	// any nefarious redirects.
	redirect = uriFromURL(redirect)
	http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
}

func (api *API) accessTokenClaims(ctx context.Context, rw http.ResponseWriter, state httpmw.OAuth2State, logger slog.Logger) (accessTokenClaims map[string]interface{}, ok bool) {
	// Assume the access token is a jwt, and signed by the provider.
	accessToken, err := api.OIDCConfig.Verifier.Verify(ctx, state.Token.AccessToken)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to verify access token as secondary claims source", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to verify access token.",
			Detail:  fmt.Sprintf("sourcing secondary claims from access token: %s", err.Error()),
		})
		return nil, false
	}

	rawClaims := make(map[string]any)
	err = accessToken.Claims(&rawClaims)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to unmarshal access token claims", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to unmarshal access token claims.",
			Detail:  err.Error(),
		})
		return nil, false
	}

	return rawClaims, true
}

func (api *API) userInfoClaims(ctx context.Context, rw http.ResponseWriter, state httpmw.OAuth2State, logger slog.Logger) (userInfoClaims map[string]interface{}, ok bool) {
	userInfoClaims = make(map[string]interface{})
	userInfo, err := api.OIDCConfig.Provider.UserInfo(ctx, oauth2.StaticTokenSource(state.Token))
	switch {
	case err == nil:
		err = userInfo.Claims(&userInfoClaims)
		if err != nil {
			logger.Error(ctx, "oauth2: unable to unmarshal user info claims", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to unmarshal user info claims.",
				Detail:  err.Error(),
			})
			return nil, false
		}
		logger.Debug(ctx, "got oidc claims",
			slog.F("source", "userinfo"),
			slog.F("claim_fields", claimFields(userInfoClaims)),
			slog.F("blank", blankFields(userInfoClaims)),
		)
	case !strings.Contains(err.Error(), "user info endpoint is not supported by this provider"):
		logger.Error(ctx, "oauth2: unable to obtain user information claims", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to obtain user information claims.",
			Detail:  "The attempt to fetch claims via the UserInfo endpoint failed: " + err.Error(),
		})
		return nil, false
	default:
		// The OIDC provider does not support the UserInfo endpoint.
		// This is not an error, but we should log it as it may mean
		// that some claims are missing.
		logger.Warn(ctx, "OIDC provider does not support the user info endpoint, ensure that all required claims are present in the id_token",
			slog.Error(err),
		)
	}
	return userInfoClaims, true
}

// claimFields returns the sorted list of fields in the claims map.
func claimFields(claims map[string]interface{}) []string {
	fields := []string{}
	for field := range claims {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

// blankFields returns the list of fields in the claims map that are
// an empty string.
func blankFields(claims map[string]interface{}) []string {
	fields := make([]string, 0)
	for field, value := range claims {
		if valueStr, ok := value.(string); ok && valueStr == "" {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}

// mergeClaims merges the claims from a and b and returns the merged set.
// claims from b take precedence over claims from a.
func mergeClaims(a, b map[string]interface{}) map[string]interface{} {
	c := make(map[string]interface{})
	for k, v := range a {
		c[k] = v
	}
	for k, v := range b {
		c[k] = v
	}
	return c
}

type oauthLoginParams struct {
	User      database.User
	Link      database.UserLink
	State     httpmw.OAuth2State
	LinkedID  string
	LoginType database.LoginType

	// The following are necessary in order to
	// create new users.
	AllowSignups bool
	Email        string
	Username     string
	Name         string
	AvatarURL    string
	// OrganizationSync has the organizations that the user will be assigned to.
	OrganizationSync idpsync.OrganizationParams
	GroupSync        idpsync.GroupParams
	RoleSync         idpsync.RoleParams

	// UserClaims should only be populated for OIDC logins.
	// It is used to save the user's claims on login.
	UserClaims database.UserLinkClaims

	commitLock       sync.Mutex
	initAuditRequest func(params *audit.RequestParams) *audit.Request[database.User]
	commits          []func()
}

func (p *oauthLoginParams) SetInitAuditRequest(f func(params *audit.RequestParams) (*audit.Request[database.User], func())) *oauthLoginParams {
	p.initAuditRequest = func(params *audit.RequestParams) *audit.Request[database.User] {
		p.commitLock.Lock()
		defer p.commitLock.Unlock()
		req, commit := f(params)
		p.commits = append(p.commits, commit)
		return req
	}
	return p
}

func (p *oauthLoginParams) CommitAuditLogs() {
	p.commitLock.Lock()
	defer p.commitLock.Unlock()
	for _, f := range p.commits {
		f()
	}
}

func (api *API) oauthLogin(r *http.Request, params *oauthLoginParams) ([]*http.Cookie, database.User, database.APIKey, error) {
	var (
		ctx                  = r.Context()
		user                 database.User
		cookies              []*http.Cookie
		logger               = api.Logger.Named(userAuthLoggerName)
		auditor              = *api.Auditor.Load()
		dormantConvertAudit  *audit.Request[database.User]
		initDormantAuditOnce = sync.OnceFunc(func() {
			dormantConvertAudit = params.initAuditRequest(&audit.RequestParams{
				Audit:            auditor,
				Log:              api.Logger,
				Request:          r,
				Action:           database.AuditActionWrite,
				OrganizationID:   uuid.Nil,
				AdditionalFields: audit.BackgroundTaskFields(audit.BackgroundSubsystemDormancy),
			})
		})
	)

	var isConvertLoginType bool
	err := api.Database.InTx(func(tx database.Store) error {
		var (
			link database.UserLink
			err  error
		)
		user = params.User
		link = params.Link

		// If you do a convert to OIDC and your email does not match, we need to
		// catch this and not make a new account.
		if isMergeStateString(params.State.StateString) {
			// Always clear this cookie. If it succeeds, we no longer need it.
			// If it fails, we no longer care about it.
			cookies = append(cookies, clearOAuthConvertCookie())
			user, err = api.convertUserToOauth(ctx, r, tx, params)
			if err != nil {
				return err
			}
			params.User = user
			isConvertLoginType = true
		}

		// nolint:gocritic // Getting user count is a system function.
		userCount, err := tx.GetUserCount(dbauthz.AsSystemRestricted(ctx), false)
		if err != nil {
			return xerrors.Errorf("unable to fetch user count: %w", err)
		}

		// Allow the first user to sign up with OIDC, regardless of
		// whether signups are enabled or not.
		allowSignup := userCount == 0 || params.AllowSignups

		if user.ID == uuid.Nil && !allowSignup {
			signupsDisabledText := "Please contact your Coder administrator to request access."
			if api.OIDCConfig != nil && api.OIDCConfig.SignupsDisabledText != "" {
				signupsDisabledText = render.HTMLFromMarkdown(api.OIDCConfig.SignupsDisabledText)
			}
			return &idpsync.HTTPError{
				Code:                 http.StatusForbidden,
				Msg:                  "Signups are disabled",
				Detail:               signupsDisabledText,
				RenderStaticPage:     true,
				RenderDetailMarkdown: true,
			}
		}

		if user.ID != uuid.Nil && user.LoginType != params.LoginType {
			return wrongLoginTypeHTTPError(user.LoginType, params.LoginType)
		}

		// This can happen if a user is a built-in user but is signing in
		// with OIDC for the first time.
		if user.ID == uuid.Nil {
			//nolint:gocritic
			_, err = tx.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
				Username: params.Username,
			})
			if err == nil {
				var (
					original      = params.Username
					validUsername bool
				)
				for i := 0; i < 10; i++ {
					alternate := fmt.Sprintf("%s-%s", original, namesgenerator.GetRandomName(1))

					params.Username = codersdk.UsernameFrom(alternate)

					//nolint:gocritic
					_, err := tx.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
						Username: params.Username,
					})
					if xerrors.Is(err, sql.ErrNoRows) {
						validUsername = true
						break
					}
					if err != nil {
						return xerrors.Errorf("get user by email/username: %w", err)
					}
				}
				if !validUsername {
					return &idpsync.HTTPError{
						Code: http.StatusConflict,
						Msg:  fmt.Sprintf("exhausted alternatives for taken username %q", original),
					}
				}
			}

			//nolint:gocritic
			defaultOrganization, err := tx.GetDefaultOrganization(dbauthz.AsSystemRestricted(ctx))
			if err != nil {
				return xerrors.Errorf("unable to fetch default organization: %w", err)
			}

			rbacRoles := []string{}
			// If this is the first user, add the owner role.
			if userCount == 0 {
				rbacRoles = append(rbacRoles, rbac.RoleOwner().String())
			}

			//nolint:gocritic
			user, err = api.CreateUser(dbauthz.AsSystemRestricted(ctx), tx, CreateUserRequest{
				CreateUserRequestWithOrgs: codersdk.CreateUserRequestWithOrgs{
					Email:    params.Email,
					Username: params.Username,
					// This is a kludge, but all users are defaulted into the default
					// organization. This exists as the default behavior.
					// If org sync is enabled and configured, the user's groups
					// will change based on the org sync settings.
					OrganizationIDs: []uuid.UUID{defaultOrganization.ID},
					UserStatus:      ptr.Ref(codersdk.UserStatusActive),
				},
				LoginType:          params.LoginType,
				accountCreatorName: "oauth",
				RBACRoles:          rbacRoles,
			})
			if err != nil {
				return xerrors.Errorf("create user: %w", err)
			}

			if userCount == 0 {
				telemetryUser := telemetry.ConvertUser(user)
				// The email is not anonymized for the first user.
				telemetryUser.Email = &user.Email
				api.Telemetry.Report(&telemetry.Snapshot{
					Users: []telemetry.User{telemetryUser},
				})
			}
		}

		// Activate dormant user on sign-in
		if user.Status == database.UserStatusDormant {
			// This is necessary because transactions can be retried, and we
			// only want to add the audit log a single time.
			initDormantAuditOnce()
			dormantConvertAudit.UserID = user.ID
			dormantConvertAudit.Old = user
			//nolint:gocritic // System needs to update status of the user account (dormant -> active).
			user, err = tx.UpdateUserStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateUserStatusParams{
				ID:        user.ID,
				Status:    database.UserStatusActive,
				UpdatedAt: dbtime.Now(),
			})
			if err != nil {
				logger.Error(ctx, "unable to update user status to active", slog.Error(err))
				return xerrors.Errorf("update user status: %w", err)
			}
			dormantConvertAudit.New = user
		}

		if link.UserID == uuid.Nil {
			//nolint:gocritic // System needs to insert the user link (linked_id, oauth_token, oauth_expiry).
			link, err = tx.InsertUserLink(dbauthz.AsSystemRestricted(ctx), database.InsertUserLinkParams{
				UserID:                 user.ID,
				LoginType:              params.LoginType,
				LinkedID:               params.LinkedID,
				OAuthAccessToken:       params.State.Token.AccessToken,
				OAuthAccessTokenKeyID:  sql.NullString{}, // set by dbcrypt if required
				OAuthRefreshToken:      params.State.Token.RefreshToken,
				OAuthRefreshTokenKeyID: sql.NullString{}, // set by dbcrypt if required
				OAuthExpiry:            params.State.Token.Expiry,
				Claims:                 params.UserClaims,
			})
			if err != nil {
				return xerrors.Errorf("insert user link: %w", err)
			}
		}

		if link.UserID != uuid.Nil {
			//nolint:gocritic // System needs to update the user link (linked_id, oauth_token, oauth_expiry).
			link, err = tx.UpdateUserLink(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLinkParams{
				UserID:                 user.ID,
				LoginType:              params.LoginType,
				OAuthAccessToken:       params.State.Token.AccessToken,
				OAuthAccessTokenKeyID:  sql.NullString{}, // set by dbcrypt if required
				OAuthRefreshToken:      params.State.Token.RefreshToken,
				OAuthRefreshTokenKeyID: sql.NullString{}, // set by dbcrypt if required
				OAuthExpiry:            params.State.Token.Expiry,
				Claims:                 params.UserClaims,
			})
			if err != nil {
				return xerrors.Errorf("update user link: %w", err)
			}
		}

		err = api.IDPSync.SyncOrganizations(ctx, tx, user, params.OrganizationSync)
		if err != nil {
			return xerrors.Errorf("sync organizations: %w", err)
		}

		// Group sync needs to occur after org sync, since a user can join an org,
		// then have their groups sync to said org.
		err = api.IDPSync.SyncGroups(ctx, tx, user, params.GroupSync)
		if err != nil {
			return xerrors.Errorf("sync groups: %w", err)
		}

		// Role sync needs to occur after org sync.
		err = api.IDPSync.SyncRoles(ctx, tx, user, params.RoleSync)
		if err != nil {
			return xerrors.Errorf("sync roles: %w", err)
		}

		needsUpdate := false
		if user.AvatarURL != params.AvatarURL {
			user.AvatarURL = params.AvatarURL
			needsUpdate = true
		}
		if user.Name != params.Name {
			user.Name = params.Name
			needsUpdate = true
		}

		// If the upstream email or username has changed we should mirror
		// that in Coder. Many enterprises use a user's email/username as
		// security auditing fields so they need to stay synced.
		// NOTE: username updating has been halted since it can have infrastructure
		// provisioning consequences (updates to usernames may delete persistent
		// resources such as user home volumes).
		if user.Email != params.Email {
			user.Email = params.Email
			needsUpdate = true
		}

		if needsUpdate {
			// TODO(JonA): Since we're processing updates to a user's upstream
			// email/username, it's possible for a different built-in user to
			// have already claimed the username.
			// In such cases in the current implementation this user can now no
			// longer sign in until an administrator finds the offending built-in
			// user and changes their username.
			//nolint:gocritic
			user, err = tx.UpdateUserProfile(dbauthz.AsSystemRestricted(ctx), database.UpdateUserProfileParams{
				ID:        user.ID,
				Email:     user.Email,
				Name:      user.Name,
				Username:  user.Username,
				UpdatedAt: dbtime.Now(),
				AvatarURL: user.AvatarURL,
			})
			if err != nil {
				return xerrors.Errorf("update user profile: %w", err)
			}
		}

		return nil
	}, nil)
	if err != nil {
		return nil, database.User{}, database.APIKey{}, xerrors.Errorf("in tx: %w", err)
	}

	var key database.APIKey
	oldKey, _, ok := httpmw.APIKeyFromRequest(ctx, api.Database, nil, r)
	if ok && oldKey != nil && isConvertLoginType {
		// If this is a convert login type, and it succeeds, then delete the old
		// session. Force the user to log back in.
		err := api.Database.DeleteAPIKeyByID(r.Context(), oldKey.ID)
		if err != nil {
			// Do not block this login if we fail to delete the old API key.
			// Just delete the cookie and continue.
			api.Logger.Warn(r.Context(), "failed to delete old API key in convert to oidc",
				slog.Error(err),
				slog.F("old_api_key_id", oldKey.ID),
				slog.F("user_id", user.ID),
			)
		}
		cookies = append(cookies, &http.Cookie{
			Name:     codersdk.SessionTokenCookie,
			Path:     "/",
			MaxAge:   -1,
			Secure:   api.SecureAuthCookie,
			HttpOnly: true,
		})
		// This is intentional setting the key to the deleted old key,
		// as the user needs to be forced to log back in.
		key = *oldKey
	} else {
		//nolint:gocritic
		cookie, newKey, err := api.createAPIKey(dbauthz.AsSystemRestricted(ctx), apikey.CreateParams{
			UserID:          user.ID,
			LoginType:       params.LoginType,
			DefaultLifetime: api.DeploymentValues.Sessions.DefaultDuration.Value(),
			RemoteAddr:      r.RemoteAddr,
		})
		if err != nil {
			return nil, database.User{}, database.APIKey{}, xerrors.Errorf("create API key: %w", err)
		}
		cookies = append(cookies, cookie)
		key = *newKey
	}

	return cookies, user, key, nil
}

// convertUserToOauth will convert a user from password base loginType to
// an oauth login type. If it fails, it will return a httpError
func (api *API) convertUserToOauth(ctx context.Context, r *http.Request, db database.Store, params *oauthLoginParams) (database.User, error) {
	user := params.User

	// Trying to convert to OIDC, but the email does not match.
	// So do not make a new user, just block the request.
	if user.ID == uuid.Nil {
		return database.User{}, idpsync.HTTPError{
			Code: http.StatusBadRequest,
			Msg:  fmt.Sprintf("The oidc account with the email %q does not match the email of the account you are trying to convert. Contact your administrator to resolve this issue.", params.Email),
		}
	}

	jwtCookie, err := r.Cookie(OAuthConvertCookieValue)
	if err != nil {
		return database.User{}, idpsync.HTTPError{
			Code: http.StatusBadRequest,
			Msg: fmt.Sprintf("Convert to oauth cookie not found. Missing signed jwt to authorize this action. " +
				"Please try again."),
		}
	}
	var claims OAuthConvertStateClaims

	err = jwtutils.Verify(ctx, api.OIDCConvertKeyCache, jwtCookie.Value, &claims)
	if xerrors.Is(err, cryptokeys.ErrKeyNotFound) || xerrors.Is(err, cryptokeys.ErrKeyInvalid) || xerrors.Is(err, jose.ErrCryptoFailure) || xerrors.Is(err, jwtutils.ErrMissingKeyID) {
		// These errors are probably because the user is mixing 2 coder deployments.
		return database.User{}, idpsync.HTTPError{
			Code: http.StatusBadRequest,
			Msg:  "Using an invalid jwt to authorize this action. Ensure there is only 1 coder deployment and try again.",
		}
	}
	if err != nil {
		return database.User{}, idpsync.HTTPError{
			Code: http.StatusInternalServerError,
			Msg:  fmt.Sprintf("Error parsing jwt: %v", err),
		}
	}

	// At this point, this request could be an attempt to convert from
	// password auth to oauth auth. Always log these attempts.
	var (
		auditor           = *api.Auditor.Load()
		oauthConvertAudit = params.initAuditRequest(&audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)

	oauthConvertAudit.UserID = claims.UserID
	oauthConvertAudit.Old = user

	if claims.Issuer != api.DeploymentID {
		return database.User{}, idpsync.HTTPError{
			Code: http.StatusForbidden,
			Msg:  "Request to convert login type failed. Issuer mismatch. Found a cookie from another coder deployment, please try again.",
		}
	}

	if params.State.StateString != claims.State {
		return database.User{}, idpsync.HTTPError{
			Code: http.StatusForbidden,
			Msg:  "Request to convert login type failed. State mismatch.",
		}
	}

	// Make sure the merge state generated matches this OIDC login request.
	// It needs to have the correct login type information for this
	// user.
	if user.ID != claims.UserID ||
		codersdk.LoginType(user.LoginType) != claims.FromLoginType ||
		codersdk.LoginType(params.LoginType) != claims.ToLoginType {
		return database.User{}, idpsync.HTTPError{
			Code: http.StatusForbidden,
			Msg:  fmt.Sprintf("Request to convert login type from %s to %s failed", user.LoginType, params.LoginType),
		}
	}

	// Convert the user and default to the normal login flow.
	// If the login succeeds, this transaction will commit and the user
	// will be converted.
	// nolint:gocritic // system query to update user login type. The user already
	// provided their password to authenticate this request.
	user, err = db.UpdateUserLoginType(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLoginTypeParams{
		NewLoginType: params.LoginType,
		UserID:       user.ID,
	})
	if err != nil {
		return database.User{}, idpsync.HTTPError{
			Code: http.StatusInternalServerError,
			Msg:  "Failed to convert user to new login type",
		}
	}
	oauthConvertAudit.New = user
	return user, nil
}

// githubLinkedID returns the unique ID for a GitHub user.
func githubLinkedID(u *github.User) string {
	return strconv.FormatInt(u.GetID(), 10)
}

// oidcLinkedID returns the uniqued ID for an OIDC user.
// See https://openid.net/specs/openid-connect-core-1_0.html#ClaimStability .
func oidcLinkedID(tok *oidc.IDToken) string {
	return strings.Join([]string{tok.Issuer, tok.Subject}, "||")
}

// findLinkedUser tries to find a user by their unique OAuth-linked ID.
// If it doesn't not find it, it returns the user by their email.
func findLinkedUser(ctx context.Context, db database.Store, linkedID string, emails ...string) (database.User, database.UserLink, error) {
	var (
		user database.User
		link database.UserLink
	)
	link, err := db.GetUserLinkByLinkedID(ctx, linkedID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return user, link, xerrors.Errorf("get user auth by linked ID: %w", err)
	}

	if err == nil {
		user, err = db.GetUserByID(ctx, link.UserID)
		if err != nil {
			return database.User{}, database.UserLink{}, xerrors.Errorf("get user by id: %w", err)
		}
		if !user.Deleted {
			return user, link, nil
		}
		// If the user was deleted, act as if no account link exists.
		user = database.User{}
	}

	for _, email := range emails {
		user, err = db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
			Email: email,
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return user, link, xerrors.Errorf("get user by email: %w", err)
		}
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		break
	}

	if user.ID == uuid.Nil {
		// No user found.
		return database.User{}, database.UserLink{}, nil
	}

	// LEGACY: This is annoying but we have to search for the user_link
	// again except this time we search by user_id and login_type. It's
	// possible that a user_link exists without a populated 'linked_id'.
	link, err = db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
		UserID:    user.ID,
		LoginType: user.LoginType,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return database.User{}, database.UserLink{}, xerrors.Errorf("get user link by user id and login type: %w", err)
	}

	return user, link, nil
}

func isMergeStateString(state string) bool {
	return strings.HasPrefix(state, mergeStateStringPrefix)
}

func clearOAuthConvertCookie() *http.Cookie {
	return &http.Cookie{
		Name:   OAuthConvertCookieValue,
		Path:   "/",
		MaxAge: -1,
	}
}

func wrongLoginTypeHTTPError(user database.LoginType, params database.LoginType) idpsync.HTTPError {
	addedMsg := ""
	if user == database.LoginTypePassword {
		addedMsg = " You can convert your account to use this login type by visiting your account settings."
	}
	return idpsync.HTTPError{
		Code:             http.StatusForbidden,
		RenderStaticPage: true,
		Msg:              "Incorrect login type",
		Detail: fmt.Sprintf("Attempting to use login type %q, but the user has the login type %q.%s",
			params, user, addedMsg),
	}
}
