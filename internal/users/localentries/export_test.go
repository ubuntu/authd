package localentries

import "github.com/ubuntu/authd/internal/users/types"

// WithGroupPath overrides the default /etc/group path for tests.
func WithGroupPath(p string) Option {
	return func(o *options) {
		o.inputGroupFile = p
		o.outputGroupFile = p
	}
}

// WithGroupInputPath overrides the default /etc/group path for input in tests.
func WithGroupInputPath(p string) Option {
	return func(o *options) {
		o.inputGroupFile = p
	}
}

// WithGroupOutputPath overrides the default /etc/group path for output in tests.
func WithGroupOutputPath(p string) Option {
	return func(o *options) {
		o.outputGroupFile = p
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

func ParseLocalGroups(groupFilePath string) ([]types.GroupEntry, error) {
	return parseLocalGroups(groupFilePath)
}
