package cache

const (
	// DbName is dbName exported for tests.
	DbName = dbName
	// DirtyFlagDbName is dirtyFlagDbName exported for tests.
	DirtyFlagDbName = dirtyFlagDbName
)

// RequestClearDatabase is used in tests for checking the behaviour of the database dynamic clear up.
func RequestClearDatabase(c *Cache) {
	c.requestClearDatabase()
}
