package odb

import (
	"database/sql"
	"time"
)

type DatabaseOptions struct {
	Host     string
	Port     int
	Username string
	Password string
	Database string
}

type DBConnection struct {
	Conn    *sql.DB
	options DatabaseOptions
}

// Close closes the database connection.
func (db *DBConnection) Close() error {
	return db.Conn.Close()
}

// Ping pings the database connection.
func (db *DBConnection) Ping() error {
	return db.Conn.Ping()
}

// GetDB returns the database connection.
func (db *DBConnection) GetDB() *sql.DB {
	return db.Conn
}

// configureDB sets the database connection pool parameters.
func configureDB(db *sql.DB) {
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(time.Minute * 15)
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(15)
}
