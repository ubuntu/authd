// Package users support all common action on the system for user handling.
package users

// UserInfo is the user information returned by the broker.
type UserInfo struct {
	Name  string
	UID   int
	Gecos string
	Dir   string
	Shell string

	Groups []GroupInfo
}

// GroupInfo is the group information returned by the broker.
type GroupInfo struct {
	Name string
	GID  *int
}
