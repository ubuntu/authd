package authorizer

// All those functions and methods are only for tests.
// They are not exported, and guarded by testing assertions.

import (
	"fmt"
	"os/user"
	"strconv"

	"github.com/ubuntu/authd/internal/testsdetection"
)

// withCurrentUserAsRoot returns an Option that sets the rootUID to the current user's UID.
func withCurrentUserAsRoot() Option {
	testsdetection.MustBeTesting()

	uid := currentUserUID()
	return func(o *options) {
		o.rootUID = uid
	}
}

// currentUserUID returns the current user UID or panics.
func currentUserUID() uint32 {
	testsdetection.MustBeTesting()

	u, err := user.Current()
	if err != nil {
		panic(fmt.Sprintf("could not get current user: %v", err))
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		panic(fmt.Sprintf("current uid is not an int (%v): %v", u.Uid, err))
	}

	return uint32(uid)
}

// setCurrentRootAsRoot mutates a default permission to the current user's UID if currentUserAsRoot is true.
//
//nolint:unused // false positive as used in authorizertests with linkname.
func (a *Authorizer) setCurrentRootAsRoot(currentUserAsRoot bool) {
	testsdetection.MustBeTesting()

	if !currentUserAsRoot {
		a.rootUID = defaultOptions.rootUID
		return
	}

	a.rootUID = currentUserUID()
}
