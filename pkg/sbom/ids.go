package sbom

import (
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 4122 name-based UUIDs mandate SHA-1; not a security use.
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// yapNamespaceUUID is the namespace used to derive reproducible name-based
// (version 5) UUIDs when SOURCE_DATE_EPOCH is set.
var yapNamespaceUUID = [16]byte{
	0x6b, 0xa7, 0xb8, 0x11, 0x9d, 0xad, 0x11, 0xd1,
	0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
}

// documentTime returns the creation timestamp for SBOM documents, honouring
// SOURCE_DATE_EPOCH for reproducible builds.
func documentTime() time.Time {
	return files.SourceDateEpochFromEnv().UTC()
}

// reproducible reports whether SOURCE_DATE_EPOCH requests reproducible output.
func reproducible() bool {
	return os.Getenv("SOURCE_DATE_EPOCH") != ""
}

// newSerialUUID returns an RFC 4122 UUID string for the given package.
// With SOURCE_DATE_EPOCH set it is a deterministic version 5 UUID derived from
// the package identity and epoch; otherwise it is a random version 4 UUID.
func newSerialUUID(pkg *pkgbuild.PKGBUILD) string {
	var id [16]byte

	if reproducible() {
		h := sha1.New() //nolint:gosec // see import comment
		_, _ = h.Write(yapNamespaceUUID[:])
		_, _ = fmt.Fprintf(h, "%s/%s/%s/%s", pkg.PkgName, pkg.PkgVer, pkg.PkgRel,
			os.Getenv("SOURCE_DATE_EPOCH"))
		copy(id[:], h.Sum(nil))

		id[6] = (id[6] & 0x0f) | 0x50
	} else {
		if _, err := rand.Read(id[:]); err != nil {
			// crypto/rand failing is practically impossible; fall back to
			// a time-derived value so the serial number stays well-formed.
			copy(id[:], fmt.Appendf(nil, "%016x", time.Now().UnixNano()))
		}

		id[6] = (id[6] & 0x0f) | 0x40
	}

	id[8] = (id[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}

// purlName percent-encodes a name for use in a Package URL.
func purlName(name string) string {
	return strings.ReplaceAll(url.PathEscape(name), "+", "%2B")
}

// spdxIDGenerator hands out unique, schema-valid SPDX identifiers.
type spdxIDGenerator struct {
	used map[string]bool
}

func newSPDXIDGenerator() *spdxIDGenerator {
	return &spdxIDGenerator{used: map[string]bool{}}
}

// next returns a unique SPDXRef-<prefix>-<sanitised name> identifier. SPDX
// idstrings may only contain letters, digits, '.' and '-'.
func (g *spdxIDGenerator) next(prefix, name string) string {
	var sb strings.Builder

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteByte('-')
		}
	}

	base := "SPDXRef-" + prefix + "-" + sb.String()
	id := base

	for n := 1; g.used[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}

	g.used[id] = true

	return id
}

// dependency is a de-duplicated dependency name.
type dependency struct {
	name    string
	runtime bool
}

// collectDependencies returns unique dependency names: runtime dependencies
// first (in declaration order), then make dependencies not already listed.
func collectDependencies(pkg *pkgbuild.PKGBUILD) []dependency {
	seen := map[string]bool{}

	var deps []dependency

	add := func(list []string, runtime bool) {
		for _, raw := range list {
			name := extractDepName(raw)
			if name == "" || seen[name] {
				continue
			}

			seen[name] = true

			deps = append(deps, dependency{name: name, runtime: runtime})
		}
	}

	add(pkg.Depends, true)
	add(pkg.MakeDepends, false)

	return deps
}

// hashFromSum converts a PKGBUILD checksum into a CycloneDX hash, inferring
// the algorithm from the hex digest length. It returns nil for SKIP or
// unrecognised values.
func hashFromSum(sum string) *CycloneDXHash {
	sum = strings.ToLower(strings.TrimSpace(sum))

	for _, c := range sum {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return nil
		}
	}

	var alg string

	switch len(sum) {
	case 32:
		alg = "MD5"
	case 40:
		alg = "SHA-1"
	case 64:
		alg = "SHA-256"
	case 96:
		alg = "SHA-384"
	case 128:
		alg = "SHA-512"
	default:
		return nil
	}

	return &CycloneDXHash{Alg: alg, Value: sum}
}
