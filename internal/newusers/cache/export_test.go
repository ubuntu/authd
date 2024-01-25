package cache

// DbPath exposes the path to the database file for testing.
func (c *Cache) DbPath() string {
	return c.db.Path()
}
