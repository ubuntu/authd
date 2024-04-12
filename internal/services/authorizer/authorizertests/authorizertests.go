// Package authorizertests are exported functions to be run in 3rd party package or integration tests.
package authorizertests

import (
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
