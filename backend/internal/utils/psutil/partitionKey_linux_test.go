//go:build linux

package psutil

import (
	"testing"
)

func TestUnescapeMountinfoPath(t *testing.T) {
	cases := map[string]string{
		`/mnt/my\040disk`: "/mnt/my disk",
		`/a\011b`:         "/a\tb",
		`/a\012b`:         "/a\nb",
		`/a\134b`:         `/a\b`,
	}
	for in, want := range cases {
		if got := unescapeMountinfoPath(in); got != want {
			t.Errorf("unescapeMountinfoPath(%q) = %q, want %q", in, got, want)
		}
	}
}
