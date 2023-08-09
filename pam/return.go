package main

// Various signalling return messaging to PAM.

// ExitMsger is the exit message type and optional accompagning details.
type ExitMsger interface {
	ExitMsg() string
}

// pamSuccess signals PAM module to return PAM_SUCCESS and Quit tea.Model.
type pamSuccess struct {
}

// ExitMsg returns the string of pamSuccess.
func (err pamSuccess) ExitMsg() string {
	return ""
}

// pamIgnore signals PAM module to return PAM_IGNORE and Quit tea.Model.
type pamIgnore struct {
	msg string
}

// ExitMsg returns the string of pamIgnore message.
func (err pamIgnore) ExitMsg() string {
	return err.msg
}

// pamAbort signals PAM module to return PAM_ABORT and Quit tea.Model.
type pamAbort struct {
	msg string
}

// ExitMsg returns the string of pamAbort message.
func (err pamAbort) ExitMsg() string {
	return err.msg
}

// pamSystemError signals PAM module to return PAM_SYSTEM_ERROR and Quit tea.Model.
type pamSystemError struct {
	msg string
}

// ExitMsg returns the string of pamSystemError message.
func (err pamSystemError) ExitMsg() string {
	return err.msg
}

// pamAuthError signals PAM module to return PAM_AUTH_ERROR and Quit tea.Model.
type pamAuthError struct {
	msg string
}

// ExitMsg returns the string of pamAuthError message.
func (err pamAuthError) ExitMsg() string {
	return err.msg
}
