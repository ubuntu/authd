package main

// Various signalling return messaging to PAM.

// pamSuccess signals PAM module to return PAM_SUCCESS and Quit tea.Model.
type pamSuccess struct {
}

// String returns the string of pamSuccess.
func (err pamSuccess) String() string {
	return ""
}

// pamIgnore signals PAM module to return PAM_IGNORE and Quit tea.Model.
type pamIgnore struct {
	msg string
}

// String returns the string of pamIgnore message.
func (err pamIgnore) String() string {
	return err.msg
}

// pamAbort signals PAM module to return PAM_ABORT and Quit tea.Model.
type pamAbort struct {
	msg string
}

// String returns the string of pamAbort message.
func (err pamAbort) String() string {
	return err.msg
}

// pamSystemError signals PAM module to return PAM_SYSTEM_ERROR and Quit tea.Model.
type pamSystemError struct {
	msg string
}

// String returns the string of pamSystemError message.
func (err pamSystemError) String() string {
	return err.msg
}

// pamAuthError signals PAM module to return PAM_AUTH_ERROR and Quit tea.Model.
type pamAuthError struct {
	msg string
}

// String returns the string of pamAuthError message.
func (err pamAuthError) String() string {
	return err.msg
}
