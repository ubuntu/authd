package db

// Path exposes the path to the database file for testing.
func (m *Manager) Path() string {
	return m.path
}

// GroupFile exposes the path to the group file for testing.
func GroupFile() string {
	return groupFile
}

// SetGroupFile sets the path to the group file for testing.
func SetGroupFile(path string) {
	groupFile = path
}

// GetCreateSchemaQuery exposes the query to create the schema for testing.
func GetCreateSchemaQuery() string {
	return createSchemaQuery
}

// SetCreateSchemaQuery sets the query to create the schema for testing.
func SetCreateSchemaQuery(query string) {
	createSchemaQuery = query
}
