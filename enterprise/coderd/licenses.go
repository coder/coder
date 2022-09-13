package coderd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

const (
	CurrentVersion        = 3
	HeaderKeyID           = "kid"
	AccountTypeSalesforce = "salesforce"
	VersionClaim          = "version"
	PubSubEventLicenses   = "licenses"
)

var ValidMethods = []string{"EdDSA"}

// key20220812 is the Coder license public key with id 2022-08-12 used to validate licenses signed
// by our signing infrastructure
//
//go:embed keys/2022-08-12
var key20220812 []byte

var keys = map[string]ed25519.PublicKey{"2022-08-12": ed25519.PublicKey(key20220812)}

type Features struct {
	UserLimit int64 `json:"user_limit"`
	AuditLog  int64 `json:"audit_log"`
}

type Claims struct {
	jwt.RegisteredClaims
	// LicenseExpires is the end of the legit license term, and the start of the grace period, if
	// there is one.  The standard JWT claim "exp" (ExpiresAt in jwt.RegisteredClaims, above) is
	// the end of the grace period (identical to LicenseExpires if there is no grace period).
	// The reason we use the standard claim for the end of the grace period is that we want JWT
	// processing libraries to consider the token "valid" until then.
	LicenseExpires *jwt.NumericDate `json:"license_expires,omitempty"`
	AccountType    string           `json:"account_type,omitempty"`
	AccountID      string           `json:"account_id,omitempty"`
	Version        uint64           `json:"version"`
	Features       Features         `json:"features"`
}

var (
	ErrInvalidVersion        = xerrors.New("license must be version 3")
	ErrMissingKeyID          = xerrors.Errorf("JOSE header must contain %s", HeaderKeyID)
	ErrMissingLicenseExpires = xerrors.New("license missing license_expires")
)

// parseLicense parses the license and returns the claims. If the license's signature is invalid or
// is not parsable, an error is returned.
func parseLicense(l string, keys map[string]ed25519.PublicKey) (jwt.MapClaims, error) {
	tok, err := jwt.Parse(
		l,
		keyFunc(keys),
		jwt.WithValidMethods(ValidMethods),
	)
	if err != nil {
		return nil, err
	}
	if claims, ok := tok.Claims.(jwt.MapClaims); ok && tok.Valid {
		version, ok := claims[VersionClaim].(float64)
		if !ok {
			return nil, ErrInvalidVersion
		}
		if int64(version) != CurrentVersion {
			return nil, ErrInvalidVersion
		}
		return claims, nil
	}
	return nil, xerrors.New("unable to parse Claims")
}

// validateDBLicense validates a database.License record, and if valid, returns the claims.  If
// unparsable or invalid, it returns an error
func validateDBLicense(l database.License, keys map[string]ed25519.PublicKey) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(
		l.JWT,
		&Claims{},
		keyFunc(keys),
		jwt.WithValidMethods(ValidMethods),
	)
	if err != nil {
		return nil, err
	}
	if claims, ok := tok.Claims.(*Claims); ok && tok.Valid {
		if claims.Version != uint64(CurrentVersion) {
			return nil, ErrInvalidVersion
		}
		if claims.LicenseExpires == nil {
			return nil, ErrMissingLicenseExpires
		}
		return claims, nil
	}
	return nil, xerrors.New("unable to parse Claims")
}

func keyFunc(keys map[string]ed25519.PublicKey) func(*jwt.Token) (interface{}, error) {
	return func(j *jwt.Token) (interface{}, error) {
		keyID, ok := j.Header[HeaderKeyID].(string)
		if !ok {
			return nil, ErrMissingKeyID
		}
		k, ok := keys[keyID]
		if !ok {
			return nil, xerrors.Errorf("no key with ID %s", keyID)
		}
		return k, nil
	}
}

// licenseAPI handles enterprise licenses, and attaches to the main coderd.API via the
// LicenseHandler option, so that it serves all routes under /api/v2/licenses
type licenseAPI struct {
	router   chi.Router
	logger   slog.Logger
	database database.Store
	pubsub   database.Pubsub
	auth     *coderd.HTTPAuthorizer
}

func newLicenseAPI(
	l slog.Logger,
	db database.Store,
	ps database.Pubsub,
	auth *coderd.HTTPAuthorizer,
) *licenseAPI {
	r := chi.NewRouter()
	a := &licenseAPI{router: r, logger: l, database: db, pubsub: ps, auth: auth}
	r.Post("/", a.postLicense)
	r.Get("/", a.licenses)
	r.Delete("/{id}", a.delete)
	return a
}

func (a *licenseAPI) handler() http.Handler {
	return a.router
}

// postLicense adds a new Enterprise license to the cluster.  We allow multiple different licenses
// in the cluster at one time for several reasons:
//
//  1. Upgrades --- if the license format changes from one version of Coder to the next, during a
//     rolling update you will have different Coder servers that need different licenses to function.
//  2. Avoid abrupt feature breakage --- when an admin uploads a new license with different features
//     we generally don't want the old features to immediately break without warning.  With a grace
//     period on the license, features will continue to work from the old license until its grace
//     period, then the users will get a warning allowing them to gracefully stop using the feature.
func (a *licenseAPI) postLicense(rw http.ResponseWriter, r *http.Request) {
	if !a.auth.Authorize(r, rbac.ActionCreate, rbac.ResourceLicense) {
		httpapi.Forbidden(rw)
		return
	}

	var addLicense codersdk.AddLicenseRequest
	if !httpapi.Read(rw, r, &addLicense) {
		return
	}

	claims, err := parseLicense(addLicense.License, keys)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid license",
			Detail:  err.Error(),
		})
		return
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid license",
			Detail:  "exp claim missing or not parsable",
		})
		return
	}
	expTime := time.Unix(int64(exp), 0)

	dl, err := a.database.InsertLicense(r.Context(), database.InsertLicenseParams{
		UploadedAt: database.Now(),
		JWT:        addLicense.License,
		Exp:        expTime,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Unable to add license to database",
			Detail:  err.Error(),
		})
		return
	}
	err = a.pubsub.Publish(PubSubEventLicenses, []byte("add"))
	if err != nil {
		a.logger.Error(context.Background(), "failed to publish license add", slog.Error(err))
		// don't fail the HTTP request, since we did write it successfully to the database
	}

	httpapi.Write(rw, http.StatusCreated, convertLicense(dl, claims))
}

func convertLicense(dl database.License, c jwt.MapClaims) codersdk.License {
	return codersdk.License{
		ID:         dl.ID,
		UploadedAt: dl.UploadedAt,
		Claims:     c,
	}
}

func (a *licenseAPI) licenses(rw http.ResponseWriter, r *http.Request) {
	licenses, err := a.database.GetLicenses(r.Context())
	if xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusOK, []codersdk.License{})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching licenses.",
			Detail:  err.Error(),
		})
		return
	}

	licenses, err = coderd.AuthorizeFilter(a.auth, r, rbac.ActionRead, licenses)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching licenses.",
			Detail:  err.Error(),
		})
		return
	}
	sdkLicenses, err := convertLicenses(licenses)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error parsing licenses.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(rw, http.StatusOK, sdkLicenses)
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

func (a *licenseAPI) delete(rw http.ResponseWriter, r *http.Request) {
	if !a.auth.Authorize(r, rbac.ActionDelete, rbac.ResourceLicense) {
		httpapi.Forbidden(rw)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: "License ID must be an integer",
		})
		return
	}

	_, err = a.database.DeleteLicense(r.Context(), int32(id))
	if xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: "Unknown license ID",
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting license",
			Detail:  err.Error(),
		})
		return
	}

	err = a.pubsub.Publish(PubSubEventLicenses, []byte("delete"))
	if err != nil {
		a.logger.Error(context.Background(), "failed to publish license delete", slog.Error(err))
		// don't fail the HTTP request, since we did write it successfully to the database
	}
	rw.WriteHeader(http.StatusOK)
}
