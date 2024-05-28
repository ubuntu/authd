// Package cachetestutils export cache test functionalities used by other packages.
package cachetestutils

import (
	"io"
	"os"
	"testing"
	//nolint:revive,nolintlint // needed for go:linkname, but only used in tests. nolinlint as false positive then.
	_ "unsafe"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testsdetection"
	"github.com/ubuntu/authd/internal/users/cache"
)

func init() {
	// No import outside of testing environment.
	testsdetection.MustBeTesting()
}

var (
	// DbName is database name exported for tests
	//go:linkname DbName github.com/ubuntu/authd/internal/users/cache.dbName
	DbName string
)

// DumpToYaml deserializes the cache database as a string in yaml format.
//
//go:linkname DumpToYaml github.com/ubuntu/authd/internal/users/cache.(*Cache).dumpToYaml
func DumpToYaml(c *cache.Cache) (string, error)

// DbfromYAML loads a yaml formatted of the buckets from a reader and dump it into destDir, with its dbname.
//
//go:linkname DbfromYAML github.com/ubuntu/authd/internal/users/cache.dbfromYAML
func DbfromYAML(r io.Reader, destDir string) error

// CreateDBFromYAML creates the database inside destDir and loads the src file content into it.
func CreateDBFromYAML(t *testing.T, src, destDir string) {
	t.Helper()

	f, err := os.Open(src)
	require.NoError(t, err, "Setup: should be able to read source file")
	defer f.Close()

	err = DbfromYAML(f, destDir)
	require.NoError(t, err, "Setup: should be able to write database file")
}
