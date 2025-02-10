package db

// All those functions and methods are only for tests.
// They are not exported, and guarded by testing assertions.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ubuntu/authd/internal/testsdetection"
	"github.com/ubuntu/authd/log"
	"gopkg.in/yaml.v3"
)

// Z_ForTests_DumpNormalizedYAML gets the content of the database, normalizes it
// (so that it can be compared with a golden file) and returns it as a YAML string.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_DumpNormalizedYAML(t *testing.T, c *Database) string {
	t.Helper()
	testsdetection.MustBeTesting()

	// Get all users
	users, err := allUsers(c.db)
	require.NoError(t, err)

	// Sort the users by UID.
	sort.Slice(users, func(i, j int) bool {
		return users[i].UID < users[j].UID
	})

	// Get all groups
	groups, err := allGroupsInternal(c.db)
	require.NoError(t, err)

	// Sort the groups by GID.
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].GID < groups[j].GID
	})

	// Get all rows from the users_to_groups table.
	userGroups, err := allUserGroupsInternal(c.db)
	require.NoError(t, err)

	// Sort the userGroups by UID.
	sort.Slice(userGroups, func(i, j int) bool {
		if userGroups[i].UID == userGroups[j].UID {
			return userGroups[i].GID < userGroups[j].GID
		}
		return userGroups[i].UID < userGroups[j].UID
	})

	content := struct {
		Users         []UserDB         `yaml:"users"`
		Groups        []groupRow       `yaml:"groups"`
		UsersToGroups []userToGroupRow `yaml:"users_to_groups"`
	}{
		Users:         users,
		Groups:        groups,
		UsersToGroups: userGroups,
	}

	// Marshal the content into a YAML string.
	yamlData, err := yaml.Marshal(content)
	require.NoError(t, err)

	return string(yamlData)
}

// Z_ForTests_DBName returns the name of the database.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_DBName() string {
	testsdetection.MustBeTesting()
	return filename
}

// Z_ForTests_CreateDBFromYAML creates the bbolt database inside destDir and loads the src file content into it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_CreateDBFromYAML(t *testing.T, src, destDir string) {
	t.Helper()
	testsdetection.MustBeTesting()

	src, err := filepath.Abs(src)
	require.NoError(t, err)

	log.Debugf(context.Background(), "Loading SQLite database from %s", src)

	f, err := os.Open(src)
	require.NoError(t, err)

	createDBFromYAMLReader(t, f, destDir)
}

// Z_ForTests_CreateDBFromYAML creates the bbolt database inside destDir and loads the src file content into it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_CreateDBFromYAMLReader(t *testing.T, r io.Reader, destDir string) {
	t.Helper()
	createDBFromYAMLReader(t, r, destDir)
}

func createDBFromYAMLReader(t *testing.T, r io.Reader, destDir string) {
	t.Helper()

	yamlData, err := io.ReadAll(r)
	require.NoError(t, err)

	// unmarshal the content into a map.
	dbContent := make(map[string][]map[string]string)
	err = yaml.Unmarshal(yamlData, dbContent)
	require.NoError(t, err)

	db, err := New(destDir)
	require.NoError(t, err)
	defer func() {
		err := db.Close()
		require.NoError(t, err)
	}()

	tablesInOrder := []string{"users", "groups", "users_to_groups"}

	// Insert data
	for _, table := range tablesInOrder {
		records, exists := dbContent[table]
		if !exists {
			continue
		}

		for _, record := range records {
			columns := ""
			values := ""
			var vals []any

			for col, val := range record {
				if columns != "" {
					columns += ", "
					values += ", "
				}
				columns += col
				values += "?"
				vals = append(vals, val)
			}
			log.Debugf(context.Background(), "Inserting into %s: %s", table, vals)

			//nolint:gosec // We don't care about SQL injection in our tests.
			query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, columns, values)
			_, err = db.db.Exec(query, vals...)
			require.NoError(t, err)
		}
	}

	log.Debug(context.Background(), "Database created")
}
