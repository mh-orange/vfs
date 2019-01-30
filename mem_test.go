package vfs

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadWriteSeekerSeek(t *testing.T) {
	tests := []struct {
		size    int64
		current int64
		offset  int64
		whence  int
		want    int64
		wantErr error
	}{
		{100, 0, 10, io.SeekStart, 10, nil},
		{100, 0, 10, io.SeekEnd, 110, nil},
		{100, 50, 10, io.SeekCurrent, 60, nil},
		{100, 10, 10, 42, 10, ErrWhence},
		{100, 0, -10, io.SeekStart, 60, ErrInvalidSeek},
	}

	for _, test := range tests {
		rws := &readWriteSeeker{file: &memFile{size: test.size}, offset: test.current}
		n, err := rws.Seek(test.offset, test.whence)
		if err == test.wantErr {
			if err == nil {
				if test.want != rws.offset || test.want != n {
					t.Errorf("Expected %d got %d", test.want, n)
				}
			}
		} else {
			t.Errorf("Expected %v got %v", test.wantErr, err)
		}
	}
}

func testMemFsNotExist(fs *MemFs, filename string) func(t *testing.T) {
	return func(t *testing.T) {
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
	}
}

func testMemFsWriteFile(fs *MemFs, filename string, content []byte, perm os.FileMode) func(t *testing.T) {
	return func(t *testing.T) {
		err := fs.WriteFile(filename, content, perm)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func testMemFsCreateFile(fs *MemFs, filename string) func(t *testing.T) {
	return func(t *testing.T) {
		fs.Create(filename)
		if _, found := fs.files[filename]; !found {
			t.Errorf("Create file failed")
		}
	}
}

func testMemFsStatFile(fs *MemFs, filename string, wantSize int64, wantPerm os.FileMode, wantErr error) func(t *testing.T) {
	return func(t *testing.T) {
		got, err := fs.Stat(filename)
		if err == nil {
			if got.Name() != filepath.Base(filename) {
				t.Errorf("Wanted %v got %v", filepath.Base(filename), got.Name())
			}

			if got.Size() != wantSize {
				t.Errorf("Wanted %d got %d", wantSize, got.Size())
			}

			if got.Mode() != wantPerm {
				t.Errorf("Wanted %v got %v", wantPerm, got.Mode())
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
		} else if err != wantErr {
			t.Errorf("Unexpected error wanted %v got %v", wantErr, err)
		}
	}
}

func testMemFsReadFile(fs *MemFs, filename string, want []byte) func(t *testing.T) {
	return func(t *testing.T) {
		got, err := fs.ReadFile(filename)
		if err == nil {
			if !bytes.Equal(want, got) {
				t.Errorf("Didn't read expected data")
				t.Logf("Wanted:\n%s\nGot:\n%s\n", hex.Dump(want), hex.Dump(got))
			}
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func testMemFsAppendFile(fs *MemFs, filename string, want []byte) func(t *testing.T) {
	return func(t *testing.T) {
		rws, err := fs.OpenFile(filename, WrOnlyFlag|AppendFlag, 0)
		if err == nil {
			data := make([]byte, 42)
			n, err := rand.Read(data)
			if int64(n) < 42 || err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			want = append(want, data...)
			rws.Write(data)

			got, _ := fs.ReadFile(filename)
			if !bytes.Equal(want, got) {
				t.Errorf("Files do not match after append")
			}
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func testMemFsChmodFile(fs *MemFs, filename string, want os.FileMode) func(t *testing.T) {
	return func(t *testing.T) {
		err := fs.Chmod(filename, want)
		if err == nil {
			var fi os.FileInfo
			fi, err = fs.Stat(filename)
			if err == nil && fi.Mode().Perm() != want {
				t.Errorf("Wanted %v got %v", fi.Mode().Perm(), want)
			}
		}

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func TestMemFs(t *testing.T) {
	fs := NewMemFs()
	startPerm := os.FileMode(0644)
	endPerm := os.FileMode(0755)

	writeFile := "write_file_test.txt"
	createFile := "create_file_test.txt"
	size := int64(blocksize*3 - 42)
	want := make([]byte, size)
	n, err := rand.Read(want)
	if int64(n) < size || err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	t.Run("stat file", testMemFsNotExist(fs, writeFile))
	t.Run("write file", testMemFsWriteFile(fs, writeFile, want, startPerm))
	t.Run("create file", testMemFsCreateFile(fs, createFile))
	t.Run("stat file", testMemFsStatFile(fs, writeFile, size, startPerm, nil))
	t.Run("read file", testMemFsReadFile(fs, writeFile, want))
	t.Run("append file", testMemFsAppendFile(fs, writeFile, want))
	t.Run("chmod file", testMemFsChmodFile(fs, writeFile, endPerm))
}
