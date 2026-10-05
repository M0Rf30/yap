// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

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
