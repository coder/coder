package codersdk_test

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type exclusion struct {
	flag bool
	env  bool
	yaml bool
}

func TestDeploymentValues_HighlyConfigurable(t *testing.T) {
	t.Parallel()

	// This test ensures that every deployment option has
	// a corresponding Flag, Env, and YAML name, unless explicitly excluded.

	excludes := map[string]exclusion{
		// These are used to configure YAML support itself, so
		// they make no sense within the YAML file.
		"Config Path": {
			yaml: true,
		},
		"Write Config": {
			yaml: true,
			env:  true,
		},
		// Dangerous values? Not sure we should help users
		// persistent their configuration.
		"DANGEROUS: Allow Path App Sharing": {
			yaml: true,
		},
		"DANGEROUS: Allow Site Owners to Access Path Apps": {
			yaml: true,
		},
		// Secrets
		"Trace Honeycomb API Key": {
			yaml: true,
		},
		"OAuth2 GitHub Client Secret": {
			yaml: true,
		},
		"OIDC Client Secret": {
			yaml: true,
		},
		"Postgres Connection URL": {
			yaml: true,
		},
		"SCIM API Key": {
			yaml: true,
		},
		"External Token Encryption Keys": {
			yaml: true,
		},
		"External Auth Providers": {
			// Technically External Auth Providers can be provided through the env,
			// but bypassing serpent. See cli.ReadExternalAuthProvidersFromEnv.
			flag: true,
			env:  true,
		},
		"Provisioner Daemon Pre-shared Key (PSK)": {
			yaml: true,
		},
	}

	set := (&codersdk.DeploymentValues{}).Options()
	for _, opt := range set {
		// These are generally for development, so their configurability is
		// not relevant.
		if opt.Hidden {
			delete(excludes, opt.Name)
			continue
		}

		if codersdk.IsSecretDeploymentOption(opt) && opt.YAML != "" {
			// Secrets should not be written to YAML and instead should continue
			// to be read from the environment.
			//
			// Unfortunately, secrets are still accepted through flags for
			// legacy purposes. Eventually, we should prevent that.
			t.Errorf("Option %q is a secret but has a YAML name", opt.Name)
		}

		excluded := excludes[opt.Name]
		switch {
		case opt.YAML == "" && !excluded.yaml:
			t.Errorf("Option %q should have a YAML name", opt.Name)
		case opt.YAML != "" && excluded.yaml:
			t.Errorf("Option %q is excluded but has a YAML name", opt.Name)
		case opt.Flag == "" && !excluded.flag:
			t.Errorf("Option %q should have a flag name", opt.Name)
		case opt.Flag != "" && excluded.flag:
			t.Errorf("Option %q is excluded but has a flag name", opt.Name)
		case opt.Env == "" && !excluded.env:
			t.Errorf("Option %q should have an env name", opt.Name)
		case opt.Env != "" && excluded.env:
			t.Errorf("Option %q is excluded but has an env name", opt.Name)
		}

		// Also check all env vars are prefixed with CODER_
		const prefix = "CODER_"
		if opt.Env != "" && !strings.HasPrefix(opt.Env, prefix) {
			t.Errorf("Option %q has an env name (%q) that is not prefixed with %s", opt.Name, opt.Env, prefix)
		}

		delete(excludes, opt.Name)
	}

	for opt := range excludes {
		t.Errorf("Excluded option %q is not in the deployment config. Remove it?", opt)
	}
}

func TestSSHConfig_ParseOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name          string
		ConfigOptions serpent.StringArray
		ExpectError   bool
		Expect        map[string]string
	}{
		{
			Name:          "Empty",
			ConfigOptions: []string{},
			Expect:        map[string]string{},
		},
		{
			Name: "Whitespace",
			ConfigOptions: []string{
				"test value",
			},
			Expect: map[string]string{
				"test": "value",
			},
		},
		{
			Name: "SimpleValueEqual",
			ConfigOptions: []string{
				"test=value",
			},
			Expect: map[string]string{
				"test": "value",
			},
		},
		{
			Name: "SimpleValues",
			ConfigOptions: []string{
				"test=value",
				"foo=bar",
			},
			Expect: map[string]string{
				"test": "value",
				"foo":  "bar",
			},
		},
		{
			Name: "ValueWithQuote",
			ConfigOptions: []string{
				"bar=buzz=bazz",
			},
			Expect: map[string]string{
				"bar": "buzz=bazz",
			},
		},
		{
			Name: "NoEquals",
			ConfigOptions: []string{
				"foobar",
			},
			ExpectError: true,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			c := codersdk.SSHConfig{
				SSHConfigOptions: tt.ConfigOptions,
			}
			got, err := c.ParseOptions()
			if tt.ExpectError {
				require.Error(t, err, tt.ConfigOptions.String())
			} else {
				require.NoError(t, err, tt.ConfigOptions.String())
				require.Equalf(t, tt.Expect, got, tt.ConfigOptions.String())
			}
		})
	}
}

func TestTimezoneOffsets(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name           string
		Now            time.Time
		Loc            *time.Location
		ExpectedOffset int
	}{
		{
			Name:           "UTC",
			Loc:            time.UTC,
			ExpectedOffset: 0,
		},

		{
			Name:           "Eastern",
			Now:            time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("America/New_York")),
			ExpectedOffset: 5,
		},
		{
			// Daylight savings is on the 14th of March to Nov 7 in 2021
			Name:           "EasternDaylightSavings",
			Now:            time.Date(2021, 3, 16, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("America/New_York")),
			ExpectedOffset: 4,
		},
		{
			Name:           "Central",
			Now:            time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("America/Chicago")),
			ExpectedOffset: 6,
		},
		{
			Name:           "CentralDaylightSavings",
			Now:            time.Date(2021, 3, 16, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("America/Chicago")),
			ExpectedOffset: 5,
		},
		{
			Name:           "Ireland",
			Now:            time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("Europe/Dublin")),
			ExpectedOffset: 0,
		},
		{
			Name:           "IrelandDaylightSavings",
			Now:            time.Date(2021, 4, 3, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("Europe/Dublin")),
			ExpectedOffset: -1,
		},
		{
			Name: "HalfHourTz",
			Now:  time.Date(2024, 1, 20, 6, 0, 0, 0, must(time.LoadLocation("Asia/Yangon"))),
			// This timezone is +6:30, but the function rounds to the nearest hour.
			// This is intentional because our DAUs endpoint only covers 1-hour offsets.
			// If the user is in a non-hour timezone, they get the closest hour bucket.
			Loc:            must(time.LoadLocation("Asia/Yangon")),
			ExpectedOffset: -6,
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			offset := codersdk.TimezoneOffsetHourWithTime(c.Now, c.Loc)
			require.Equal(t, c.ExpectedOffset, offset)
		})
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func TestDeploymentValues_DurationFormatNanoseconds(t *testing.T) {
	t.Parallel()

	set := (&codersdk.DeploymentValues{}).Options()
	for _, s := range set {
		if s.Value.Type() != "duration" {
			continue
		}
		// Just make sure the annotation is set.
		// If someone wants to not format a duration, they can
		// explicitly set the annotation to false.
		if s.Annotations.IsSet("format_duration") {
			continue
		}
		t.Logf("Option %q is a duration but does not have the format_duration annotation.", s.Name)
		t.Logf("To fix this, add the following to the option declaration:")
		t.Logf(`Annotations: serpent.Annotations{}.Mark(annotationFormatDurationNS, "true"),`)
		t.FailNow()
	}
}

//go:embed testdata/*
var testData embed.FS

func TestExternalAuthYAMLConfig(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		// The windows marshal function uses different line endings.
		// Not worth the effort getting this to work on windows.
		t.SkipNow()
	}

	file := func(t *testing.T, name string) string {
		data, err := testData.ReadFile(fmt.Sprintf("testdata/%s", name))
		require.NoError(t, err, "read testdata file %q", name)
		return string(data)
	}
	githubCfg := codersdk.ExternalAuthConfig{
		Type:                "github",
		ClientID:            "client_id",
		ClientSecret:        "client_secret",
		ID:                  "id",
		AuthURL:             "https://example.com/auth",
		TokenURL:            "https://example.com/token",
		ValidateURL:         "https://example.com/validate",
		AppInstallURL:       "https://example.com/install",
		AppInstallationsURL: "https://example.com/installations",
		NoRefresh:           true,
		Scopes:              []string{"user:email", "read:org"},
		ExtraTokenKeys:      []string{"extra", "token"},
		DeviceFlow:          true,
		DeviceCodeURL:       "https://example.com/device",
		Regex:               "^https://example.com/.*$",
		DisplayName:         "GitHub",
		DisplayIcon:         "/static/icons/github.svg",
	}

	// Input the github section twice for testing a slice of configs.
	inputYAML := func() string {
		f := file(t, "githubcfg.yaml")
		lines := strings.SplitN(f, "\n", 2)
		// Append github config twice
		return f + lines[1]
	}()

	expected := []codersdk.ExternalAuthConfig{
		githubCfg, githubCfg,
	}

	dv := codersdk.DeploymentValues{}
	opts := dv.Options()
	// replace any tabs with the proper space indentation
	inputYAML = strings.ReplaceAll(inputYAML, "\t", "  ")

	// This is the order things are done in the cli, so just
	// keep it the same.
	var n yaml.Node
	err := yaml.Unmarshal([]byte(inputYAML), &n)
	require.NoError(t, err)

	err = n.Decode(&opts)
	require.NoError(t, err)
	require.ElementsMatchf(t, expected, dv.ExternalAuthConfigs.Value, "from yaml")

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	err = enc.Encode(dv.ExternalAuthConfigs)
	require.NoError(t, err)

	// Because we only marshal the 1 section, the correct section name is not applied.
	output := strings.Replace(out.String(), "value:", "externalAuthProviders:", 1)
	require.Equal(t, inputYAML, output, "re-marshaled is the same as input")
}

func TestFeatureComparison(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name     string
		A        codersdk.Feature
		B        codersdk.Feature
		Expected int
	}{
		{
			Name:     "Empty",
			Expected: 0,
		},
		// Entitlement check
		//		Entitled
		{
			Name:     "EntitledVsGracePeriod",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod},
			Expected: 1,
		},
		{
			Name: "EntitledVsGracePeriodLimits",
			A:    codersdk.Feature{Entitlement: codersdk.EntitlementEntitled},
			// Entitled should still win here
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: ptr.Ref[int64](100), Actual: ptr.Ref[int64](50)},
			Expected: 1,
		},
		{
			Name:     "EntitledVsNotEntitled",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementNotEntitled},
			Expected: 3,
		},
		{
			Name:     "EntitledVsUnknown",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled},
			B:        codersdk.Feature{Entitlement: ""},
			Expected: 4,
		},
		//		GracePeriod
		{
			Name:     "GracefulVsNotEntitled",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementNotEntitled},
			Expected: 2,
		},
		{
			Name:     "GracefulVsUnknown",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod},
			B:        codersdk.Feature{Entitlement: ""},
			Expected: 3,
		},
		//		NotEntitled
		{
			Name:     "NotEntitledVsUnknown",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementNotEntitled},
			B:        codersdk.Feature{Entitlement: ""},
			Expected: 1,
		},
		// --
		{
			Name:     "EntitledVsGracePeriodCapable",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref[int64](100), Actual: ptr.Ref[int64](200)},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: ptr.Ref[int64](300), Actual: ptr.Ref[int64](200)},
			Expected: -1,
		},
		// UserLimits
		{
			// Tests an exceeded limit that is entitled vs a graceful limit that
			// is not exceeded. This is the edge case that we should use the graceful period
			// instead of the entitled.
			Name:     "UserLimitExceeded",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(200))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: ptr.Ref(int64(300)), Actual: ptr.Ref(int64(200))},
			Expected: -1,
		},
		{
			Name:     "UserLimitExceededNoEntitled",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(200))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementNotEntitled, Limit: ptr.Ref(int64(300)), Actual: ptr.Ref(int64(200))},
			Expected: 3,
		},
		{
			Name:     "HigherLimit",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(110)), Actual: ptr.Ref(int64(200))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(200))},
			Expected: 10, // Diff in the limit #
		},
		{
			Name:     "HigherActual",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(300))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(200))},
			Expected: 100, // Diff in the actual #
		},
		{
			Name:     "LimitExists",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(50))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: nil, Actual: ptr.Ref(int64(200))},
			Expected: 1,
		},
		{
			Name:     "LimitExistsGrace",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(50))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: nil, Actual: ptr.Ref(int64(200))},
			Expected: 1,
		},
		{
			Name:     "ActualExists",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(50))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: nil},
			Expected: 1,
		},
		{
			Name:     "NotNils",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(50))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: nil, Actual: nil},
			Expected: 1,
		},
		{
			// This is super strange, but it is possible to have a limit but no actual.
			// Just adding this test case to solidify the behavior.
			// Feel free to change this if you think it should be different.
			Name:     "LimitVsActual",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: nil},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: nil, Actual: ptr.Ref(int64(200))},
			Expected: 1,
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			r := codersdk.CompareFeatures(tc.A, tc.B)
			logIt := !assert.Equal(t, tc.Expected, r)

			// Comparisons should be like addition. A - B = -1 * (B - A)
			r = codersdk.CompareFeatures(tc.B, tc.A)
			logIt = logIt || !assert.Equalf(t, tc.Expected*-1, r, "the inverse comparison should also be true")
			if logIt {
				ad, _ := json.Marshal(tc.A)
				bd, _ := json.Marshal(tc.B)
				t.Logf("a = %s\nb = %s", ad, bd)
			}
		})
	}
}

// TestPremiumSuperSet tests that the "premium" feature set is a superset of the
// "enterprise" feature set.
func TestPremiumSuperSet(t *testing.T) {
	t.Parallel()

	enterprise := codersdk.FeatureSetEnterprise
	premium := codersdk.FeatureSetPremium
	require.Subset(t, premium.Features(), enterprise.Features(), "premium should be a superset of enterprise. If this fails, update the premium feature set to include all enterprise features.")
}
