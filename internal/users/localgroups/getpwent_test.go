package localgroups

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetPasswdUsernames(t *testing.T) {
	t.Parallel()

	got, err := getPasswdUsernames()
	require.NoError(t, err, "GetPasswdUsernames should not return an error")
	require.NotEmpty(t, got, "GetPasswdUsernames should never return an empty list")
	require.Contains(t, got, "root", "GetPasswdUsernames should always return root")
}
