package users

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetPasswdUsernames(t *testing.T) {
	t.Parallel()

	got := getPasswdUsernames()
	require.NotEmpty(t, got, "GetPasswdUsernames should never return an empty list")
	require.Contains(t, got, "root", "GetPasswdUsernames should always return root")
}
