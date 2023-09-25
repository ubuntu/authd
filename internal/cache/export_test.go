package cache

// RequestClearDatabase is used in tests for checking the behaviour of the database dynamic clear up.
func RequestClearDatabase(c *Cache) {
	c.requestClearDatabase()
}
