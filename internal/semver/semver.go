// Package semver is a wrapper around golang.org/x/mod/semver which prefixes the version string with a "v" before
// calling the underlying functions.
package semver

import "golang.org/x/mod/semver"

// IsValid checks if the version string is a valid semantic version.
func IsValid(v string) bool {
	return semver.IsValid("v" + v)
}

// Compare compares two semantic versions.
func Compare(v1, v2 string) int {
	return semver.Compare("v"+v1, "v"+v2)
}
