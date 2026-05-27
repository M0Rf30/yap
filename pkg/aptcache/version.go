// Debian version comparison.
//
// Implements the algorithm described in deb-version(7): split into
// epoch / upstream version / debian revision, then alternately compare
// non-digit and digit chunks. In the non-digit ordering, '~' sorts
// before the empty string, then letters, then everything else.

package aptcache

import (
	"strconv"
	"strings"
)

// CompareDebVersion returns -1, 0, or +1 comparing two Debian version strings
// per the deb-version(7) algorithm. Empty strings sort lowest.
func CompareDebVersion(a, b string) int {
	if a == b {
		return 0
	}

	if a == "" {
		return -1
	}

	if b == "" {
		return 1
	}

	epochA, upstreamA, revA := splitDebVersion(a)
	epochB, upstreamB, revB := splitDebVersion(b)

	if c := compareInt(epochA, epochB); c != 0 {
		return c
	}

	if c := compareVersionPart(upstreamA, upstreamB); c != 0 {
		return c
	}

	return compareVersionPart(revA, revB)
}

// splitDebVersion breaks "[epoch:]upstream[-revision]" into (epoch, upstream, revision).
// An absent epoch is treated as 0. An absent revision is "".
func splitDebVersion(v string) (epoch int, upstream, revision string) {
	if i := strings.IndexByte(v, ':'); i >= 0 {
		// epoch is digits only; if parse fails, treat ':' as part of upstream.
		if n, err := strconv.Atoi(v[:i]); err == nil {
			epoch = n
			v = v[i+1:]
		}
	}

	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		upstream = v[:i]
		revision = v[i+1:]

		return epoch, upstream, revision
	}

	return epoch, v, ""
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// compareVersionPart compares upstream or revision strings per Debian rules.
// It alternates non-digit and digit chunks until one diverges.
func compareVersionPart(a, b string) int {
	for {
		// Non-digit chunk.
		na, ra := splitNonDigit(a)
		nb, rb := splitNonDigit(b)

		if c := compareNonDigit(na, nb); c != 0 {
			return c
		}

		// Digit chunk.
		da, ra2 := splitDigit(ra)
		db, rb2 := splitDigit(rb)

		if c := compareDigit(da, db); c != 0 {
			return c
		}

		if ra2 == "" && rb2 == "" {
			return 0
		}

		a, b = ra2, rb2
	}
}

// splitNonDigit returns (nonDigitPrefix, rest) — rest starts with a digit or is empty.
func splitNonDigit(s string) (prefix, rest string) {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return s[:i], s[i:]
		}
	}

	return s, ""
}

// splitDigit returns (digitPrefix, rest) — rest starts with a non-digit or is empty.
func splitDigit(s string) (prefix, rest string) {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return s[:i], s[i:]
		}
	}

	return s, ""
}

// compareNonDigit compares two non-digit chunks character by character using
// the Debian ordering: '~' < empty < letter < other.
func compareNonDigit(a, b string) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var ca, cb int

		if i < len(a) {
			ca = debCharOrder(a[i])
		} else {
			ca = debCharOrder(0) // empty
		}

		if i < len(b) {
			cb = debCharOrder(b[i])
		} else {
			cb = debCharOrder(0) // empty
		}

		if ca != cb {
			return compareInt(ca, cb)
		}
	}

	return 0
}

// debCharOrder maps a byte to its sort key for non-digit comparison.
// Order: '~' < (empty) < letters < everything else (by ASCII among same class).
// We use a 4-digit composite so within-class ASCII order is preserved.
func debCharOrder(c byte) int {
	switch {
	case c == '~':
		return 0<<16 | int(c)
	case c == 0:
		return 1 << 16 // empty / end of string
	case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
		return 2<<16 | int(c)
	default:
		return 3<<16 | int(c)
	}
}

// compareDigit compares two digit strings numerically, ignoring leading zeros.
func compareDigit(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")

	if len(a) != len(b) {
		return compareInt(len(a), len(b))
	}

	return strings.Compare(a, b)
}
