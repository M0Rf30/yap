// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package source

// ParseURIForTesting exposes parseURI for unit tests.
func (src *Source) ParseURIForTesting() {
	src.parseURI()
}

// GetProtocolForTesting exposes getProtocol for unit tests.
func (src *Source) GetProtocolForTesting() string {
	return src.getProtocol()
}
