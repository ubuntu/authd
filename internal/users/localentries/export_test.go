package localentries

import "github.com/ubuntu/authd/internal/users/types"

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

// GroupFileBackupPath exposes the path to the group file backup for testing.
func GroupFileBackupPath(groupFilePath string) string {
	return groupFileBackupPath(groupFilePath)
}

// ValidateChangedGroups validates the new groups given the current, changed and new groups.
func ValidateChangedGroups(currentGroups, newGroups []types.GroupEntry) error {
	return validateChangedGroups(currentGroups, newGroups)
}
