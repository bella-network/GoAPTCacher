package fscache_old

import "time"

type RecentFileCache struct {
	LastAccessed time.Time `json:"last_accessed"`
	LastChecked  time.Time `json:"last_checked"`
}
