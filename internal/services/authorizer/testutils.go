package authorizer

// All those functions and methods are only for tests.
// They are not exported, and guarded by testing assertions.

import (
	"fmt"
	"os/user"
	"strconv"

	"github.com/ubuntu/authd/internal/testsdetection"
)

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
