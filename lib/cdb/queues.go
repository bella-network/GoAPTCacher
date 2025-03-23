package cdb

import (
	"database/sql"
	"time"
)

func TrackRequest(db *sql.DB, cacheHit bool, transferred int64) error {
	_, err := db.Exec(
		"INSERT OR IGNORE INTO stats (date, requests, hits, misses, traffic_down, traffic_up, tunnel, tunnel_transfer) VALUES (?, 0, 0, 0, 0, 0, 0, 0)",
		time.Now().Format("2006-01-02"),
	)
	if err != nil {
		return err
	}

	if cacheHit {
		_, err = db.Exec(
			"UPDATE stats SET requests = requests + 1, hits = hits + 1, traffic_up = traffic_up + ? WHERE date = ?",
			transferred,
			time.Now().Format("2006-01-02"),
		)
	} else {
		_, err = db.Exec(
			"UPDATE stats SET requests = requests + 1, misses = misses + 1, traffic_down = traffic_down + ?, traffic_up = traffic_up + ? WHERE date = ?",
			transferred,
			transferred,
			time.Now().Format("2006-01-02"),
		)
	}

	return err
}

// TrackTunnelRequest tracks a tunnel request in the database.
func TrackTunnelRequest(db *sql.DB, transferred int64) error {
	_, err := db.Exec(
		"INSERT OR IGNORE INTO stats (date, requests, hits, misses, traffic_down, traffic_up, tunnel, tunnel_transfer) VALUES (?, 0, 0, 0, 0, 0, 0, 0)",
		time.Now().Format("2006-01-02"),
	)
	if err != nil {
		return err
	}

	_, err = db.Exec(
		"UPDATE stats SET requests = requests + 1, tunnel = tunnel + 1, traffic_down = traffic_down + ?, traffic_up = traffic_up + ?, tunnel_transfer = tunnel_transfer + ? WHERE date = ?",
		transferred,
		transferred,
		transferred,
		time.Now().Format("2006-01-02"),
	)

	return err
}
