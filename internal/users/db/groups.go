package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

type groupRow struct {
	Name string
	GID  uint32
	UGID string
}

type userToGroupRow struct {
	UID uint32
	GID uint32
}

// NewGroupDB creates a new GroupDB.
func NewGroupDB(name string, gid uint32, ugid string, members []string) GroupDB {
	return GroupDB{
		Name:  name,
		GID:   gid,
		UGID:  ugid,
		Users: members,
	}
}

// GroupByID returns a group matching this gid or an error if the database is corrupted or no entry was found.
func (c *Database) GroupByID(gid uint32) (GroupDB, error) {
	return groupByID(c.db, gid)
}

func groupByID(db queryable, gid uint32) (GroupDB, error) {
	query := `SELECT name, gid, ugid FROM groups WHERE gid = ?`
	row := db.QueryRow(query, gid)

	var g GroupDB
	err := row.Scan(&g.Name, &g.GID, &g.UGID)
	if errors.Is(err, sql.ErrNoRows) {
		return GroupDB{}, NoDataFoundError{key: strconv.FormatUint(uint64(gid), 10), table: "groups"}
	}
	if err != nil {
		return GroupDB{}, fmt.Errorf("query error: %w", err)
	}

	g.Users, err = getGroupMembers(db, g.GID)
	if err != nil {
		return GroupDB{}, fmt.Errorf("failed to get group members: %w", err)
	}

	return g, nil
}

// GroupByName returns a group matching a given name or an error if the database is corrupted or no entry was found.
func (c *Database) GroupByName(name string) (GroupDB, error) {
	query := `SELECT name, gid, ugid FROM groups WHERE name = ?`
	row := c.db.QueryRow(query, name)

	var g GroupDB
	err := row.Scan(&g.Name, &g.GID, &g.UGID)
	if errors.Is(err, sql.ErrNoRows) {
		return GroupDB{}, NoDataFoundError{key: name, table: "groups"}
	}
	if err != nil {
		return GroupDB{}, fmt.Errorf("query error: %w", err)
	}

	g.Users, err = getGroupMembers(c.db, g.GID)
	if err != nil {
		return GroupDB{}, fmt.Errorf("failed to get group members: %w", err)
	}

	return g, nil
}

// GroupByUGID returns a group matching this ugid or an error if the database is corrupted or no entry was found.
func (c *Database) GroupByUGID(ugid string) (GroupDB, error) {
	query := `SELECT name, gid, ugid FROM groups WHERE ugid = ?`
	row := c.db.QueryRow(query, ugid)

	var g GroupDB
	err := row.Scan(&g.Name, &g.GID, &g.UGID)
	if errors.Is(err, sql.ErrNoRows) {
		return GroupDB{}, NoDataFoundError{key: ugid, table: "groups"}
	}
	if err != nil {
		return GroupDB{}, fmt.Errorf("query error: %w", err)
	}

	g.Users, err = getGroupMembers(c.db, g.GID)
	if err != nil {
		return GroupDB{}, fmt.Errorf("failed to get group members: %w", err)
	}

	return g, nil
}

// AllGroups returns all groups from the database.
func (c *Database) AllGroups() ([]GroupDB, error) {
	return c.allGroups(c.db)
}

func (c *Database) allGroups(db queryable) ([]GroupDB, error) {
	groups, err := allGroupsInternal(db)
	if err != nil {
		return nil, err
	}

	var res []GroupDB
	for _, g := range groups {
		group := NewGroupDB(g.Name, g.GID, g.UGID, nil)
		group.Users, err = getGroupMembers(db, g.GID)
		if err != nil {
			return nil, fmt.Errorf("failed to get group members: %w", err)
		}
		res = append(res, group)
	}

	return res, nil
}

// allGroupsInternal returns all groups from the database as the internal groupRow type.
func allGroupsInternal(db queryable) ([]groupRow, error) {
	query := `SELECT name, gid, ugid FROM groups`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer closeRows(rows)

	var groups []groupRow
	for rows.Next() {
		var g groupRow
		err := rows.Scan(&g.Name, &g.GID, &g.UGID)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		groups = append(groups, g)
	}

	// Check for errors from iteration
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return groups, nil
}

func insertOrUpdateGroup(db queryable, g GroupDB) error {
	exists, err := groupExists(db, g)
	if err != nil {
		return fmt.Errorf("failed to check if group exists: %w", err)
	}

	if !exists {
		return insertGroup(db, g)
	}

	return updateGroup(db, g)
}

func groupExists(db queryable, g GroupDB) (bool, error) {
	query := `
		SELECT 1 FROM groups 
		WHERE name = ? OR gid = ? OR ugid = ? 
		LIMIT 1`
	row := db.QueryRow(query, g.Name, g.GID, g.UGID)

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

func insertGroup(db queryable, g GroupDB) error {
	_, err := db.Exec(`INSERT INTO groups (name, gid, ugid) VALUES (?, ?, ?)`, g.Name, g.GID, g.UGID)
	if err != nil {
		return fmt.Errorf("insert group error: %w", err)
	}

	return nil
}

func updateGroup(db queryable, g GroupDB) error {
	_, err := db.Exec(`UPDATE groups SET name = ?, gid = ?, ugid = ? WHERE gid = ?`, g.Name, g.GID, g.UGID, g.GID)
	if err != nil {
		return fmt.Errorf("update group error: %w", err)
	}

	return nil
}
