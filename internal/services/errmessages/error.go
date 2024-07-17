package errmessages

// ErrToDisplay defines an error that needs to be sent unaltered to the client.
type ErrToDisplay struct {
	error
}

// NewErrorToDisplay returns a new ErrorToDisplay.
func NewErrorToDisplay(err error) error {
	return ErrToDisplay{err}
}
