package vfs

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemFs(t *testing.T) {
	fs := NewMemFs()
	startPerm := os.FileMode(0644)
	endPerm := os.FileMode(0755)

	filename := "file_test.txt"
	size := int64(blocksize*3 - 42)
	want := make([]byte, size)
	n, err := rand.Read(want)
	if int64(n) < size || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// check for not exists...
	t.Run("check for ErrNotExist", func(t *testing.T) {
		_, err := fs.Stat(filename)
		if err != ErrNotExist {
			t.Errorf("Expected ErrNotExist got %v", err)
		}

		_, err = fs.Open(filename)
		if err != ErrNotExist {
			t.Errorf("Expected ErrNotExist got %v", err)
		}

		err = fs.Chmod(filename, 000)
		if err != ErrNotExist {
			t.Errorf("Expected ErrNotExist got %v", err)
		}

	})

	t.Run("write file", func(t *testing.T) {
		err := fs.WriteFile(filename, want, startPerm)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("stat file", func(t *testing.T) {
		got, err := fs.Stat(filename)
		if err == nil {
			if got.Name() != filepath.Base(filename) {
				t.Errorf("Wanted %v got %v", filepath.Base(filename), got.Name())
			}

			if got.Size() != size {
				t.Errorf("Wanted %d got %d", size, got.Size())
			}

			if got.Mode() != startPerm {
				t.Errorf("Wanted %v got %v", startPerm, got.Mode())
			}

			tim := time.Time{}
			if got.ModTime() == tim {
				t.Errorf("Wanted non-zero time")
			}

			if got.IsDir() == true {
				t.Errorf("Wanted false got true")
			}

			if got.Sys() != nil {
				t.Errorf("Wanted nil got %v", got.Sys())
			}
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("read file", func(t *testing.T) {
		got, err := fs.ReadFile(filename)
		if err == nil {
			if !bytes.Equal(want, got) {
				t.Errorf("Didn't read expected data")
				t.Logf("Wanted:\n%s\nGot:\n%s\n", hex.Dump(want), hex.Dump(got))
			}
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("append file", func(t *testing.T) {
		rws, err := fs.OpenFile(filename, O_WRONLY|O_APPEND, 0)
		if err == nil {
			app := make([]byte, 42)
			n, err := rand.Read(app)
			if int64(n) < 42 || err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			want = append(want, app...)
			rws.Write(app)

			got, _ := fs.ReadFile(filename)
			if !bytes.Equal(want, got) {
				t.Errorf("Files do not match after append")
			}
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("chmod file", func(t *testing.T) {
		err := fs.Chmod(filename, endPerm)
		if err == nil {
			var fi os.FileInfo
			fi, err = fs.Stat(filename)
			if err == nil && fi.Mode().Perm() != endPerm {
				t.Errorf("Wanted %v got %v", fi.Mode().Perm(), endPerm)
			}
		}

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}
