package testutils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	cachetests "github.com/ubuntu/authd/internal/newusers/cache/tests"
)

// CreateDBFromYAML creates the database inside destDir and loads the src file content into it.
func CreateDBFromYAML(t *testing.T, src, destDir string) {
	t.Helper()

	f, err := os.Open(src)
	require.NoError(t, err, "Setup: should be able to read source file")
	defer f.Close()

	err = cachetests.DbfromYAML(f, destDir)
	require.NoError(t, err, "Setup: should be able to write database file")
}
