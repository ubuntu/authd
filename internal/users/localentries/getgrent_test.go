package localentries

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetGroupEntries(t *testing.T) {
	t.Parallel()

	got, err := GetGroupEntries()
	require.NoError(t, err, "GetGroupEntries should never return an error")
	require.NotEmpty(t, got, "GetGroupEntries should never return an empty list")
}

func TestGetGroupByName(t *testing.T) {
	t.Parallel()

	got, err := GetGroupByName("root")
	require.NoError(t, err, "GetGroupByName should not return an error")
	require.Equal(t, got.Name, "root")
	require.Equal(t, got.GID, uint32(0))
}

func TestGetGroupByName_NotFound(t *testing.T) {
	t.Parallel()

	got, err := GetGroupByName("nonexistent")
	require.ErrorIs(t, err, ErrGroupNotFound)
	require.Equal(t, got.Name, "")
}
