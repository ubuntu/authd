package cache_test

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
	_ "unsafe"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/cache"
	"github.com/ubuntu/authd/internal/testutils"
)

func TestNew(t *testing.T) {
	t.Parallel()

	perm0644 := os.FileMode(0644)
	perm0000 := os.FileMode(0000)

	tests := map[string]struct {
		dbFile string
		dirty  bool
		perm   *fs.FileMode

		wantErr bool
	}{
		"New without any initialized database":    {},
		"New with already existing database":      {dbFile: "multiple_users_and_groups"},
		"Database flagged as dirty is cleared up": {dbFile: "multiple_users_and_groups", dirty: true},

		"Error on cacheDir non existent cacheDir":      {dbFile: "-", wantErr: true},
		"Error on invalid permission on database file": {dbFile: "multiple_users_and_groups", perm: &perm0644, wantErr: true},
		"Error on unreadable database file":            {dbFile: "multiple_users_and_groups", perm: &perm0000, wantErr: true},
	}
	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			dbDestPath := filepath.Join(cacheDir, cache.DbName)

			if tc.dbFile == "-" {
				err := os.RemoveAll(cacheDir)
				require.NoError(t, err, "Setup: could not remove temporary cache directory")
			} else if tc.dbFile != "" {
				createDBFile(t, filepath.Join("testdata", tc.dbFile+".db.yaml"), cacheDir)
			}
			if tc.dirty {
				err := os.WriteFile(filepath.Join(cacheDir, cache.DirtyFlagDbName), nil, 0600)
				require.NoError(t, err, "Setup: could not create dirty flag file")
			}
			if tc.perm != nil {
				err := os.Chmod(dbDestPath, *tc.perm)
				require.NoError(t, err, "Setup: could not change mode of database file")
			}

			c, err := cache.New(cacheDir)
			if tc.wantErr {
				require.Error(t, err, "New should return an error but didn't")
				return
			}
			require.NoError(t, err)
			defer c.Close()

			if tc.dirty {
				// Let the cache cleanup start proceeding
				time.Sleep(200 * time.Millisecond)
			}

			got, err := dumpToYaml(c)
			require.NoError(t, err, "Created database should be valid yaml content")

			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "Did not get expected database content")

			// check database permission
			fileInfo, err := os.Stat(dbDestPath)
			require.NoError(t, err, "Failed to stat database")
			perm := fileInfo.Mode().Perm()
			require.Equal(t, fs.FileMode(0600), perm, "Database permission should be 0600")

			// database should not be marked as dirty
			_, err = os.Stat(filepath.Join(cacheDir, cache.DirtyFlagDbName))
			require.ErrorIs(t, err, fs.ErrNotExist, "Dirty flag should have been removed")
		})
	}
}

func createDBFile(t *testing.T, src, destDir string) {
	t.Helper()

	f, err := os.Open(src)
	require.NoError(t, err, "Setup: should be able to read source file")
	defer f.Close()

	err = dbfromYAML(f, destDir)
	require.NoError(t, err, "Setup: should be able to write database file")
}

//go:linkname dumpToYaml github.com/ubuntu/authd/internal/cache.(*Cache).dumpToYaml
func dumpToYaml(c *cache.Cache) (string, error)

//go:linkname dbfromYAML github.com/ubuntu/authd/internal/cache.dbfromYAML
func dbfromYAML(r io.Reader, destDir string) error

func TestMain(m *testing.M) {
	testutils.InstallUpdateFlag()

	os.Exit(m.Run())
}
