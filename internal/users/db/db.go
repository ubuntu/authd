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
	"syscall"

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

	if err := checkOwnerAndPermissions(dbPath); err != nil {
		return nil, err
	}

	// Use cache=shared to avoid the "database is locked" error as documented in the FAQ:
	// https://github.com/mattn/go-sqlite3?tab=readme-ov-file#faq
	dataSourceName := fmt.Sprintf("file:%s?cache=shared", dbPath)
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	// Set database connections to 1 to avoid the "database is locked" error as documented in the FAQ:
	// https://github.com/mattn/go-sqlite3?tab=readme-ov-file#faq
	db.SetMaxOpenConns(1)

	// Enable foreign key support (this needs to be done for each connection, so we can't do it in the schema).
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if !exists {
		log.Debugf(context.Background(), "Creating new SQLite database at %v", dbPath)
		_, err = db.Exec(createSchema)
		if err != nil {
			// Remove the database file if we failed to create the schema, to avoid that authd tries to use a broken
			// database on the next start.
			if removeErr := os.Remove(dbPath); removeErr != nil {
				log.Warningf(context.Background(), "Failed to remove database file after failed schema creation: %v", removeErr)
			}
			return nil, fmt.Errorf("failed to create schema: %w", err)
		}
	}

	return &Manager{db: db, path: dbPath, mu: sync.RWMutex{}}, nil
}

// checkOwnerAndPermissions checks if the database file has secure owner and permissions.
func checkOwnerAndPermissions(path string) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("can't stat database file: %v", err)
	}

	// Fail if the file is not owned by root or the current user.
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("can't get file information for %s", path)
	}
	if stat.Uid != 0 && int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("unexpected file owner for %s, should be root or %d but is %d", path, os.Getuid(), stat.Uid)
	}

	// Fail if the file is world-writable.
	perm := fileInfo.Mode().Perm()
	if perm&0002 != 0 {
		return fmt.Errorf("insecure file permissions for %s: %o", path, perm)
	}

	return nil
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

func (m *Manager) migrateData(dbDir string) (err error) {
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
		group := GroupRow{Name: g.Name, GID: g.GID, UGID: g.UGID}

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

func commitOrRollBackTransaction(err error, tx *sql.Tx) error {
	// If there's an error, roll back the transaction
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to rollback transaction: %w", rollbackErr))
		}
		return err
	}

	// Otherwise, commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
