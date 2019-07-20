package vfs

import (
	"testing"
)

func TestOsFsPath(t *testing.T) {
	tests := []struct {
		root  string
		input string
		want  string
	}{
		{"/tmp", "", "/tmp"},
		{"/tmp", "../../foo", "/tmp/foo"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			fs := &osfs{test.root}
			got := fs.path(test.input)
			if test.want != got {
				t.Errorf("Wanted %q got %q", test.want, got)
			}
		})
	}
}
