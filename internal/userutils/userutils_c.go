package userutils

/*
#include <shadow.h>
*/
import "C"

import (
	"fmt"

	"github.com/ubuntu/authd/internal/errno"
)

// WriteLockShadowPassword locks for writing the shadow password database by
// using the standard libc lckpwdf() function.
// While the database is locked read operations can happen, but no other process
// is allowed to write.
// Note that this call will block all the other processes trying to access the
// database in write mode, while it will return an error if called while the
// lock is already hold by this process.
func writeLockShadowPassword() error {
	errno.Lock()
	defer errno.Unlock()

	if C.lckpwdf() == 0 {
		return nil
	}

	if err := errno.Get(); err != nil {
		return fmt.Errorf("%w: %w", ErrLock, err)
	}

	return ErrLock
}

// WriteUnlockShadowPassword unlocks for writing the shadow password database by
// using the standard libc ulckpwdf() function.
// As soon as this function is called all the other waiting processes will be
// allowed to take the lock.
func writeUnlockShadowPassword() error {
	errno.Lock()
	defer errno.Unlock()

	if C.ulckpwdf() == 0 {
		return nil
	}

	if err := errno.Get(); err != nil {
		return fmt.Errorf("%w: %w", ErrUnlock, err)
	}

	return ErrUnlock
}
