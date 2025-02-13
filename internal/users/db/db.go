// Package db handles transaction with an underlying database to store user and group information.
package db

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	// sqlite3 driver.
	_ "github.com/mattn/go-sqlite3"
	"github.com/ubuntu/authd/internal/fileutils"
	"github.com/ubuntu/authd/internal/users/db/bbolt"
	"github.com/ubuntu/authd/log"
)

var (
	filename = "authd.sqlite3"
	//go:embed sql/create_schema.sql
	createSchema string
)

// Manager is an abstraction to interact with the database.
type Manager struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

// GroupDB is the public type representing a group in the database.
type GroupDB struct {
	Name  string
	GID   uint32
	UGID  string
	Users []string
}

// queryable is an interface to execute SQL queries. Both sql.DB and sql.Tx implement this interface.
type queryable interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// New creates a new database manager by creating or opening the underlying database.
func New(dbDir string) (*Manager, error) {
	dbPath := filepath.Join(dbDir, filename)

	exists, err := fileutils.FileExists(dbPath)
	if err != nil {
		return nil, err
	}
	if !exists {
		// Create the database with permissions 0600.
		if err := fileutils.Touch(dbPath); err != nil {
			return nil, err
		}
	}

	// Fail if permissions are not 0600
	// TODO: I don't see why we should fail here instead of just fixing the permissions.
	fileInfo, err := os.Stat(dbPath)
	if err != nil {
		return nil, fmt.Errorf("can't stat database file: %v", err)
	}
	perm := fileInfo.Mode().Perm()
	if perm != 0600 {
		return nil, fmt.Errorf("wrong file permission for %s: %o", dbPath, perm)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Enable foreign key support (this needs to be done for each connection, so we can't do it in the schema).
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if !exists {
		log.Debugf(context.Background(), "Creating new SQLite database at %v", dbPath)
		_, err = db.Exec(createSchema)
		if err != nil {
			return nil, err
		}
	}

	return &Manager{db: db, path: dbPath, mu: sync.RWMutex{}}, nil
}

// MigrateData migrates data from bbolt to SQLite.
func MigrateData(dbDir string) error {
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

	return m.migrateData(dbDir)
}

func (m *Manager) migrateData(dbDir string) error {
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

	// Start a transaction
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// Ensure the transaction is committed or rolled back
	defer func() {
		// If there's an error, roll back the transaction
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("failed to rollback transaction: %w", rollbackErr))
			}
			return
		}
		// Otherwise, commit the transaction
		err = tx.Commit()
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

		user := UserDB{
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
		group := GroupDB{
			Name:  g.Name,
			GID:   g.GID,
			UGID:  g.UGID,
			Users: g.Users,
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

// Close closes the db and signal the monitoring goroutine to stop.
func (m *Manager) Close() error {
	log.Debugf(context.Background(), "Closing database")
	return m.db.Close()
}

// Filename returns the name of the database file.
func Filename() string {
	return filename
}

// RemoveDB removes the database file.
func RemoveDB(dbDir string) error {
	return os.Remove(filepath.Join(dbDir, filename))
}

// NoDataFoundError is returned when we didn’t find a matching entry.
type NoDataFoundError struct {
	key   string
	table string
}

// Error implements the error interface.
func (err NoDataFoundError) Error() string {
	return fmt.Sprintf("no result matching %v in %v", err.key, err.table)
}

// Is makes this error insensitive to the key and table names.
func (NoDataFoundError) Is(target error) bool { return target == NoDataFoundError{} }

func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		log.Warningf(context.Background(), "failed to close rows: %v", err)
	}
}
