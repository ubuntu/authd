//go:build !integrationtests

package localgroups

var defaultOptions options

func init() {
	defaultOptions = options{
		groupPath:    "/etc/group",
		gpasswdCmd:   []string{"gpasswd"},
		getUsersFunc: getPasswdUsernames,
	}
}
