package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/ubuntu/authd/log"
)

const allUserColumns = "name, uid, gid, gecos, dir, shell, broker_id"
const publicUserColumns = "name, uid, gid, gecos, dir, shell, broker_id"
const allUserColumnsWithPlaceholders = "name = ?, uid = ?, gid = ?, gecos = ?, dir = ?, shell = ?, broker_id = ?"

// UserDB is the public type that is shared to external packages.
type UserDB struct {
	Name  string
	UID   uint32
	GID   uint32
	Gecos string // Gecos is an optional field. It can be empty.
	Dir   string
	Shell string

	// BrokerID specifies the broker the user last successfully authenticated with.
	BrokerID string `yaml:"broker_id,omitempty"`
}

// NewUserDB creates a new UserDB.
func NewUserDB(name string, uid, gid uint32, gecos, dir, shell string) UserDB {
	return UserDB{
		Name:  name,
		UID:   uid,
		GID:   gid,
		Gecos: gecos,
		Dir:   dir,
		Shell: shell,
	}
}

// UserByID returns a user matching this uid or an error if the database is corrupted or no entry was found.
func (c *Database) UserByID(uid uint32) (UserDB, error) {
	return userByID(c.db, uid)
}

func userByID(db queryable, uid uint32) (UserDB, error) {
	query := fmt.Sprintf(`SELECT %s FROM users WHERE uid = ?`, publicUserColumns)
	row := db.QueryRow(query, uid)

	var u UserDB
	err := row.Scan(&u.Name, &u.UID, &u.GID, &u.Gecos, &u.Dir, &u.Shell, &u.BrokerID)
	if errors.Is(err, sql.ErrNoRows) {
		return UserDB{}, NoDataFoundError{key: strconv.FormatUint(uint64(uid), 10), table: "users"}
	}
	if err != nil {
		return UserDB{}, fmt.Errorf("query error: %w", err)
	}

	return u, nil
}

// UserByName returns a user matching this name or an error if the database is corrupted or no entry was found.
func (c *Database) UserByName(name string) (UserDB, error) {
	query := fmt.Sprintf(`SELECT %s FROM users WHERE name = ?`, publicUserColumns)
	row := c.db.QueryRow(query, name)

	var u UserDB
	err := row.Scan(&u.Name, &u.UID, &u.GID, &u.Gecos, &u.Dir, &u.Shell, &u.BrokerID)
	if errors.Is(err, sql.ErrNoRows) {
		return UserDB{}, NoDataFoundError{key: name, table: "users"}
	}
	if err != nil {
		return UserDB{}, fmt.Errorf("query error: %w", err)
	}

	return u, nil
}

// AllUsers returns all users or an error if the database is corrupted.
func (c *Database) AllUsers() ([]UserDB, error) {
	return allUsers(c.db)
}

func allUsers(db queryable) ([]UserDB, error) {
	query := fmt.Sprintf(`SELECT %s FROM users`, allUserColumns)
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer closeRows(rows)

	var users []UserDB
	for rows.Next() {
		var u UserDB
		err := rows.Scan(&u.Name, &u.UID, &u.GID, &u.Gecos, &u.Dir, &u.Shell, &u.BrokerID)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		users = append(users, u)
	}

	// Check for errors from iteration
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return users, nil
}

func insertOrUpdateUserByID(db queryable, u UserDB) error {
	exists, err := userExists(db, u)
	if err != nil {
		return fmt.Errorf("failed to check if user exists: %w", err)
	}

	if !exists {
		return insertUser(db, u)
	}

	return updateUserByID(db, u)
}

func userExists(db queryable, u UserDB) (bool, error) {
	query := `
		SELECT 1 FROM users 
		WHERE name = ? OR uid = ? 
		LIMIT 1`
	row := db.QueryRow(query, u.Name, u.UID)

	var exists int
	err := row.Scan(&exists)

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query error: %w", err)
	}

	return true, nil
}

func insertUser(db queryable, u UserDB) error {
	log.Debugf(context.Background(), "Inserting user %v", u.Name)
	query := fmt.Sprintf(`INSERT INTO users (%s) VALUES (?, ?, ?, ?, ?, ?, ?)`, allUserColumns)
	_, err := db.Exec(query, u.Name, u.UID, u.GID, u.Gecos, u.Dir, u.Shell, u.BrokerID)
	if err != nil {
		return fmt.Errorf("insert user error: %w", err)
	}
	return nil
}

func updateUserByID(db queryable, u UserDB) error {
	log.Debugf(context.Background(), "Updating user %v", u.Name)
	query := fmt.Sprintf(`UPDATE users SET %s WHERE uid = ?`, allUserColumnsWithPlaceholders)
	_, err := db.Exec(query, u.Name, u.UID, u.GID, u.Gecos, u.Dir, u.Shell, u.BrokerID, u.UID)
	if err != nil {
		return fmt.Errorf("update user error: %w", err)
	}
	return nil
}

// DeleteUser removes the user from the database.
func (c *Database) DeleteUser(uid uint32) error {
	query := `DELETE FROM users WHERE uid = ?`
	res, err := c.db.Exec(query, uid)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return NoDataFoundError{table: "users", key: strconv.FormatUint(uint64(uid), 10)}
	}

	return nil
}
