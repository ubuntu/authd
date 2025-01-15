package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ubuntu/authd/internal/users/db/bbolt"
	"github.com/ubuntu/authd/log"
)

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

	return m.migrateFromBBoltToSQLite(dbDir)
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
			// Version 0.4.2 introduced both the migration to SQLite and the lowercase usernames,
			// so we do both in one go and lowercase the usernames here.
			Name:     strings.ToLower(u.Name),
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

var schemaMigrations = map[string]string{
	"Migrate to lowercase user and group names": `
	UPDATE users SET name = LOWER(name);
	UPDATE groups SET name = LOWER(name);
	`,
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
	for description, query := range schemaMigrations {
		v++
		if currentVersion >= v {
			continue
		}

		log.Infof(context.Background(), "Applying schema migration: %s", description)
		_, err := m.db.Exec(query)
		if err != nil {
			return fmt.Errorf("failed to apply schema migration: %w", err)
		}

		if err := setSchemaVersion(m.db, v); err != nil {
			return fmt.Errorf("failed to update schema version: %w", err)
		}
	}

	log.Debugf(context.Background(), "Schema version after migrations: %d", v)

	return nil
}
