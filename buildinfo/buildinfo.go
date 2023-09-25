package buildinfo

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

var (
	buildInfo      *debug.BuildInfo
	buildInfoValid bool
	readBuildInfo  sync.Once

	externalURL     string
	readExternalURL sync.Once

	version     string
	readVersion sync.Once

	// Updated by buildinfo_slim.go on start.
	slim bool

	// Injected with ldflags at build, see scripts/build_go.sh
	tag  string
	agpl string // either "true" or "false", ldflags does not support bools
)

const (
	// noVersion is the reported version when the version cannot be determined.
	// Usually because `go build` is run instead of `make build`.
	noVersion = "v0.0.0"

	// develPreRelease is the pre-release tag for developer versions of the
	// application. This includes CI builds. The pre-release tag should be appended
	// to the version with a "-".
	// Example: v0.0.0-devel
	develPreRelease = "devel"
)

// Version returns the semantic version of the build.
// Use golang.org/x/mod/semver to compare versions.
func Version() string {
	readVersion.Do(func() {
		revision, valid := revision()
		if valid {
			revision = "+" + revision[:7]
		}
		if tag == "" {
			// This occurs when the tag hasn't been injected,
			// like when using "go run".
			// <version>-<pre-release>+<revision>
			version = fmt.Sprintf("%s-%s%s", noVersion, develPreRelease, revision)
			return
		}
		version = "v" + tag
		// The tag must be prefixed with "v" otherwise the
		// semver library will return an empty string.
		if semver.Build(version) == "" {
			version += revision
		}
	})
	return version
}

// VersionsMatch compares the two versions. It assumes the versions match if
// the major and the minor versions are equivalent. Patch versions are
// disregarded. If it detects that either version is a developer build it
// returns true.
func VersionsMatch(v1, v2 string) bool {
	// If no version is attached, then it is a dev build outside of CI. The version
	// will be disregarded... hopefully they know what they are doing.
	if strings.Contains(v1, noVersion) || strings.Contains(v2, noVersion) {
		return true
	}

	return semver.MajorMinor(v1) == semver.MajorMinor(v2)
}

func IsDevVersion(v string) bool {
	return strings.Contains(v, "-"+develPreRelease)
}

// IsDev returns true if this is a development build.
// CI builds are also considered development builds.
func IsDev() bool {
	return IsDevVersion(Version())
}

// IsSlim returns true if this is a slim build.
func IsSlim() bool {
	return slim
}

// IsAGPL returns true if this is an AGPL build.
func IsAGPL() bool {
	return strings.Contains(agpl, "t")
}

func IsBoringCrypto() bool {
	return boringcrypto
}

// ExternalURL returns a URL referencing the current Coder version.
// For production builds, this will link directly to a release.
// For development builds, this will link to a commit.
func ExternalURL() string {
	readExternalURL.Do(func() {
		repo := "https://github.com/coder/coder"
		revision, valid := revision()
		if !valid {
			externalURL = repo
			return
		}
		externalURL = fmt.Sprintf("%s/commit/%s", repo, revision)
	})
	return externalURL
}

// Time returns when the Git revision was published.
func Time() (time.Time, bool) {
	value, valid := find("vcs.time")
	if !valid {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic("couldn't parse time: " + err.Error())
	}
	return parsed, true
}

// revision returns the Git hash of the build.
func revision() (string, bool) {
	return find("vcs.revision")
}

// find panics if a setting with the specific key was not
// found in the build info.
func find(key string) (string, bool) {
	readBuildInfo.Do(func() {
		buildInfo, buildInfoValid = debug.ReadBuildInfo()
	})
	if !buildInfoValid {
		panic("couldn't read build info")
	}
	for _, setting := range buildInfo.Settings {
		if setting.Key != key {
			continue
		}
		return setting.Value, true
	}
	return "", false
}
