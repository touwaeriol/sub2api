package plugin

import (
	"fmt"

	"golang.org/x/mod/semver"
)

// SDKVersion is the semver of the plugin SDK that the host is built with.
//
// Plugins declare the SDK version they require via Meta.APIVersion; the host
// compares the two with [CheckAPIVersion]. When adding backwards-compatible
// surface (new interface, new optional method), bump the minor. When making a
// breaking change, bump the major.
const SDKVersion = "0.1.0"

// sdkVersionPrefixed is SDKVersion with the "v" prefix that golang.org/x/mod/semver expects.
const sdkVersionPrefixed = "v" + SDKVersion

// CheckAPIVersion reports whether a plugin declaring the given required SDK
// version is compatible with the host's [SDKVersion].
//
// Compatibility rule:
//   - major versions must be identical
//   - the host's minor must be >= the plugin's required minor
//
// The patch component is ignored. A plugin requiring v0.1.5 runs fine on host
// SDK v0.1.0 in the 0.x era because 0.x is treated as "any minor works as long
// as major matches" — we still gate minor-up to allow plugins to opt into
// newer surface deterministically.
func CheckAPIVersion(required string) error {
	if required == "" {
		return fmt.Errorf("%w: plugin did not declare APIVersion", ErrAPIVersionIncompat)
	}
	req := required
	if req[0] != 'v' {
		req = "v" + req
	}
	if !semver.IsValid(req) {
		return fmt.Errorf("%w: invalid APIVersion %q", ErrAPIVersionIncompat, required)
	}
	if semver.Major(req) != semver.Major(sdkVersionPrefixed) {
		return fmt.Errorf("%w: plugin requires %s, host provides %s (major mismatch)",
			ErrAPIVersionIncompat, required, SDKVersion)
	}
	// Host minor must be >= required minor.
	if semver.Compare(semver.MajorMinor(sdkVersionPrefixed), semver.MajorMinor(req)) < 0 {
		return fmt.Errorf("%w: plugin requires %s, host provides %s (minor too low)",
			ErrAPIVersionIncompat, required, SDKVersion)
	}
	return nil
}
