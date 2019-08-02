package vfs

import (
	"testing"
)

func TestOsPath(t *testing.T) {
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

func TestOsWatcher(t *testing.T) {
	fs := NewTempFs()
	watcher, err := fs.Watcher(make(chan Event))
	if err == nil {
		// make sure we can close it, that's about the most we can do.. at least
		// it makes sure the go routines exit since they can't exit until writing
		// to the closer channel
		watcher.Close()
	} else {
		t.Errorf("Unexpected error: %v", err)
	}
}
