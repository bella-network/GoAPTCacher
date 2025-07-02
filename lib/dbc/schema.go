package dbc

import "gitlab.com/bella.network/goaptcacher/pkg/odb"

func CreateSchema(conn *odb.DBConnection) error {
	db := conn.GetDB()

	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS domains (
  id INT(11) UNSIGNED NOT NULL AUTO_INCREMENT,
  protocol TINYINT(1) UNSIGNED NOT NULL DEFAULT 0,
  domain VARCHAR(255) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY domain_protocol (domain, protocol),
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS files (
  id BIGINT(20) UNSIGNED NOT NULL AUTO_INCREMENT,
  domain INT(11) UNSIGNED NOT NULL,
  path TEXT NOT NULL,
  url TEXT NOT NULL,
  size BIGINT(20) UNSIGNED NOT NULL DEFAULT 0,
  modified DATETIME NOT NULL DEFAULT current_timestamp(),
  etag VARCHAR(100) CHARACTER SET ascii COLLATE ascii_general_ci DEFAULT NULL,
  sha256 TEXT character set ascii COLLATE ascii_general_ci DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY url (url) USING HASH,
  KEY domain (domain),
  CONSTRAINT files_ibfk_1 FOREIGN KEY (domain) REFERENCES domains (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS access_cache (
  file BIGINT(20) UNSIGNED NOT NULL,
  last_access DATETIME NOT NULL,
  last_check DATETIME NOT NULL,
  PRIMARY KEY (file),
  CONSTRAINT access_cache_ibfk_1 FOREIGN KEY (file) REFERENCES files (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS files_delete (
  file BIGINT(20) UNSIGNED NOT NULL,
  time DATETIME NOT NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
  PRIMARY KEY (file),
  CONSTRAINT files_delete_ibfk_1 FOREIGN KEY (file) REFERENCES files (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS stats (
  date DATE NOT NULL DEFAULT current_timestamp(),
  requests BIGINT(20) UNSIGNED NOT NULL DEFAULT 0,
  hits BIGINT(20) NOT NULL DEFAULT 0,
  misses BIGINT(20) NOT NULL DEFAULT 0,
  tunnel BIGINT(20) NOT NULL DEFAULT 0,
  traffic_down BIGINT(20) NOT NULL DEFAULT 0,
  traffic_up BIGINT(20) NOT NULL DEFAULT 0,
  tunnel_transfer BIGINT(20) NOT NULL DEFAULT 0,
  PRIMARY KEY (date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS keyvalue (
  key VARCHAR(100) CHARACTER SET ascii COLLATE ascii_general_ci NOT NULL,
  value TEXT NOT NULL,
  PRIMARY KEY (key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
INSERT INTO keyvalue (key, value) VALUES
('schema_version',	'1');
`)
	if err != nil {
		return err
	}

	return nil
}
