package localentries

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetPasswdEntries(t *testing.T) {
	t.Parallel()

	got, err := GetPasswdEntries()
	require.NoError(t, err, "GetPasswdEntries should never return an error")
	require.NotEmpty(t, got, "GetPasswdEntries should never return an empty list")

	// Check if the root user is present in the list
	rootFound := slices.ContainsFunc(got, func(entry Passwd) bool {
		return entry.Name == "root"
	})
	require.True(t, rootFound, "GetPasswdEntries should always return root")
}

func TestGetPasswdByName(t *testing.T) {
	t.Parallel()

	got, err := GetPasswdByName("root")
	require.NoError(t, err, "GetPasswdByName should not return an error")
	require.Equal(t, got.Name, "root")
	require.Equal(t, got.UID, uint32(0))
}

func TestGetPasswdByName_NotFound(t *testing.T) {
	t.Parallel()

	got, err := GetPasswdByName("nonexistent")
	require.ErrorIs(t, err, ErrUserNotFound)
	require.Equal(t, got.Name, "")
}
