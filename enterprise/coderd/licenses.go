package coderd

import (
	"context"
	"crypto/ed25519"
	_ "embed"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"

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

//go:embed keys/2022-08-12
var key20220812 []byte

var keys = map[string]ed25519.PublicKey{"2022-08-12": ed25519.PublicKey(key20220812)}

type Features struct {
	UserLimit int64 `json:"user_limit"`
	AuditLog  int64 `json:"audit_log"`
}

type Claims struct {
	jwt.RegisteredClaims
	LicenseExpires *jwt.NumericDate `json:"license_expires,omitempty"`
	AccountType    string           `json:"account_type,omitempty"`
	AccountID      string           `json:"account_id,omitempty"`
	Version        uint64           `json:"version"`
	Features       Features         `json:"features"`
}

var (
	ErrInvalidVersion = xerrors.New("license must be version 3")
	ErrMissingKeyID   = xerrors.Errorf("JOSE header must contain %s", HeaderKeyID)
)

// parseLicense parses the license and returns the claims. If the license's signature is invalid or
// is not parsable, an error is returned.
func parseLicense(l string, keys map[string]ed25519.PublicKey) (jwt.MapClaims, error) {
	tok, err := jwt.Parse(
		l,
		keyFunc(keys),
		jwt.WithValidMethods(ValidMethods),
	)
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
	if err != nil {
		return nil, err
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

type LicenseAPI struct {
	handler  chi.Router
	logger   slog.Logger
	database database.Store
	pubsub   database.Pubsub
	auth     *coderd.HTTPAuthorizer
}

func NewLicenseAPI(
	l slog.Logger,
	db database.Store,
	ps database.Pubsub,
	auth *coderd.HTTPAuthorizer) *LicenseAPI {
	r := chi.NewRouter()
	a := &LicenseAPI{handler: r, logger: l, database: db, pubsub: ps, auth: auth}
	r.Post("/", a.postLicense)
	return a
}

func (a *LicenseAPI) Handler() http.Handler {
	return a.handler
}

func (a *LicenseAPI) postLicense(rw http.ResponseWriter, r *http.Request) {
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
		Jwt:        addLicense.License,
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
