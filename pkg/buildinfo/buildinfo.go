package buildinfo

// Version metadata is overridden at build time via ldflags.
var (
	Version = "0.0.0-DEBUG"
	Commit  = "unknown"
	Date    = "unknown"
)
