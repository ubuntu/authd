package db

// Path exposes the path to the database file for testing.
func (c *Database) Path() string {
	return c.path
}
