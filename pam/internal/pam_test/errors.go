package pam_test

import "github.com/msteinert/pam/v2"

// ErrorTest is like pam.Error but we redefine some hopefully unused errors to values for testing purposes.
type ErrorTest pam.Error

const (
	// ErrInvalid is an invalid error value.
	ErrInvalid = pam.ErrAbort

	// ErrInvalidMethod is used on invalid method calls.
	ErrInvalidMethod = pam.ErrCredInsufficient

	// ErrReturnMismatch is used on unexpected return values.
	ErrReturnMismatch = pam.ErrCred

	// ErrInvalidArguments is used on invalid arguments.
	ErrInvalidArguments = pam.ErrAuthtokDisableAging

	// ErrArgumentTypeMismatch is used on invalid arguments types.
	ErrArgumentTypeMismatch = pam.ErrAuthtokLockBusy
)
