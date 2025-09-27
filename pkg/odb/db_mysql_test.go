package odb

import (
	"testing"
)

func TestNewMySQL(t *testing.T) {
	options := DatabaseOptions{
		Host:     "localhost",
		Port:     3306,
		Username: "testuser",
		Password: "testpassword",
		Database: "testdb",
	}

	dbConn, err := NewMySQL(options)
	if err != nil {
		t.Skipf("Skipping MySQL test: %v", err)
	}
	defer dbConn.Close()

	if err := dbConn.Ping(); err != nil {
		t.Fatalf("Failed to ping MySQL database: %v", err)
	}
}
