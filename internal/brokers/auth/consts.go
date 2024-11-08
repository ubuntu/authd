// Package auth contains the authentication related code.
package auth

const (
	// Granted is the response when the authentication is granted.
	Granted = "granted"
	// Denied is the response when the authentication is denied.
	Denied = "denied"
	// Cancelled is the response when the authentication is cancelled.
	Cancelled = "cancelled"
	// Retry is the response when the authentication needs to be retried (another chance).
	Retry = "retry"
	// Next is the response when another MFA (including changing password) authentication is necessary.
	Next = "next"
)

// Replies is the list of all possible authentication replies.
var Replies = []string{Granted, Denied, Cancelled, Retry, Next}

const (
	// SessionModeAuth is the name of the authentication session.
	SessionModeAuth = "auth"
	// SessionModePasswd is the name of the passwd session.
	SessionModePasswd = "passwd"
)
