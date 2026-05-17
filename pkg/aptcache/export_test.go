// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptcache

import "io"

// NewCacheForTesting creates an empty Cache suitable for unit tests.
func NewCacheForTesting() *Cache {
	return &Cache{
		entries:   make(map[string]*PackageInfo),
		providers: make(map[string][]string),
	}
}

// ParseDeb822ForTesting exposes parseDeb822 for unit tests.
func (c *Cache) ParseDeb822ForTesting(r io.Reader, dpkgStatus bool) error {
	return c.parseDeb822(r, dpkgStatus)
}
