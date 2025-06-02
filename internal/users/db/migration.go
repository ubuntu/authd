package db

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubuntu/authd/internal/fileutils"
	"github.com/ubuntu/authd/internal/users/db/bbolt"
	"github.com/ubuntu/authd/internal/users/localentries"
	userslocking "github.com/ubuntu/authd/internal/users/locking"
	"github.com/ubuntu/authd/log"
)

var groupFile = localentries.GroupFile

// MigrateFromBBoltToSQLite migrates data from bbolt to SQLite.
func MigrateFromBBoltToSQLite(dbDir string) error {
	m, err := New(dbDir)
	if err != nil {
		return err
	}
	defer func() {
		err := m.Close()
		if err != nil {
			log.Warningf(context.Background(), "Failed to close database after migration: %v", err)
		}
	}()

	err = m.migrateFromBBoltToSQLite(dbDir)
	if err != nil {
		return err
	}

	// Apply schema migrations after the migration to SQLite.
	// The call to `New` above created the database with the current schema version,
	// so we need to set the schema version to 0 first.
	if err := setSchemaVersion(m.db, 0); err != nil {
		return fmt.Errorf("failed to reset schema version: %w", err)
	}
	if err := m.maybeApplyMigrations(); err != nil {
		return err
	}

	return nil
}

func (m *Manager) migrateFromBBoltToSQLite(dbDir string) (err error) {
	log.Infof(context.Background(), "Migrating data from bbolt to SQLite")

	// Open the bbolt database.
	bboltDB, err := bbolt.New(dbDir)
	if err != nil {
		return err
	}
	defer func() {
		err := bboltDB.Close()
		if err != nil {
			log.Warningf(context.Background(), "Failed to close bbolt database: %v", err)
		}
	}()

	// Use transaction to ensure that all data is migrated or none at all
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// Ensure the transaction is committed or rolled back
	defer func() {
		err = commitOrRollBackTransaction(err, tx)
	}()

	// Migrate users
	bboltUsers, err := bboltDB.AllUsers()
	if err != nil {
		return err
	}

	for _, u := range bboltUsers {
		brokerID, err := bboltDB.BrokerForUser(u.Name)
		if err != nil {
			return err
		}

		user := UserRow{
			Name:     u.Name,
			UID:      u.UID,
			GID:      u.GID,
			Gecos:    u.Gecos,
			Dir:      u.Dir,
			Shell:    u.Shell,
			BrokerID: brokerID,
		}

		log.Debugf(context.Background(), "Migrating user %v", user.Name)
		if err := insertUser(tx, user); err != nil {
			return err
		}
	}

	// Migrate groups
	bboltGroups, err := bboltDB.AllGroups()
	if err != nil {
		return err
	}

	for _, g := range bboltGroups {
		group := GroupRow{
			Name: g.Name,
			GID:  g.GID,
			UGID: g.UGID,
		}

		log.Debugf(context.Background(), "Migrating group %v", group.Name)
		if err := insertGroup(tx, group); err != nil {
			return err
		}
	}

	// Add users to groups
	for _, u := range bboltUsers {
		groups, err := bboltDB.UserGroups(u.UID)
		if errors.Is(err, bbolt.NoDataFoundError{}) {
			continue
		}
		if err != nil {
			return err
		}

		for _, g := range groups {
			if err := addUserToGroup(tx, u.UID, g.GID); err != nil {
				return err
			}
		}
	}

	// Add user to local groups
	for _, u := range bboltUsers {
		localGroups, err := bboltDB.UserLocalGroups(u.UID)
		if errors.Is(err, bbolt.NoDataFoundError{}) {
			continue
		}
		if err != nil {
			return err
		}

		for _, g := range localGroups {
			if err := addUserToLocalGroup(tx, u.UID, g); err != nil {
				return err
			}
		}
	}

	log.Debug(context.Background(), "Done migrating data from bbolt to SQLite")
	return nil
}

type schemaMigration struct {
	description string
	migrate     func(*Manager) error
}

var schemaMigrations = []schemaMigration{
	{
		description: "Migrate to lowercase user and group names",
		migrate: func(m *Manager) error {
			// Start a transaction to ensure atomicity
			tx, err := m.db.Begin()

			// Ensure the transaction is committed or rolled back
			defer func() {
				err = commitOrRollBackTransaction(err, tx)
			}()

			users, err := allUsers(tx)
			if err != nil {
				return fmt.Errorf("failed to get users from database: %w", err)
			}

			var oldNames, newNames []string
			for _, u := range users {
				oldNames = append(oldNames, u.Name)
				newNames = append(newNames, strings.ToLower(u.Name))
			}

			if err := renameUsersInGroupFile(oldNames, newNames); err != nil {
				return fmt.Errorf("failed to rename users in %s file: %w",
					groupFile, err)
			}

			// Delete groups that would cause unique constraint violations
			if err := removeGroupsWithNameConflicts(tx); err != nil {
				return fmt.Errorf("failed to remove groups with name conflicts: %w", err)
			}

			query := `UPDATE users SET name = LOWER(name);
					  UPDATE groups SET ugid = LOWER(ugid) WHERE ugid = name;
					  UPDATE groups SET name = LOWER(name);`
			_, err = tx.Exec(query)
			return err
		},
	},
}

func (m *Manager) maybeApplyMigrations() error {
	currentVersion, err := getSchemaVersion(m.db)
	if err != nil {
		return err
	}

	if currentVersion >= len(schemaMigrations) {
		return nil
	}

	log.Debugf(context.Background(), "Schema version before migrations: %d", currentVersion)

	v := 0
	for _, migration := range schemaMigrations {
		v++
		if currentVersion >= v {
			continue
		}

		log.Infof(context.Background(), "Applying schema migration: %s", migration.description)
		if err := migration.migrate(m); err != nil {
			return fmt.Errorf("error applying schema migration: %w", err)
		}

		if err := setSchemaVersion(m.db, v); err != nil {
			return fmt.Errorf("failed to update schema version: %w", err)
		}
	}

	log.Debugf(context.Background(), "Schema version after migrations: %d", v)

	return nil
}

func groupFileTemporaryPath() string {
	return fmt.Sprintf("%s+", groupFile)
}

func groupFileBackupPath() string {
	return fmt.Sprintf("%s-", groupFile)
}

// renameUsersInGroupFile renames users in the /etc/group file.
func renameUsersInGroupFile(oldNames, newNames []string) error {
	log.Debugf(context.Background(), "Renaming users in %q: %v -> %v", groupFile,
		oldNames, newNames)

	if len(oldNames) == 0 && len(newNames) == 0 {
		// Nothing to do.
		return nil
	}

	// Note that we can't use gpasswd here because `gpasswd --add` checks for the existence of the user, which causes an
	// NSS request to be sent to authd, but authd is not ready yet because we are still migrating the database.
	err := userslocking.WriteLock()
	if err != nil {
		return fmt.Errorf("failed to lock group file: %w", err)
	}
	defer func() {
		if err := userslocking.WriteUnlock(); err != nil {
			log.Warningf(context.Background(), "Failed to unlock group file: %v", err)
		}
	}()

	content, err := os.ReadFile(groupFile)
	if err != nil {
		return fmt.Errorf("error reading %s: %w", groupFile, err)
	}

	oldLines := strings.Split(string(content), "\n")
	var newLines []string

	for _, line := range oldLines {
		if line == "" {
			continue
		}

		fields := strings.SplitN(line, ":", 4)
		if len(fields) != 4 {
			return fmt.Errorf("unexpected number of fields in %s line (expected 4, got %d): %s",
				groupFile, len(fields), line)
		}

		users := strings.Split(fields[3], ",")
		for j, user := range users {
			for k, oldName := range oldNames {
				if user == oldName {
					users[j] = newNames[k]
				}
			}
		}

		fields[3] = strings.Join(users, ",")
		newLines = append(newLines, strings.Join(fields, ":"))
	}

	// Add final new line to the group file.
	newLines = append(newLines, "")

	if slices.Compare(oldLines, newLines) == 0 {
		return nil
	}

	backupPath := groupFileBackupPath()
	oldBackup := ""

	if tmpDir, err := os.MkdirTemp(os.TempDir(), "authd-migration-backup"); err == nil {
		defer os.Remove(tmpDir)

		b := filepath.Join(tmpDir, filepath.Base(backupPath))
		err := fileutils.CopyFile(backupPath, b)
		if err == nil {
			oldBackup = b
			defer os.Remove(oldBackup)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warningf(context.Background(), "Failed to create backup of %q: %v",
				backupPath, err)
		}
	}

	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warningf(context.Background(), "Failed to remove group file backup: %v", err)
	}

	backupAction := os.Rename
	if fi, _ := os.Lstat(groupFile); fi != nil && fi.Mode()&fs.ModeSymlink != 0 {
		backupAction = fileutils.CopyFile
	}
	if err := backupAction(groupFile, backupPath); err != nil {
		log.Warningf(context.Background(), "Failed make a backup for the group file: %v", err)

		if oldBackup != "" {
			// Backup of current group file failed, let's restore the old backup.
			if err := fileutils.Lrename(oldBackup, backupPath); err != nil {
				log.Warningf(context.Background(), "Failed restoring %q to %q: %v",
					oldBackup, backupPath, err)
			}
		}
	}

	tempPath := groupFileTemporaryPath()
	//nolint:gosec // G306 /etc/group should indeed have 0644 permissions
	if err := os.WriteFile(tempPath, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("error writing %s: %w", tempPath, err)
	}

	if err := fileutils.Lrename(tempPath, groupFile); err != nil {
		return fmt.Errorf("error renaming %s to %s: %w", tempPath, groupFile, err)
	}

	return nil
}

func removeGroupsWithNameConflicts(db queryable) error {
	// Delete groups with conflicting names
	rows, err := db.Query(`
		SELECT name FROM groups
		WHERE rowid NOT IN (
			SELECT MIN(rowid)
			FROM groups
			GROUP BY LOWER(name)
		);`)
	if err != nil {
		return fmt.Errorf("failed to query for groups with name conflicts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("failed to scan group name: %w", err)
		}

		log.Noticef(context.Background(), "Deleting group due to name conflict: %s", name)
		if _, err := db.Exec("DELETE FROM groups WHERE name = ?", name); err != nil {
			return fmt.Errorf("failed to delete group %s: %w", name, err)
		}
	}

	return nil
}
