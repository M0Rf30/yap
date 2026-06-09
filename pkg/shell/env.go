package shell

import (
	"os"
	"strings"
)

// FilterEnv returns the subset of os.Environ() whose keys are present in
// allowList, then appends "key=value" for every defaults entry whose key
// is still missing. Package installers use this to build the minimal,
// secret-free environment forwarded to maintainer scriptlets: everything
// not allow-listed (CI secrets, signing passphrases, cloud credentials)
// is stripped so a malicious or buggy scriptlet cannot exfiltrate it.
//
// Each format keeps its own allow-list and defaults (dpkg and rpm filter
// slightly different sets); this helper provides the shared mechanics.
func FilterEnv(allowList map[string]bool, defaults map[string]string) []string {
	parent := os.Environ()
	filtered := make([]string, 0, len(parent))

	for _, kv := range parent {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}

		if allowList[k] {
			filtered = append(filtered, kv)
		}
	}

	for key, val := range defaults {
		if !HasEnvKey(filtered, key) {
			filtered = append(filtered, key+"="+val)
		}
	}

	return filtered
}

// HasEnvKey reports whether an environment variable key exists in a list
// of "key=value" entries.
func HasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}

	return false
}
