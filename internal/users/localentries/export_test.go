package localentries

import userslocking "github.com/ubuntu/authd/internal/users/locking"

// WithGroupPath overrides the default /etc/group path for tests.
func WithGroupPath(p string) Option {
	return func(o *options) {
		o.groupInputPath = p
		o.groupOutputPath = p
	}
}

// WithGroupInputPath overrides the default /etc/group path for input in tests.
func WithGroupInputPath(p string) Option {
	return func(o *options) {
		o.groupInputPath = p
	}
}

// WithGroupOutputPath overrides the default /etc/group path for output in tests.
func WithGroupOutputPath(p string) Option {
	return func(o *options) {
		o.groupOutputPath = p
	}
}

// WithMockUserDBLocking uses a mock implementation to lock the users database.
func WithMockUserDBLocking() Option {
	return func(o *options) {
		mock := userslocking.SimpleMock{}
		o.writeLockFunc = mock.WriteLock
		o.writeUnlockFunc = mock.WriteUnlock
	}
}

// WithUserDBLockedInstance allows to use a test-provided locked instance that
// can be used to verify the refcounting behavior instead of relying on the
// default instance that is used normally.
func WithUserDBLockedInstance(userDBLocked *UserDBLocked) Option {
	return func(o *options) {
		o.userDBLocked = userDBLocked
	}
}

// GroupFileBackupPath exposes the path to the group file backup for testing.
func GroupFileBackupPath(groupFilePath string) string {
	return groupFileBackupPath(groupFilePath)
}
