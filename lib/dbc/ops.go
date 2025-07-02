package dbc

import "gitlab.com/bella.network/goaptcacher/pkg/odb"

func AddDomain(db *odb.DBConnection, domain string) error {
	// Prepare the SQL statement to insert a new domain into the database.
	stmt, err := db.GetDB().Prepare("INSERT IGNORE INTO domains (domain) VALUES (?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Execute the statement with the provided domain.
	_, err = stmt.Exec(domain)
	if err != nil {
		return err
	}

	return nil
}
