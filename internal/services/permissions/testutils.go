package permissions

// All those functions and methods are only for tests.
// They are not exported, and guarded by testing assertions.

import (
	"fmt"
	"math"
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
	uid, err := strconv.ParseUint(u.Uid, 10, 0)
	if err != nil || uid > math.MaxUint32 {
		panic(fmt.Sprintf("current uid is not an uint32 (%v): %v", u.Uid, err))
	}

	return uint32(uid)
}

// setCurrentUserAsRoot mutates a default permission to the current user's UID if currentUserAsRoot is true.
//
//nolint:unused // false positive as used in permissionstests with linkname.
func (m *Manager) setCurrentUserAsRoot(currentUserAsRoot bool) {
	testsdetection.MustBeTesting()

	if !currentUserAsRoot {
		m.rootUID = defaultOptions.rootUID
		return
	}

	m.rootUID = currentUserUID()
}
