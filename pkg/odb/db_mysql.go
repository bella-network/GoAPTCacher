package odb

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// NewMySQL creates a new MySQL database connection.
func NewMySQL(options DatabaseOptions) (*DBConnection, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&collation=utf8mb4_unicode_ci&timeout=30s&readTimeout=30s&writeTimeout=30s&parseTime=true&loc=Local&interpolateParams=true&allowNativePasswords=true&maxAllowedPacket=67108864",
		options.Username,
		options.Password,
		options.Host,
		options.Port,
		options.Database,
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// Set the database connection pool parameters.
	configureDB(db)

	// Ping the database connection to verify the connection.
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &DBConnection{db, options}, nil
}
