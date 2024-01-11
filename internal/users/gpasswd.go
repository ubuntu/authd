//go:build !integrationtests

package users

var defaultOptions options

func init() {
	defaultOptions = options{
		groupPath:    "/etc/group",
		gpasswdCmd:   []string{"gpasswd"},
		getUsersFunc: getPasswdUsernames,
	}
}
