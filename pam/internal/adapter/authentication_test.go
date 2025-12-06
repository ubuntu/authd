package adapter

import (
	"testing"

	"github.com/msteinert/pam/v2"
	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/brokers/auth"
	"github.com/ubuntu/authd/internal/proto/authd"
)

// When 2FA is required, a password-only granted result should be turned into
// PAM_CRED_INSUFFICIENT.
func TestAuthenticationRequire2FAOnPasswordGranted(t *testing.T) {
	m := newAuthenticationModel(nil, Native, authd.SessionMode_LOGIN, true)
	m.currentAuthMode = "password"

	granted := isAuthenticatedResultReceived{access: auth.Granted}
	_, cmd := m.Update(granted)
	require.NotNil(t, cmd)

	got := cmd()
	pamErr, ok := got.(pamError)
	require.True(t, ok, "expected pamError message, got %T", got)
	require.Equal(t, pam.ErrCredInsufficient, pamErr.Status())
}
