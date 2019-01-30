package vfs

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
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
			fs := &osFs{test.root}
			got := fs.path(test.input)
			if test.want != got {
				t.Errorf("Wanted %q got %q", test.want, got)
			}
		})
	}
}

func TestOsFs(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "osfs_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	fs := NewOsFs(tmpdir)
	startPerm := os.FileMode(0644)
	endPerm := os.FileMode(0755)

	filename := "file_test.txt"
	want := []byte("Hello, playground\n")
	t.Run("write file", func(t *testing.T) {
		got := []byte{}
		err := fs.WriteFile(filename, want, startPerm)
		if err == nil {
			got, err = ioutil.ReadFile(filepath.Join(tmpdir, filename))
			if err == nil {
				if !bytes.Equal(want, got) {
					t.Errorf("Wanted %q got %q", string(want), string(got))
				}
			}
		}

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("read file", func(t *testing.T) {
		got, err := fs.ReadFile(filename)
		if err == nil {
			if !bytes.Equal(want, got) {
				t.Errorf("Wanted %q got %q", string(want), string(got))
			}
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("stat file", func(t *testing.T) {
		want, err := os.Stat(filepath.Join(tmpdir, filename))
		if err == nil {
			var got os.FileInfo
			got, err = fs.Stat(filename)
			if !reflect.DeepEqual(want, got) {
				t.Errorf("Wanted %+v got %+v", want, got)
			}
		}

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("chmod file", func(t *testing.T) {
		err := fs.Chmod(filename, endPerm)
		if err == nil {
			var fi os.FileInfo
			fi, err = os.Stat(filepath.Join(tmpdir, filename))
			if fi.Mode().Perm() != endPerm {
				t.Errorf("Wanted %v got %v", fi.Mode().Perm(), endPerm)
			}
		}

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}
