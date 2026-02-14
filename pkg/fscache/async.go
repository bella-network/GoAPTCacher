package fscache

import "log"

func (c *FSCache) trackRequestAsync(cacheHit bool, transferred int64) {
	go func() {
		if err := c.TrackRequest(cacheHit, transferred); err != nil {
			log.Printf("[WARN:STATS] failed to track request: %v", err)
		}
	}()
}

func (c *FSCache) hitAsync(protocol int, domain, path string) {
	go func() {
		if err := c.Hit(protocol, domain, path); err != nil {
			log.Printf("[WARN:ACCESS] failed to update hit count for %s%s: %v", domain, path, err)
		}
	}()
}

func (c *FSCache) addURLIfNotExistsAsync(protocol int, domain, path, urlString string) {
	go func() {
		if err := c.AddURLIfNotExists(protocol, domain, path, urlString); err != nil {
			log.Printf("[WARN:ACCESS] failed to update URL for %s%s: %v", domain, path, err)
		}
	}()
}
