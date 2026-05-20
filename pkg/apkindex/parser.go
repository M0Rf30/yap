package apkindex

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// stripVersionConstraint strips any version constraint suffix from a package
// spec like "musl-dev>=1.2" or "foo!=1.0" → "musl-dev" / "foo".
// Handles all common operators: !=, >=, <=, ~, =, >, <.
// Order matters: !=, >=, <= must be checked before single-char operators.
func stripVersionConstraint(spec string) string {
	spec = strings.TrimSpace(spec)

	// Multi-char operators first (longest match wins).
	for _, op := range []string{"!=", ">=", "<="} {
		if before, _, ok := strings.Cut(spec, op); ok {
			return strings.TrimSpace(before)
		}
	}

	// Single-char operators.
	for _, op := range []string{"~", "=", ">", "<"} {
		if before, _, ok := strings.Cut(spec, op); ok {
			return strings.TrimSpace(before)
		}
	}

	return spec
}

// ParseIndex parses an APKINDEX text stream into the index.
// repoBaseURL is attached to each parsed Package for later download URL construction.
func (idx *Index) ParseIndex(r io.Reader, repoBaseURL string) error {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for large lines (some packages have many dependencies).
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	var cur Package

	cur.RepoBaseURL = repoBaseURL

	for scanner.Scan() {
		line := scanner.Text()

		// Blank line signals end of stanza.
		if line == "" {
			idx.flushAPKPackage(&cur, repoBaseURL)
			continue
		}

		// APK APKINDEX uses single-char field tags: "P:<value>"
		if len(line) < 2 || line[1] != ':' {
			continue
		}

		tag := line[0:1]
		val := strings.TrimSpace(line[2:])

		applyAPKField(&cur, tag, val)
	}

	// Flush last stanza (file may not end with blank line).
	idx.flushAPKPackage(&cur, repoBaseURL)

	return scanner.Err()
}

// applyAPKField applies a parsed APKINDEX field to a Package struct.
// Handles all single-character field tags used in APKINDEX format.
// Dispatches to applyAPKBasicField or applyAPKDepField based on tag type.
func applyAPKField(pkg *Package, tag, val string) {
	if applyAPKBasicField(pkg, tag, val) {
		return
	}

	applyAPKDepField(pkg, tag, val)
}

// applyAPKBasicField handles scalar APKINDEX fields (P, V, A, S, I, T, U, L, o, m, C).
// Returns true if the field was handled, false otherwise.
func applyAPKBasicField(pkg *Package, tag, val string) bool {
	switch tag {
	case "P":
		pkg.Name = val
		return true
	case "V":
		pkg.Version = val
		return true
	case "A":
		pkg.Arch = val
		return true
	case "S":
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			pkg.Size = n
		}

		return true
	case "I":
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			pkg.InstSize = n
		}

		return true
	case "T":
		pkg.Description = val
		return true
	case "U":
		pkg.URL = val
		return true
	case "L":
		pkg.License = val
		return true
	case "o":
		pkg.Origin = val
		return true
	case "m":
		pkg.Maintainer = val
		return true
	case "C":
		pkg.Checksum = val
		return true
	}

	return false
}

// applyAPKDepField handles dependency list APKINDEX fields (D, p).
func applyAPKDepField(pkg *Package, tag, val string) {
	switch tag {
	case "D":
		pkg.Depends = strings.Fields(val)
	case "p":
		pkg.Provides = strings.Fields(val)
	}
}

// flushAPKPackage registers a completed Package in the index.
// Implements first-winning strategy: only adds if not already present.
// Also registers virtual packages (Provides).
func (idx *Index) flushAPKPackage(pkg *Package, repoBaseURL string) {
	if pkg.Name == "" {
		return
	}

	pkgCopy := *pkg // copy

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// First-winning strategy: only add if not already present.
	if _, ok := idx.packages[pkgCopy.Name]; !ok {
		idx.packages[pkgCopy.Name] = &pkgCopy
	}

	// Register virtual packages (Provides).
	for _, p := range pkgCopy.Provides {
		vname := stripVersionConstraint(p)
		if vname != "" {
			idx.providers[vname] = append(idx.providers[vname], &pkgCopy)
		}
	}

	// Reset for next stanza.
	*pkg = Package{RepoBaseURL: repoBaseURL}
}
