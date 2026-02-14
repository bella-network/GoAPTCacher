package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	httppprof "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"gitlab.com/bella.network/goaptcacher/pkg/buildinfo"
)

func initDebug() {
	if !config.Debug.Enable {
		return
	}

	log.Printf("[INFO] Debug output enabled")

	if config.Debug.LogIntervalSeconds > 0 {
		go debugLogger(time.Duration(config.Debug.LogIntervalSeconds) * time.Second)
	}

	if config.Debug.Pprof.Enable {
		if err := os.MkdirAll(config.Debug.Pprof.Directory, 0o755); err != nil {
			log.Printf("[WARN:DEBUG] Unable to create pprof directory %s: %v", config.Debug.Pprof.Directory, err)
		} else {
			log.Printf("[INFO] Pprof snapshots enabled: every %ds in %s", config.Debug.Pprof.IntervalSeconds, config.Debug.Pprof.Directory)
			go pprofSnapshotter(
				config.Debug.Pprof.Directory,
				time.Duration(config.Debug.Pprof.IntervalSeconds)*time.Second,
				config.Debug.Pprof.Retain,
			)
		}
	}
}

func debugLogger(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logDebugStats()
	for range ticker.C {
		logDebugStats()
	}
}

func logDebugStats() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	log.Printf(
		"[DEBUG:MEM] goroutines=%d heap_alloc=%s heap_inuse=%s heap_idle=%s heap_released=%s sys=%s gc_num=%d pause_total=%s",
		runtime.NumGoroutine(),
		formatBytes(mem.HeapAlloc),
		formatBytes(mem.HeapInuse),
		formatBytes(mem.HeapIdle),
		formatBytes(mem.HeapReleased),
		formatBytes(mem.Sys),
		mem.NumGC,
		time.Duration(mem.PauseTotalNs), //nolint:gosec
	)
}

func handleDebugRequests(w http.ResponseWriter, r *http.Request, requestedPath string) bool {
	if !config.Debug.Enable {
		return false
	}

	if !config.Debug.AllowRemote && !isLocalRequest(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return true
	}

	switch {
	case requestedPath == "/debug" || requestedPath == "/debug/":
		writeDebugJSON(w)
		return true
	case strings.HasPrefix(requestedPath, "/debug/pprof"):
		servePprof(w, r, requestedPath)
		return true
	default:
		return false
	}
}

func writeDebugJSON(w http.ResponseWriter) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	resp := map[string]any{
		"time":       time.Now().UTC().Format(time.RFC3339),
		"version":    buildinfo.Version,
		"commit":     buildinfo.Commit,
		"built_at":   buildinfo.Date,
		"go_version": runtime.Version(),
		"goroutines": runtime.NumGoroutine(),
		"gomaxprocs": runtime.GOMAXPROCS(0),
		"pprof": map[string]any{
			"enabled":           config.Debug.Pprof.Enable,
			"directory":         config.Debug.Pprof.Directory,
			"interval_seconds":  config.Debug.Pprof.IntervalSeconds,
			"retain_snapshots":  config.Debug.Pprof.Retain,
			"allow_remote":      config.Debug.AllowRemote,
			"log_interval_secs": config.Debug.LogIntervalSeconds,
		},
		"mem": map[string]any{
			"heap_alloc":     mem.HeapAlloc,
			"heap_inuse":     mem.HeapInuse,
			"heap_idle":      mem.HeapIdle,
			"heap_released":  mem.HeapReleased,
			"heap_sys":       mem.HeapSys,
			"stack_inuse":    mem.StackInuse,
			"stack_sys":      mem.StackSys,
			"mcache_inuse":   mem.MCacheInuse,
			"mspan_inuse":    mem.MSpanInuse,
			"sys":            mem.Sys,
			"num_gc":         mem.NumGC,
			"last_gc_unix":   mem.LastGC / uint64(time.Second),
			"pause_total_ns": mem.PauseTotalNs,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func servePprof(w http.ResponseWriter, r *http.Request, requestedPath string) {
	base := "/_goaptcacher/debug/pprof"
	path := strings.TrimPrefix(requestedPath, "/debug/pprof")
	if path == "" || path == "/" {
		writePprofIndex(w, base)
		return
	}

	name := strings.TrimPrefix(path, "/")
	switch name {
	case "cmdline":
		httppprof.Cmdline(w, r)
	case "profile":
		httppprof.Profile(w, r)
	case "symbol":
		httppprof.Symbol(w, r)
	case "trace":
		httppprof.Trace(w, r)
	default:
		httppprof.Handler(name).ServeHTTP(w, r)
	}
}

func writePprofIndex(w http.ResponseWriter, base string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, "<html><head><title>pprof</title></head><body>")
	_, _ = fmt.Fprintf(w, "<h1>pprof profiles</h1>")
	_, _ = fmt.Fprintf(w, "<p><a href=\"%s/cmdline\">cmdline</a> | <a href=\"%s/profile\">profile</a> | <a href=\"%s/symbol\">symbol</a> | <a href=\"%s/trace\">trace</a></p>", base, base, base, base)
	_, _ = fmt.Fprintf(w, "<ul>")
	profiles := pprof.Profiles()
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name() < profiles[j].Name()
	})
	for _, p := range profiles {
		name := p.Name()
		_, _ = fmt.Fprintf(w, "<li><a href=\"%s/%s\">%s</a></li>", base, name, name)
	}
	_, _ = fmt.Fprintf(w, "</ul></body></html>")
}

func pprofSnapshotter(dir string, interval time.Duration, retain int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	writePprofSnapshot(dir, retain)
	for range ticker.C {
		writePprofSnapshot(dir, retain)
	}
}

func writePprofSnapshot(dir string, retain int) {
	ts := time.Now().UTC().Format("20060102-150405")

	if err := writeHeapProfile(filepath.Join(dir, fmt.Sprintf("heap-%s.pprof", ts))); err != nil {
		log.Printf("[WARN:PPROF] heap snapshot failed: %v", err)
	}
	if err := writeProfile("goroutine", filepath.Join(dir, fmt.Sprintf("goroutine-%s.pprof", ts))); err != nil {
		log.Printf("[WARN:PPROF] goroutine snapshot failed: %v", err)
	}

	if retain > 0 {
		if err := cleanupOldProfiles(dir, retain); err != nil {
			log.Printf("[WARN:PPROF] cleanup failed: %v", err)
		}
	}
}

func writeHeapProfile(path string) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func writeProfile(name, path string) error {
	profile := pprof.Lookup(name)
	if profile == nil {
		return fmt.Errorf("profile %s not available", name)
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := profile.WriteTo(f, 0); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func cleanupOldProfiles(dir string, retain int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	type fileInfo struct {
		name string
		time time.Time
	}
	files := make([]fileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pprof") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{name: entry.Name(), time: info.ModTime()})
	}
	if len(files) <= retain {
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].time.After(files[j].time)
	})
	for _, old := range files[retain:] {
		_ = os.Remove(filepath.Join(dir, old.name))
	}
	return nil
}

func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func formatBytes(v uint64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%dB", v)
	}
	div, exp := uint64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(v)/float64(div), "KMGTPE"[exp])
}
