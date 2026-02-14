package fscache

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

const (
	statsFlushIntervalDefault = 30 * time.Second
	statsFileName             = ".stats.json"
)

type statsEntry struct {
	Requests       uint64 `json:"requests"`
	Hits           uint64 `json:"hits"`
	Misses         uint64 `json:"misses"`
	Tunnel         uint64 `json:"tunnel"`
	TrafficDown    uint64 `json:"traffic_down"`
	TrafficUp      uint64 `json:"traffic_up"`
	TunnelTransfer uint64 `json:"tunnel_transfer"`
}

type persistedStats struct {
	Version int                   `json:"version"`
	Daily   map[string]statsEntry `json:"daily"`
}

type StatsDay struct {
	Date           time.Time
	Requests       uint64
	Hits           uint64
	Misses         uint64
	Tunnel         uint64
	TrafficDown    uint64
	TrafficUp      uint64
	TunnelTransfer uint64
}

type StatsTotals struct {
	Requests       uint64
	Hits           uint64
	Misses         uint64
	Tunnel         uint64
	TrafficDown    uint64
	TrafficUp      uint64
	TunnelTransfer uint64
}

type StatsSnapshot struct {
	Totals    StatsTotals
	Daily     []StatsDay
	OldestDay time.Time
}

func (c *FSCache) statsFilePath() string {
	return filepath.Join(c.CachePath, statsFileName)
}

func (c *FSCache) startStatsFlushLoop() {
	interval := c.statsFlushInterval
	if interval <= 0 {
		interval = statsFlushIntervalDefault
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := c.flushStatsToDisk(); err != nil {
					log.Printf("[WARN:STATS] failed to persist stats: %v", err)
				}
			case <-c.statsStop:
				return
			}
		}
	}()
}

func (c *FSCache) loadStatsFromDisk() error {
	data, err := os.ReadFile(c.statsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var persisted persistedStats
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}

	loaded := make(map[string]*statsEntry, len(persisted.Daily))
	for day, entry := range persisted.Daily {
		if _, err := time.Parse("2006-01-02", day); err != nil {
			continue
		}
		entryCopy := entry
		loaded[day] = &entryCopy
	}

	c.statsMux.Lock()
	c.statsByDate = loaded
	c.statsDirty = false
	c.statsRevision = 0
	c.statsMux.Unlock()

	return nil
}

func (c *FSCache) flushStatsToDisk() error {
	c.statsMux.RLock()
	if !c.statsDirty {
		c.statsMux.RUnlock()
		return nil
	}

	revision := c.statsRevision
	daily := make(map[string]statsEntry, len(c.statsByDate))
	for day, entry := range c.statsByDate {
		daily[day] = *entry
	}
	c.statsMux.RUnlock()

	payload := persistedStats{
		Version: 1,
		Daily:   daily,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(c.CachePath, 0o755); err != nil {
		return err
	}

	targetPath := c.statsFilePath()
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}

	c.statsMux.Lock()
	if c.statsRevision == revision {
		c.statsDirty = false
	}
	c.statsMux.Unlock()

	return nil
}

func (c *FSCache) dayStatsLocked(day string) *statsEntry {
	entry, ok := c.statsByDate[day]
	if !ok {
		entry = &statsEntry{}
		c.statsByDate[day] = entry
	}
	return entry
}

// TrackRequest updates request statistics for cache hits and misses.
func (c *FSCache) TrackRequest(cacheHit bool, transferred int64) error {
	transferredBytes := nonNegativeInt64ToUint64(transferred)

	c.statsMux.Lock()
	day := time.Now().Format("2006-01-02")
	entry := c.dayStatsLocked(day)
	entry.Requests++
	if cacheHit {
		entry.Hits++
		entry.TrafficUp += transferredBytes
	} else {
		entry.Misses++
		entry.TrafficDown += transferredBytes
		entry.TrafficUp += transferredBytes
	}
	c.statsDirty = true
	c.statsRevision++
	c.statsMux.Unlock()

	return nil
}

// TrackTunnelRequest updates statistics for tunnel traffic.
func (c *FSCache) TrackTunnelRequest(transferred int64) error {
	transferredBytes := nonNegativeInt64ToUint64(transferred)

	c.statsMux.Lock()
	day := time.Now().Format("2006-01-02")
	entry := c.dayStatsLocked(day)
	entry.Requests++
	entry.Tunnel++
	entry.TrafficDown += transferredBytes
	entry.TrafficUp += transferredBytes
	entry.TunnelTransfer += transferredBytes
	c.statsDirty = true
	c.statsRevision++
	c.statsMux.Unlock()

	return nil
}

func nonNegativeInt64ToUint64(v int64) uint64 {
	if v <= 0 {
		return 0
	}

	u, err := strconv.ParseUint(strconv.FormatInt(v, 10), 10, 64)
	if err != nil {
		return 0
	}

	return u
}

// GetStatsSnapshot returns aggregate and per-day statistics.
func (c *FSCache) GetStatsSnapshot(limit int) StatsSnapshot {
	c.statsMux.RLock()
	snapshotDaily := make(map[string]statsEntry, len(c.statsByDate))
	for day, entry := range c.statsByDate {
		snapshotDaily[day] = *entry
	}
	c.statsMux.RUnlock()

	keys := make([]string, 0, len(snapshotDaily))
	for day := range snapshotDaily {
		keys = append(keys, day)
	}
	sort.Strings(keys)

	stats := StatsSnapshot{
		Daily: make([]StatsDay, 0),
	}

	for _, day := range keys {
		entry := snapshotDaily[day]
		stats.Totals.Requests += entry.Requests
		stats.Totals.Hits += entry.Hits
		stats.Totals.Misses += entry.Misses
		stats.Totals.Tunnel += entry.Tunnel
		stats.Totals.TrafficDown += entry.TrafficDown
		stats.Totals.TrafficUp += entry.TrafficUp
		stats.Totals.TunnelTransfer += entry.TunnelTransfer
	}

	if len(keys) > 0 {
		if oldest, err := time.Parse("2006-01-02", keys[0]); err == nil {
			stats.OldestDay = oldest
		}
	} else {
		stats.OldestDay = time.Now()
	}

	if limit <= 0 || limit > len(keys) {
		limit = len(keys)
	}

	for i := len(keys) - 1; i >= 0 && len(stats.Daily) < limit; i-- {
		day := keys[i]
		parsedDay, err := time.Parse("2006-01-02", day)
		if err != nil {
			continue
		}

		entry := snapshotDaily[day]
		stats.Daily = append(stats.Daily, StatsDay{
			Date:           parsedDay,
			Requests:       entry.Requests,
			Hits:           entry.Hits,
			Misses:         entry.Misses,
			Tunnel:         entry.Tunnel,
			TrafficDown:    entry.TrafficDown,
			TrafficUp:      entry.TrafficUp,
			TunnelTransfer: entry.TunnelTransfer,
		})
	}

	return stats
}

// GetCacheUsage returns number and total size of cached files tracked by metadata.
func (c *FSCache) GetCacheUsage() (uint64, uint64, error) {
	entries, err := c.collectAccessCacheRecords()
	if err != nil {
		return 0, 0, err
	}

	seen := make(map[string]struct{}, len(entries))
	var filesCached uint64
	var totalSize uint64

	for _, record := range entries {
		entry := c.normalizeAccessEntry(record.protocol, record.domain, record.path, record.entry)
		if entry.URL == nil {
			continue
		}

		localPath := c.buildLocalPath(entry.URL)
		if _, ok := seen[localPath]; ok {
			continue
		}
		seen[localPath] = struct{}{}

		info, err := os.Stat(localPath)
		if err != nil {
			continue
		}

		filesCached++
		totalSize += nonNegativeInt64ToUint64(info.Size())
	}

	return filesCached, totalSize, nil
}
