package vfs_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/mh-orange/vfs"
)

func testMkdirAll(fs vfs.FileSystem, dir string) func(t *testing.T) {
	return func(t *testing.T) {
		notExist := []string{}
		origDir := dir
		for dir != "/" && dir != "" {
			_, err := fs.Stat(dir)
			if vfs.IsNotExist(err) {
				notExist = append(notExist, dir)
			} else if err != nil {
				t.Errorf("Failed to stat %q: %v", dir, err)
				return
			}
			dir, _ = path.Split(dir)
		}

		err := vfs.MkdirAll(fs, origDir, 0755)
		if err == nil {
			for dir = origDir; dir != "/" && dir != ""; dir, _ = path.Split(dir) {
				_, err = fs.Stat(dir)
				if err != nil {
					t.Errorf("Failed to stat %q: %v", dir, err)
					return
				}
			}
		} else {
			t.Errorf("MkdirAll failed: %v", err)
		}
	}
}

func testReadFile(fs vfs.FileSystem, filename string, want []byte) func(t *testing.T) {
	return func(t *testing.T) {
		got, err := vfs.ReadFile(fs, filename)
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

func testWriteFile(fs vfs.FileSystem, filename string, content []byte, perm os.FileMode) func(t *testing.T) {
	return func(t *testing.T) {
		t.Run("mkdir all", testMkdirAll(fs, path.Dir(filename)))
		err := vfs.WriteFile(fs, filename, content, perm)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func testAppendFile(fs vfs.FileSystem, filename string, want []byte) func(t *testing.T) {
	return func(t *testing.T) {
		rws, err := fs.OpenFile(filename, vfs.WrOnlyFlag|vfs.AppendFlag, 0)
		if err == nil {
			data := make([]byte, 42)
			n, err := rand.Read(data)
			if int64(n) < 42 || err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			want = append(want, data...)
			rws.Write(data)

			got, _ := vfs.ReadFile(fs, filename)
			if !bytes.Equal(want, got) {
				t.Errorf("Files do not match after append")
			}
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func testMkdir(fs vfs.FileSystem, dirname string) func(t *testing.T) {
	return func(t *testing.T) {
		_, err := fs.Stat(dirname)
		if !vfs.IsNotExist(err) {
			t.Errorf("Expected ErrNotExist got %v", err)
		}

		err = fs.Mkdir(dirname, 0755)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		fi, err := fs.Stat(dirname)
		if err == nil {
			if fi.IsDir() == false {
				t.Errorf("Expected IsDir to return true")
			}

			want := os.FileMode(0755)
			got := fi.Mode() & os.ModePerm
			if got != want {
				t.Errorf("Want %v got %v", want, got)
			}
		} else {
			t.Errorf("Expected no error got %v", err)
		}
	}
}

func testNotExist(fs vfs.FileSystem, filename string) func(t *testing.T) {
	return func(t *testing.T) {
		_, err := fs.Stat(filename)
		if !vfs.IsNotExist(err) {
			t.Errorf("Expected ErrNotExist got %v", err)
		}

		_, err = fs.Open(filename)
		if !vfs.IsNotExist(err) {
			t.Errorf("Expected ErrNotExist got %v", err)
		}

		err = fs.Chmod(filename, 000)
		if !vfs.IsNotExist(err) {
			t.Errorf("Expected ErrNotExist got %v", err)
		}
	}
}

func testCreateFile(fs vfs.FileSystem, filename string) func(t *testing.T) {
	return func(t *testing.T) {
		if _, err := fs.Stat(filename); vfs.IsNotExist(err) {
			if _, err := fs.Create(filename); err == nil {
				if _, err := fs.Stat(filename); err != nil {
					t.Errorf("Stat file failed: %v", err)
				}
			} else {
				t.Errorf("Create file failed: %v", err)
			}
		} else {
			t.Errorf("Expected Error IsNotExist got %v", err)
		}
	}
}

func testStatFile(fs vfs.FileSystem, filename string, wantSize int64, wantPerm os.FileMode, wantErr error) func(t *testing.T) {
	return func(t *testing.T) {
		got, err := fs.Stat(filename)
		if err == nil {
			if got.Name() != path.Base(filename) {
				t.Errorf("Wanted %v got %v", path.Base(filename), got.Name())
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
		} else if err != wantErr {
			t.Errorf("Unexpected error wanted %v got %v", wantErr, err)
		}
	}
}

func testChmodFile(fs vfs.FileSystem, filename string, want os.FileMode) func(t *testing.T) {
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

func testRemoveFile(fs vfs.FileSystem, filename string) func(t *testing.T) {
	return func(t *testing.T) {
		t.Run("create file", testCreateFile(fs, filename))
		err := fs.Remove(filename)

		if err != nil {
			t.Errorf("Failed to remove file: %v", err)
		}

		if _, err = fs.Stat(filename); !vfs.IsNotExist(err) {
			t.Errorf("Expected file to not exist, got %v", err)
		}
	}
}

func testRenameFile(fs vfs.FileSystem, oldFilename, newFilename string) func(t *testing.T) {
	return func(t *testing.T) {
		t.Run("create file", testCreateFile(fs, oldFilename))
		err := fs.Rename(oldFilename, newFilename)

		if err != nil {
			t.Errorf("Failed to rename file: %v", err)
		}

		if _, err = fs.Stat(oldFilename); !vfs.IsNotExist(err) {
			t.Errorf("Expected old file to not exist, got %v", err)
		}

		if _, err = fs.Stat(newFilename); err != nil {
			t.Errorf("Expected new file to exist, got %v", err)
		}
	}
}

func TestVFS(t *testing.T) {
	for _, fs := range []vfs.FileSystem{vfs.NewMemFs(), vfs.NewTempFs()} {
		t.Run(fmt.Sprintf("%T", fs), func(t *testing.T) {
			startPerm := os.FileMode(0644)
			endPerm := os.FileMode(0755)

			writeFile := "/tmp/write_file_test.txt"
			createFile := "/tmp/foo/create_file_test.txt"
			removeFile := "remove_file_test.txt"
			renameFile := "rename_file_test.txt"
			size := int64(4000)
			want := make([]byte, size)
			n, err := rand.Read(want)
			if int64(n) < size || err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			t.Run(fmt.Sprintf("mkdir %s", path.Dir(writeFile)), testMkdir(fs, path.Dir(writeFile)))
			t.Run(fmt.Sprintf("mkdir %s", path.Dir(createFile)), testMkdir(fs, path.Dir(createFile)))
			t.Run("stat file", testNotExist(fs, writeFile))
			t.Run("write file", testWriteFile(fs, writeFile, want, startPerm))
			t.Run("create file", testCreateFile(fs, createFile))
			t.Run("rename file", testRenameFile(fs, removeFile, renameFile))
			t.Run("remove file", testRemoveFile(fs, removeFile))
			t.Run("stat file", testStatFile(fs, writeFile, size, startPerm, nil))
			t.Run("read file", testReadFile(fs, writeFile, want))
			t.Run("append file", testAppendFile(fs, writeFile, want))
			t.Run("chmod file", testChmodFile(fs, writeFile, endPerm))
			fs.Close()
		})
	}
}
