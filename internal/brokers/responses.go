package brokers

const (
	// AuthAllowed is the response when the authentication is allowed.
	AuthAllowed = "allowed"
	// AuthDenied is the response when the authentication is denied.
	AuthDenied = "denied"
	// AuthCancelled is the response when the authentication is cancelled.
	AuthCancelled = "cancelled"
)

// authReplies is the list of all possible authentication replies.
var authReplies = []string{AuthAllowed, AuthDenied, AuthCancelled}
