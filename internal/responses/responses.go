// Package responses contains the possible responses for the authentication.
package responses

const (
	// AuthAllowed is the response when the authentication is allowed.
	AuthAllowed = "allowed"
	// AuthDenied is the response when the authentication is denied.
	AuthDenied = "denied"
	// AuthCancelled is the response when the authentication is cancelled.
	AuthCancelled = "cancelled"
)

// AuthReplies is the list of all possible authentication replies.
var AuthReplies = []string{AuthAllowed, AuthDenied, AuthCancelled}
