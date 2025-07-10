package localentries

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/users/types"
)

func TestGetPasswdEntries(t *testing.T) {
	t.Parallel()

	for idx := range 10 {
		t.Run(fmt.Sprintf("iteration_%d", idx), func(t *testing.T) {
			t.Parallel()

			got, err := getUserEntries()
			require.NoError(t, err, "GetPasswdEntries should never return an error")
			require.NotEmpty(t, got, "GetPasswdEntries should never return an empty list")

			// Check if the root user is present in the list
			rootFound := slices.ContainsFunc(got, func(entry types.UserEntry) bool {
				return entry.Name == "root" && entry.UID == 0 && entry.GID == 0
			})
			require.True(t, rootFound, "GetPasswdEntries should always return root")
		})
	}
}

func TestGetPasswdByName(t *testing.T) {
	t.Parallel()

	for idx := range 10 {
		t.Run(fmt.Sprintf("iteration_%d", idx), func(t *testing.T) {
			t.Parallel()

			got, err := GetPasswdByName("root")
			require.NoError(t, err, "GetPasswdByName should not return an error")
			require.Equal(t, "root", got.Name, "Name does not match")
			require.Equal(t, uint32(0), got.UID, "UID does not match")
			require.Equal(t, uint32(0), got.GID, "GID does not match")
			require.Equal(t, "root", got.Gecos, "Gecos does not match")
			require.NotEmpty(t, got.Shell, "Shell is not empty")
			require.Equal(t, "/root", got.Dir, "Dir does not match")
		})
	}
}

func TestGetPasswdByName_NotFound(t *testing.T) {
	t.Parallel()

	for idx := range 10 {
		t.Run(fmt.Sprintf("iteration_%d", idx), func(t *testing.T) {
			t.Parallel()

			got, err := GetPasswdByName(fmt.Sprintf("nonexistent-really-%d", idx))
			require.ErrorIs(t, err, ErrUserNotFound)
			require.Empty(t, got, "Entry should be empty, but is not")
		})
	}
}
