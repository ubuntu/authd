package db

// All those functions and methods are only for tests.
// They are not exported, and guarded by testing assertions.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/ubuntu/authd/internal/testsdetection"
	"github.com/ubuntu/authd/log"
	"gopkg.in/yaml.v3"
)

// Z_ForTests_DumpNormalizedYAML gets the content of the database, normalizes it
// (so that it can be compared with a golden file) and returns it as a YAML string.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_DumpNormalizedYAML(c *Manager) (string, error) {
	testsdetection.MustBeTesting()

	// Get all users
	users, err := allUsers(c.db)
	if err != nil {
		return "", err
	}

	// Sort the users by UID.
	sort.Slice(users, func(i, j int) bool {
		return users[i].UID < users[j].UID
	})

	// Get all groups
	groups, err := allGroups(c.db)
	if err != nil {
		return "", err
	}

	// Sort the groups by GID.
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].GID < groups[j].GID
	})

	// Get all rows from the users_to_groups table.
	userGroups, err := allUserGroupsInternal(c.db)
	if err != nil {
		return "", err
	}

	// Sort the userGroups by UID.
	sort.Slice(userGroups, func(i, j int) bool {
		if userGroups[i].UID == userGroups[j].UID {
			return userGroups[i].GID < userGroups[j].GID
		}
		return userGroups[i].UID < userGroups[j].UID
	})

	// Get the schema version
	schemaVersion, err := getSchemaVersion(c.db)
	if err != nil {
		return "", err
	}

	content := struct {
		Users         []UserRow        `yaml:"users"`
		Groups        []GroupRow       `yaml:"groups"`
		UsersToGroups []userToGroupRow `yaml:"users_to_groups"`
		SchemaVersion int              `yaml:"schema_version"`
	}{
		Users:         users,
		Groups:        groups,
		UsersToGroups: userGroups,
		SchemaVersion: schemaVersion,
	}

	// Marshal the content into a YAML string.
	yamlData, err := yaml.Marshal(content)
	if err != nil {
		return "", err
	}

	return string(yamlData), nil
}

// Z_ForTests_CreateDBFromYAML creates the bbolt database inside destDir and loads the src file content into it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_CreateDBFromYAML(src, destDir string) error {
	testsdetection.MustBeTesting()

	src, err := filepath.Abs(src)
	if err != nil {
		return err
	}

	log.Debugf(context.Background(), "Loading SQLite database from %s", src)

	f, err := os.Open(src)
	if err != nil {
		return err
	}

	return createDBFromYAMLReader(f, destDir)
}

// Z_ForTests_CreateDBFromYAML creates the bbolt database inside destDir and loads the src file content into it.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_CreateDBFromYAMLReader(r io.Reader, destDir string) error {
	return createDBFromYAMLReader(r, destDir)
}

func createDBFromYAMLReader(r io.Reader, destDir string) (err error) {
	yamlData, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// unmarshal the content into a map.
	dbContent := make(map[string]any)
	err = yaml.Unmarshal(yamlData, dbContent)
	if err != nil {
		return err
	}

	db, err := New(destDir)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	tablesInOrder := []string{"users", "groups", "users_to_groups", "schema_version"}

	// Insert data
	for _, table := range tablesInOrder {
		tableContent, exists := dbContent[table]
		if !exists {
			continue
		}

		if table == "schema_version" {
			log.Debugf(context.Background(), "Setting schema version to %v", tableContent)
			query := "UPDATE schema_version SET version = ?"
			_, err = db.db.Exec(query, tableContent)
			if err != nil {
				return err
			}
			continue
		}

		// All other tables are expected to be slices of maps.
		temp, ok := tableContent.([]any)
		if !ok {
			return fmt.Errorf("unexpected type for table %s: %T, content: %v", table, tableContent, tableContent)
		}

		records := make([]map[string]any, len(temp))
		for i, v := range temp {
			record, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("unexpected type in table %s: %T, content: %v", table, v, v)
			}
			records[i] = record
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
			log.Debugf(context.Background(), "Inserting into %s: %+v", table, vals)

			//nolint:gosec // We don't care about SQL injection in our tests.
			query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, columns, values)
			_, err = db.db.Exec(query, vals...)
			if err != nil {
				return err
			}
		}
	}

	log.Debug(context.Background(), "Database created")
	return nil
}

// Z_ForTests_GetGroupFile returns the path to the group file.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_GetGroupFile() string {
	testsdetection.MustBeTesting()

	return groupFile
}

// Z_ForTests_SetGroupFile sets the group file to the provided path.
//
// nolint:revive,nolintlint // We want to use underscores in the function name here.
func Z_ForTests_SetGroupFile(groupFilePath string) {
	groupFile = groupFilePath
}
