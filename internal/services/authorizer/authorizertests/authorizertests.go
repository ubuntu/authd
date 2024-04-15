// Package authorizertests are exported functions to be run in 3rd party package or integration tests.
package authorizertests

//nolint:gci // We import unsafe as it is needed for go:linkname, but the nolint comment confuses gofmt and it adds
// a blank space between the imports, which creates problems with gci so we need to ignore it.
import (
	"fmt"
	"regexp"
	"strings"

	//nolint:revive,nolintlint // needed for go:linkname, but only used in tests. nolintlint as false positive then.
	_ "unsafe"

	"github.com/ubuntu/authd/internal/services/authorizer"
	"github.com/ubuntu/authd/internal/testsdetection"
)

// WithCurrentUserAsRoot returns an Option that sets the rootUID to the current user's UID.
//
//go:linkname WithCurrentUserAsRoot github.com/ubuntu/authd/internal/services/authorizer.withCurrentUserAsRoot
func WithCurrentUserAsRoot() authorizer.Option

// SetCurrentRootAsRoot mutates a default permission to the current user's UID if currentUserAsRoot is true.
//
//go:linkname SetCurrentRootAsRoot github.com/ubuntu/authd/internal/services/authorizer.(*Authorizer).setCurrentRootAsRoot
func SetCurrentRootAsRoot(a *authorizer.Authorizer, currentUserAsRoot bool)

/*
 * Integration tests helpers
 */

//go:linkname defaultOptions github.com/ubuntu/authd/internal/services/authorizer.defaultOptions
var defaultOptions struct {
	rootUID uint32
}

//go:linkname currentUserUID github.com/ubuntu/authd/internal/services/authorizer.currentUserUID
func currentUserUID() uint32

// DefaultCurrentUserAsRoot mocks the current user as root for the authorizer.
func DefaultCurrentUserAsRoot() {
	testsdetection.MustBeTesting()

	defaultOptions.rootUID = currentUserUID()
}

//go:linkname permErrorFmt github.com/ubuntu/authd/internal/services/authorizer.permErrorFmt
var permErrorFmt string

// IdempotentPermissionError strips the UID from the permission error message.
func IdempotentPermissionError(msg string) string {
	testsdetection.MustBeTesting()

	// We assume a known format error and we will capture change during the tests.
	// The issue is that golden files assert on the errors, that should not be the case ideally.
	permErrorRaw := strings.TrimSuffix(permErrorFmt, "%d")
	permErrorFmt := fmt.Sprintf(`%s\d+`, permErrorRaw)
	re := regexp.MustCompile(permErrorFmt)
	return re.ReplaceAllString(msg, fmt.Sprintf("%sXXXX", permErrorRaw))
}
