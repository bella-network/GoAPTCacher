package dbc

import "gitlab.com/bella.network/goaptcacher/pkg/odb"

// CheckSchemaCreation checks if the database schema has been created and is
// up-to-date. If the schema is not created, it will create it. If the schema is
// outdated, it will migrate it to the latest version.
func CheckSchemaCreation(conn *odb.DBConnection) error {
	db := conn.GetDB()

	// Check if the keyvalue table exists
	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'keyvalue')").Scan(&exists)
	if err != nil {
		return err
	}

	if !exists {
		// Create the schema if it does not exist
		err := CreateSchema(conn)
		if err != nil {
			return err
		}
	}

	// Migrate the schema if needed
	return MigrateSchema(conn)
}

// MigrateSchema checks the current schema version and performs migrations if
// necessary. It returns an error if the migration fails.
func MigrateSchema(conn *odb.DBConnection) error {
	db := conn.GetDB()

	// Get current schema version
	var currentVersion int
	err := db.QueryRow("SELECT `value` FROM `keyvalue` WHERE `key` = 'schema_version'").Scan(&currentVersion)
	if err != nil {
		return err
	}

	// Check if migration is needed
	if currentVersion <= 0 {
		return nil
	}

	// Perform migrations when needed

	return nil
}
