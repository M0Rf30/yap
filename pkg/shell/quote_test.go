package shell_test

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/shell"
)

func TestSingleQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "''"},
		{"hello", "'hello'"},
		{"with space", "'with space'"},
		{"it's", `'it'\''s'`},
		{"a'b'c", `'a'\''b'\''c'`},
		{"$VAR", "'$VAR'"},
		{"line1\nline2", "'line1\nline2'"},
	}

	for _, c := range cases {
		got := shell.SingleQuote(c.in)
		if got != c.want {
			t.Errorf("SingleQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestJoin(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"a"}, "'a'"},
		{[]string{"a", "b"}, "'a' 'b'"},
		{[]string{"yap", "build", "ubuntu-noble", "/project"},
			"'yap' 'build' 'ubuntu-noble' '/project'"},
		{[]string{"--msg", "hi there"}, "'--msg' 'hi there'"},
		{[]string{"it's", "ok"}, `'it'\''s' 'ok'`},
	}

	for _, c := range cases {
		got := shell.Join(c.in)
		if got != c.want {
			t.Errorf("Join(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
