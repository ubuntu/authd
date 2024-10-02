package localentries

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetPasswdEntries(t *testing.T) {
	t.Parallel()

	got := GetPasswdEntries()
	require.NotEmpty(t, got, "GetPasswdEntries should never return an empty list")

	// Check if the root user is present in the list
	var rootFound bool
	for _, entry := range got {
		if entry.Name == "root" {
			rootFound = true
			break
		}
	}
	require.True(t, rootFound, "GetPasswdEntries should always return root")
}

func TestGetGroupEntries(t *testing.T) {
	t.Parallel()

	got := GetGroupEntries()
	require.NotEmpty(t, got, "GetGroupEntries should never return an empty list")
}
