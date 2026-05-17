// export_test.go exposes internal helpers for white-box testing.
package gensum

// ReplaceChecksumValuesExported exposes replaceChecksumValues for tests.
func ReplaceChecksumValuesExported(content, fieldName string, newHashes []string) (string, error) {
	return replaceChecksumValues(content, fieldName, newHashes)
}

// ExtractSourceBlocksExported exposes extractArrayBlocks for source arrays.
func ExtractSourceBlocksExported(content string) map[string]string {
	return extractArrayBlocks(content, sourceArrayRe)
}

// ParseArrayValuesExported exposes parseArrayValues for tests.
func ParseArrayValuesExported(block string) []string {
	return parseArrayValues(block)
}

// ReplaceHashesInBlockExported exposes replaceHashesInBlock for tests.
func ReplaceHashesInBlockExported(block string, hashes []string) (string, error) {
	return replaceHashesInBlock(block, hashes)
}

// ExtractScalarVarsExported exposes extractScalarVars for tests.
func ExtractScalarVarsExported(content string) func(string) string {
	return extractScalarVars(content)
}
