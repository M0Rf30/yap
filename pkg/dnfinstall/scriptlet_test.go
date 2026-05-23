package dnfinstall

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFilterScriptletEnv verifies that environment filtering works correctly.
func TestFilterScriptletEnv(t *testing.T) {
	// Set some test environment variables.
	os.Setenv("TEST_ALLOWED", "yes")
	os.Setenv("PATH", "/usr/bin")
	os.Setenv("RANDOM_VAR", "should_be_filtered")

	env := filterScriptletEnv()

	// Check that PATH is present.
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			found = true
			break
		}
	}
	assert.True(t, found, "PATH should be in filtered environment")

	// Check that HOME is present.
	found = false
	for _, kv := range env {
		if strings.HasPrefix(kv, "HOME=") {
			found = true
			break
		}
	}
	assert.True(t, found, "HOME should be in filtered environment")

	// Check that random variables are filtered out.
	for _, kv := range env {
		assert.False(t, strings.HasPrefix(kv, "RANDOM_VAR="),
			"RANDOM_VAR should be filtered out")
	}
}

// TestHasEnvKey verifies environment key lookup.
func TestHasEnvKey(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"HOME=/root",
		"USER=root",
	}

	assert.True(t, hasEnvKey(env, "PATH"))
	assert.True(t, hasEnvKey(env, "HOME"))
	assert.False(t, hasEnvKey(env, "NONEXISTENT"))
}

// TestScriptletKindNames verifies that all scriptlet kinds have proper names.
func TestScriptletKindNames(t *testing.T) {
	kinds := []scriptletKind{
		scriptletPreTrans,
		scriptletPreIn,
		scriptletPostIn,
		scriptletPostTrans,
	}

	expectedNames := []string{
		"pretrans",
		"prein",
		"postin",
		"posttrans",
	}

	for i, kind := range kinds {
		tags := scriptletTags[kind]
		assert.Equal(t, expectedNames[i], tags.kindName,
			"scriptlet kind %d should have name %s", kind, expectedNames[i])
	}
}

// TestScriptletTagPairs verifies that all scriptlet kinds have valid tag pairs.
func TestScriptletTagPairs(t *testing.T) {
	for kind, tags := range scriptletTags {
		assert.NotZero(t, tags.bodyTag, "kind %d should have a body tag", kind)
		assert.NotZero(t, tags.progTag, "kind %d should have a prog tag", kind)
		assert.NotEmpty(t, tags.kindName, "kind %d should have a name", kind)
		assert.Equal(t, "1", tags.argValue, "kind %d should have argValue='1'", kind)
	}
}

// TestScriptletEnvAllowList verifies that the environment allow list is properly configured.
func TestScriptletEnvAllowList(t *testing.T) {
	// Verify that essential variables are in the allow list.
	assert.True(t, scriptletEnvAllowList["PATH"], "PATH should be allowed")
	assert.True(t, scriptletEnvAllowList["HOME"], "HOME should be allowed")
	assert.True(t, scriptletEnvAllowList["LANG"], "LANG should be allowed")
	assert.True(t, scriptletEnvAllowList["USER"], "USER should be allowed")

	// Verify that some random variable is not in the allow list.
	assert.False(t, scriptletEnvAllowList["RANDOM_VARIABLE"], "RANDOM_VARIABLE should not be allowed")
}

// TestFilterScriptletEnvMinimal verifies that PATH and HOME are always provided.
func TestFilterScriptletEnvMinimal(t *testing.T) {
	// Clear environment to test minimal case.
	oldEnv := os.Environ()
	os.Clearenv()
	defer func() {
		os.Clearenv()
		for _, kv := range oldEnv {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	env := filterScriptletEnv()

	// Check that PATH is provided even if not in parent environment.
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			found = true
			assert.Contains(t, kv, "/usr/bin", "PATH should contain /usr/bin")
			break
		}
	}
	assert.True(t, found, "PATH should be provided")

	// Check that HOME is provided.
	found = false
	for _, kv := range env {
		if strings.HasPrefix(kv, "HOME=") {
			found = true
			break
		}
	}
	assert.True(t, found, "HOME should be provided")
}

// TestScriptletTagConstants verifies that the tag constants are correct.
func TestScriptletTagConstants(t *testing.T) {
	// Verify PREIN and POSTIN tags from rpmutils.
	assert.Equal(t, 1023, scriptletTags[scriptletPreIn].bodyTag, "PREIN tag should be 1023")
	assert.Equal(t, 1085, scriptletTags[scriptletPreIn].progTag, "PREINPROG tag should be 1085")
	assert.Equal(t, 1024, scriptletTags[scriptletPostIn].bodyTag, "POSTIN tag should be 1024")
	assert.Equal(t, 1086, scriptletTags[scriptletPostIn].progTag, "POSTINPROG tag should be 1086")

	// Verify PRETRANS and POSTTRANS tags (custom constants).
	assert.Equal(t, 1151, scriptletTags[scriptletPreTrans].bodyTag, "PRETRANS tag should be 1151")
	assert.Equal(t, 1152, scriptletTags[scriptletPreTrans].progTag, "PRETRANSPROG tag should be 1152")
	assert.Equal(t, 1153, scriptletTags[scriptletPostTrans].bodyTag, "POSTTRANS tag should be 1153")
	assert.Equal(t, 1154, scriptletTags[scriptletPostTrans].progTag, "POSTTRANSPROG tag should be 1154")
}

// TestScriptletKindOrder verifies that scriptlet kinds are in the correct order.
func TestScriptletKindOrder(t *testing.T) {
	// Verify that the scriptlet kinds are defined in the correct order.
	assert.Equal(t, scriptletKind(0), scriptletPreTrans)
	assert.Equal(t, scriptletKind(1), scriptletPreIn)
	assert.Equal(t, scriptletKind(2), scriptletPostIn)
	assert.Equal(t, scriptletKind(3), scriptletPostTrans)
}

// TestHasEnvKeyEdgeCases verifies edge cases for hasEnvKey.
func TestHasEnvKeyEdgeCases(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"PATHEXT=.exe",
		"HOME=/root",
	}

	// Exact match should work.
	assert.True(t, hasEnvKey(env, "PATH"))

	// Partial match should not work (PATHEXT != PATH).
	assert.False(t, hasEnvKey(env, "PATHEX"))

	// Empty key should not match.
	assert.False(t, hasEnvKey(env, ""))

	// Empty environment should not match anything.
	assert.False(t, hasEnvKey([]string{}, "PATH"))
}

// TestFilterScriptletEnvPreservesValues verifies that filtering preserves variable values.
func TestFilterScriptletEnvPreservesValues(t *testing.T) {
	// Set a test variable that's in the allow list.
	os.Setenv("LANG", "en_US.UTF-8")

	env := filterScriptletEnv()

	// Find and verify the LANG variable.
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "LANG=") {
			found = true
			assert.Equal(t, "LANG=en_US.UTF-8", kv, "LANG value should be preserved")
			break
		}
	}
	assert.True(t, found, "LANG should be in filtered environment")
}

// TestScriptletTagPairArgValue verifies that all scriptlet kinds have argValue="1".
func TestScriptletTagPairArgValue(t *testing.T) {
	for kind, tags := range scriptletTags {
		assert.Equal(t, "1", tags.argValue,
			"scriptlet kind %d should have argValue='1' for fresh install", kind)
	}
}

// TestScriptletIntegration is a basic integration test that verifies the scriptlet
// infrastructure is properly set up.
func TestScriptletIntegration(t *testing.T) {
	// Verify that all scriptlet kinds are properly configured.
	for kind := scriptletKind(0); kind < 4; kind++ {
		tags, ok := scriptletTags[kind]
		require.True(t, ok, "scriptlet kind %d should be in scriptletTags", kind)
		require.NotEmpty(t, tags.kindName, "scriptlet kind %d should have a name", kind)
		require.NotZero(t, tags.bodyTag, "scriptlet kind %d should have a body tag", kind)
		require.NotZero(t, tags.progTag, "scriptlet kind %d should have a prog tag", kind)
	}

	// Verify that the environment filtering works.
	env := filterScriptletEnv()
	require.NotEmpty(t, env, "filtered environment should not be empty")

	// Verify that PATH is always present.
	pathFound := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			pathFound = true
			break
		}
	}
	require.True(t, pathFound, "PATH should always be in filtered environment")
}
