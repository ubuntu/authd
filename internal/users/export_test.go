package users

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
