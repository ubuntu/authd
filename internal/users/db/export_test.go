package db

// Path exposes the path to the database file for testing.
func (m *Manager) Path() string {
	return m.path
}

// UserGroupFile exposes the path to the user group file for testing.
func UserGroupFile() string {
	return userGroupFile
}

// SetUserGroupFile sets the path to the user group file for testing.
func SetUserGroupFile(path string) {
	userGroupFile = path
}

// GetCreateSchemaQuery exposes the query to create the schema for testing.
func GetCreateSchemaQuery() string {
	return createSchemaQuery
}

// SetCreateSchemaQuery sets the query to create the schema for testing.
func SetCreateSchemaQuery(query string) {
	createSchemaQuery = query
}
