package localgroups

// WithGroupPath overrides the default /etc/group path for tests.
func WithGroupPath(p string) Option {
	return func(o *options) {
		o.groupPath = p
	}
}

// WithGpasswdCmd overrides gpasswd call with specific commands for tests.
func WithGpasswdCmd(cmds []string) Option {
	return func(o *options) {
		o.gpasswdCmd = cmds
	}
}

// WithGetUsersFunc overrides the getusers func with a custom one for tests.
func WithGetUsersFunc(getUsersFunc func() []string) Option {
	return func(o *options) {
		o.getUsersFunc = getUsersFunc
	}
}
