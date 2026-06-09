package signing

import (
	"fmt"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// SignerName extracts a human-readable identity (e.g. "Fedora Project
// <[email protected]>" or "Ubuntu Archive Automatic Signing Key (2018)
// <[email protected]>") from a verified OpenPGP entity, or returns
// the hex key ID when no UID is present. Shared by the apt and rpm
// repository/package verifiers.
func SignerName(e *openpgp.Entity) string {
	if e == nil {
		return ""
	}

	for name := range e.Identities {
		return name
	}

	return fmt.Sprintf("%X", e.PrimaryKey.KeyId)
}
