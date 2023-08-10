// Package responses contains the possible responses for the authentication.
package responses

const (
	// AuthAllowed is the response when the authentication is allowed.
	AuthAllowed = "allowed"
	// AuthDenied is the response when the authentication is denied.
	AuthDenied = "denied"
	// AuthCancelled is the response when the authentication is cancelled.
	AuthCancelled = "cancelled"
	// AuthRetry is the response when the authentication needs to be retried (another chance).
	AuthRetry = "retry"
	// AuthNext is the response when another MFA (including changing password) authentication is necessary.
	AuthNext = "next"
)

// AuthReplies is the list of all possible authentication replies.
var AuthReplies = []string{AuthAllowed, AuthDenied, AuthCancelled, AuthRetry, AuthNext}
