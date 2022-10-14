package coderdenttest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd"
	"github.com/coder/coder/enterprise/coderd/license"
)

const (
	testKeyID = "enterprise-test"
)

var (
	testPrivateKey ed25519.PrivateKey
	testPublicKey  ed25519.PublicKey

	Keys = map[string]ed25519.PublicKey{}
)

func init() {
	var err error
	testPublicKey, testPrivateKey, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	Keys[testKeyID] = testPublicKey
}

type Options struct {
	*coderdtest.Options
	AuditLogging               bool
	BrowserOnly                bool
	EntitlementsUpdateInterval time.Duration
	SCIMAPIKey                 []byte
	UserWorkspaceQuota         int
}

// New constructs a codersdk client connected to an in-memory Enterprise API instance.
func New(t *testing.T, options *Options) *codersdk.Client {
	client, _, _ := NewWithAPI(t, options)
	return client
}

func NewWithAPI(t *testing.T, options *Options) (*codersdk.Client, io.Closer, *coderd.API) {
	if options == nil {
		options = &Options{}
	}
	if options.Options == nil {
		options.Options = &coderdtest.Options{}
	}
	srv, cancelFunc, oop := coderdtest.NewOptions(t, options.Options)
	coderAPI, err := coderd.New(context.Background(), &coderd.Options{
		RBACEnabled:                true,
		AuditLogging:               options.AuditLogging,
		BrowserOnly:                options.BrowserOnly,
		SCIMAPIKey:                 options.SCIMAPIKey,
		UserWorkspaceQuota:         options.UserWorkspaceQuota,
		Options:                    oop,
		EntitlementsUpdateInterval: options.EntitlementsUpdateInterval,
		Keys:                       Keys,
	})
	assert.NoError(t, err)
	srv.Config.Handler = coderAPI.AGPL.RootHandler
	var provisionerCloser io.Closer = nopcloser{}
	if options.IncludeProvisionerDaemon {
		provisionerCloser = coderdtest.NewProvisionerDaemon(t, coderAPI.AGPL)
	}

	t.Cleanup(func() {
		cancelFunc()
		_ = provisionerCloser.Close()
		_ = coderAPI.Close()
	})
	return codersdk.New(coderAPI.AccessURL), provisionerCloser, coderAPI
}

type LicenseOptions struct {
	AccountType         string
	AccountID           string
	Trial               bool
	AllFeatures         bool
	GraceAt             time.Time
	ExpiresAt           time.Time
	UserLimit           int64
	AuditLog            bool
	BrowserOnly         bool
	SCIM                bool
	WorkspaceQuota      bool
	TemplateRBACEnabled bool
}

// AddLicense generates a new license with the options provided and inserts it.
func AddLicense(t *testing.T, client *codersdk.Client, options LicenseOptions) codersdk.License {
	l, err := client.AddLicense(context.Background(), codersdk.AddLicenseRequest{
		License: GenerateLicense(t, options),
	})
	require.NoError(t, err)
	return l
}

// GenerateLicense returns a signed JWT using the test key.
func GenerateLicense(t *testing.T, options LicenseOptions) string {
	if options.ExpiresAt.IsZero() {
		options.ExpiresAt = time.Now().Add(time.Hour)
	}
	if options.GraceAt.IsZero() {
		options.GraceAt = time.Now().Add(time.Hour)
	}
	var auditLog int64
	if options.AuditLog {
		auditLog = 1
	}
	var browserOnly int64
	if options.BrowserOnly {
		browserOnly = 1
	}
	var scim int64
	if options.SCIM {
		scim = 1
	}
	var workspaceQuota int64
	if options.WorkspaceQuota {
		workspaceQuota = 1
	}

	rbacEnabled := int64(0)
	if options.TemplateRBACEnabled {
		rbacEnabled = 1
	}

	c := &license.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test@testing.test",
			ExpiresAt: jwt.NewNumericDate(options.ExpiresAt),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
		LicenseExpires: jwt.NewNumericDate(options.GraceAt),
		AccountType:    options.AccountType,
		AccountID:      options.AccountID,
		Trial:          options.Trial,
		Version:        license.CurrentVersion,
		AllFeatures:    options.AllFeatures,
		Features: license.Features{
			UserLimit:      options.UserLimit,
			AuditLog:       auditLog,
			BrowserOnly:    browserOnly,
			SCIM:           scim,
			WorkspaceQuota: workspaceQuota,
			TemplateRBAC:   rbacEnabled,
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	tok.Header[license.HeaderKeyID] = testKeyID
	signedTok, err := tok.SignedString(testPrivateKey)
	require.NoError(t, err)
	return signedTok
}

type nopcloser struct{}

func (nopcloser) Close() error { return nil }
