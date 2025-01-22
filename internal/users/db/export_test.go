package db

// DbPath exposes the path to the database file for testing.
func (c *Database) DbPath() string {
	return c.db.Path()
}
