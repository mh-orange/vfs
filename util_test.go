package vfs

import (
	"bytes"
	"io"
	"os"
	"path"
	"reflect"
	"sort"
	"testing"
)

type testFs struct {
	closed   bool
	got      []byte
	want     []byte
	dirnames []string
}

func (tf *testFs) Read(data []byte) (n int, err error) {
	n = copy(data, tf.want)
	if n <= len(data) {
		err = io.EOF
	}
	return
}

func (tf *testFs) Write(data []byte) (n int, err error) {
	return copy(tf.got, data), nil
}

func (tf *testFs) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (tf *testFs) Close() error {
	tf.closed = true
	return nil
}

func (tf *testFs) Chmod(filename string, perm os.FileMode) error { return nil }

func (tf *testFs) Create(filename string) (File, error) {
	return tf.OpenFile(filename, 0, 0)
}

func (tf *testFs) Open(filename string) (File, error) { return tf.OpenFile(filename, 0, 0) }

func (tf *testFs) OpenFile(filename string, flags OpenFlag, perm os.FileMode) (File, error) {
	return tf, nil
}

func (tf *testFs) Name() string                               { return "" }
func (tf *testFs) Readdirnames(n int) ([]string, error)       { return tf.dirnames, nil }
func (tf *testFs) Readdir(n int) ([]os.FileInfo, error)       { return nil, nil }
func (tf *testFs) Remove(name string) error                   { return nil }
func (tf *testFs) Rename(old, new string) error               { return nil }
func (tf *testFs) Mkdir(name string, perm os.FileMode) error  { return nil }
func (tf *testFs) Lstat(filename string) (os.FileInfo, error) { return nil, nil }
func (tf *testFs) Stat(filename string) (os.FileInfo, error)  { return nil, nil }

func TestUtilWriteFile(t *testing.T) {
	tests := []struct {
		got     []byte
		want    []byte
		wantErr error
	}{
		{make([]byte, 5), []byte{1, 2, 3, 4, 5}, nil},
		{make([]byte, 2), []byte{1, 2, 3, 4, 5}, nil},
	}

	for _, test := range tests {
		fs := &testFs{got: test.got}
		err := WriteFile(fs, "test file", test.want, 0)
		if err == test.wantErr {
			if err == nil {
				if !fs.closed {
					t.Errorf("Expected closed")
				}

				if !bytes.Equal(test.want, fs.got) {
					t.Errorf("Expected %x Got %x\n", test.want, fs.got)
				}
			}
		} else if err != nil {
		}
	}
}

func TestUtilReadFile(t *testing.T) {
	tests := []struct {
		want    []byte
		wantErr error
	}{
		{[]byte{1, 2, 3, 4, 5}, nil},
		{[]byte{1, 2, 3, 4, 5}, nil},
	}

	for _, test := range tests {
		fs := &testFs{want: test.want}
		got, err := ReadFile(fs, "test file")
		if err == test.wantErr {
			if err == nil {
				if !fs.closed {
					t.Errorf("Expected closed")
				}

				if !bytes.Equal(test.want, got) {
					t.Errorf("Expected %x Got %x\n", test.want, got)
				}
			}
		} else if err != nil {
		}
	}
}

func TestUtilWalk(t *testing.T) {
	tests := []struct {
		name     string
		root     string
		match    string
		treeDir  []string
		treeFile []string
		walkErr  error
		want     []string
		wantErr  error
	}{
		{
			name:     "happy path",
			root:     "/",
			treeDir:  []string{"/one/two/three"},
			treeFile: []string{"/one/1.txt", "/one/two/2.txt", "/one/two/three/3.txt"},
			want:     []string{"/", "/one", "/one/1.txt", "/one/two", "/one/two/2.txt", "/one/two/three", "/one/two/three/3.txt"},
		},
		{
			name:    "root error",
			root:    "/one",
			wantErr: ErrNotExist,
		},
		{
			name:    "root error",
			root:    "/one",
			match:   "/one",
			want:    []string{"/one"},
			walkErr: ErrSkipDir,
		},
		{
			name:     "skip dir",
			treeDir:  []string{"/one/two/three"},
			treeFile: []string{"/one/1.txt", "/one/two/2.txt", "/one/two/three/3.txt"},
			root:     "/",
			match:    "/one/two",
			walkErr:  ErrSkipDir,
			want:     []string{"/", "/one", "/one/1.txt", "/one/two"},
			wantErr:  nil,
		},
		{
			name:    "walk error",
			treeDir: []string{"/one/two/three"},
			root:    "/",
			match:   "/one/two",
			walkErr: ErrNotExist,
			want:    []string{"/", "/one", "/one/two"},
			wantErr: ErrNotExist,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewTempFs()
			if closer, ok := fs.(io.Closer); ok {
				defer closer.Close()
			}

			for _, path := range test.treeDir {
				MkdirAll(fs, path, 0777)
			}

			for _, path := range test.treeFile {
				fs.Create(path)
			}

			var got []string
			walkFn := func(path string, info os.FileInfo, err error) error {
				got = append(got, path)
				if test.match != "" && test.match == path {
					err = test.walkErr
				}
				return err
			}
			sort.Strings(got)

			// Expect no errors.
			gotErr := Walk(fs, test.root, walkFn)
			if pe, ok := gotErr.(*PathError); ok {
				gotErr = pe.Cause
			}

			if gotErr == nil {
				if test.wantErr == nil {
					if !reflect.DeepEqual(test.want, got) {
						t.Errorf("Wanted paths %s got %s", test.want, got)
					}
				} else {
					t.Errorf("Wanted error %v got %v", test.wantErr, gotErr)
				}
			} else {
				if test.wantErr == nil || gotErr.Error() != test.wantErr.Error() {
					t.Errorf("Wanted error %v got %v", test.wantErr, gotErr)
				}
			}
		})
	}
}

func TestUtilMkdirAll(t *testing.T) {
	fs := NewTempFs()
	if closer, ok := fs.(io.Closer); ok {
		defer closer.Close()
	}
	dir := "/_TestMkdirAll_/dir/./dir2"
	err := MkdirAll(fs, dir, 0777)
	if err != nil {
		t.Fatalf("MkdirAll %q: %s", dir, err)
	}

	// Already exists, should succeed.
	err = MkdirAll(fs, dir, 0777)
	if err != nil {
		t.Fatalf("MkdirAll %q (second time): %s", dir, err)
	}

	// Make file.
	fpath := dir + "/file"
	f, err := fs.Create(fpath)
	if err != nil {
		t.Fatalf("create %q: %s", fpath, err)
	}
	if closer, ok := f.(io.Closer); ok {
		defer closer.Close()
	}

	// Can't make directory named after file.
	err = MkdirAll(fs, fpath, 0777)
	if err == nil {
		t.Fatalf("MkdirAll %q: no error", fpath)
	}
	perr, ok := err.(*PathError)
	if !ok {
		t.Fatalf("MkdirAll %q returned %T, not *PathError", fpath, err)
	}
	if path.Clean(perr.Path) != path.Clean(fpath) {
		t.Fatalf("MkdirAll %q returned wrong error path: %q not %q", fpath, path.Clean(perr.Path), path.Clean(fpath))
	}

	// Can't make subdirectory of file.
	ffpath := fpath + "/subdir"
	err = MkdirAll(fs, ffpath, 0777)
	if err == nil {
		t.Fatalf("MkdirAll %q: no error", ffpath)
	}
	perr, ok = err.(*PathError)
	if !ok {
		t.Fatalf("MkdirAll %q returned %T, not *PathError", ffpath, err)
	}
	if path.Clean(perr.Path) != path.Clean(fpath) {
		t.Fatalf("MkdirAll %q returned wrong error path: %q not %q", ffpath, path.Clean(perr.Path), path.Clean(fpath))
	}
}

func TestGlob(t *testing.T) {
	fs := NewTempFs()
	fs.Create("foo.bar")
	fs.Create("fubar.go")
	fs.Mkdir("/fun", 0750)
	fs.Create("/fun/foo.bar")
	tests := []struct {
		pattern string
		result  []string
	}{
		{"/foo.bar", []string{"/foo.bar"}},
		{"/f?o.bar", []string{"/foo.bar"}},
		{"/*", []string{"/foo.bar", "/fubar.go", "/fun"}},
		{"/*/foo.bar", []string{"/fun/foo.bar"}},
	}

	for _, tt := range tests {
		pattern := tt.pattern
		result := tt.result
		matches, err := Glob(fs, pattern)
		if err != nil {
			t.Errorf("Glob error for %q: %s", pattern, err)
			continue
		}
		if !reflect.DeepEqual(result, matches) {
			t.Errorf("Glob(%#q) = %#v want %v", pattern, matches, result)
		}
	}

	for _, pattern := range []string{"no_match", "../*/no_match"} {
		matches, err := Glob(fs, pattern)
		if err != nil {
			t.Errorf("Glob error for %q: %s", pattern, err)
			continue
		}
		if len(matches) != 0 {
			t.Errorf("Glob(%#q) = %#v want []", pattern, matches)
		}
	}
	fs.Close()
}
