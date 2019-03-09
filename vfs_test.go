package vfs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenFlagCheck(t *testing.T) {
	tests := []struct {
		flag OpenFlag
		want error
	}{
		{RdOnlyFlag, nil},
		{WrOnlyFlag, nil},
		{RdWrFlag, nil},
		{WrOnlyFlag | RdWrFlag, ErrInvalidFlags},
		{RdOnlyFlag | AppendFlag, ErrInvalidFlags},
		{RdOnlyFlag | CreateFlag, ErrInvalidFlags},
		{RdOnlyFlag | ExclFlag, ErrInvalidFlags},
		{RdOnlyFlag | TruncFlag, ErrInvalidFlags},
		{RdOnlyFlag | AppendFlag | CreateFlag, ErrInvalidFlags},
		{RdWrFlag | AppendFlag, nil},
		{RdWrFlag | CreateFlag, nil},
		{RdWrFlag | ExclFlag, nil},
		{RdWrFlag, nil},
		{RdWrFlag | TruncFlag, nil},
		{WrOnlyFlag | AppendFlag, nil},
		{WrOnlyFlag | CreateFlag, nil},
		{WrOnlyFlag | ExclFlag, nil},
		{WrOnlyFlag, nil},
		{WrOnlyFlag | TruncFlag, nil},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got := test.flag.check()
			if test.want != got {
				t.Errorf("Wanted %v got %v", test.want, got)
			}
		})
	}
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

type testFs struct {
	closed   bool
	got      []byte
	want     []byte
	dirnames []string
}

func (tf *testFs) Chmod(filename string, perm os.FileMode) error { return nil }
func (tf *testFs) Create(filename string) (File, error) {
	return tf.OpenFile(filename, 0, 0)
}

func (tf *testFs) Open(filename string) (File, error) { return tf.OpenFile(filename, 0, 0) }

func (tf *testFs) OpenFile(filename string, flags OpenFlag, perm os.FileMode) (File, error) {
	return tf, nil
}

func (tf *testFs) Readdirnames(n int) ([]string, error)       { return tf.dirnames, nil }
func (tf *testFs) Remove(name string) error                   { return nil }
func (tf *testFs) Mkdir(name string, perm os.FileMode) error  { return nil }
func (tf *testFs) Lstat(filename string) (os.FileInfo, error) { return nil, nil }
func (tf *testFs) Stat(filename string) (os.FileInfo, error)  { return nil, nil }

func TestWriteFile(t *testing.T) {
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

func TestReadFile(t *testing.T) {
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

/*
func testMemFsWriteFile(fs *MemFs, filename string, content []byte, perm os.FileMode) func(t *testing.T) {
	return func(t *testing.T) {
		err := fs.WriteFile(filename, content, perm)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
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
*/

type Node struct {
	name    string
	entries []*Node // nil if the entry is a file
	mark    int
}

var tree = &Node{
	"testdata",
	[]*Node{
		{"a", nil, 0},
		{"b", []*Node{}, 0},
		{"c", nil, 0},
		{
			"d",
			[]*Node{
				{"x", nil, 0},
				{"y", []*Node{}, 0},
				{
					"z",
					[]*Node{
						{"u", nil, 0},
						{"v", nil, 0},
					},
					0,
				},
			},
			0,
		},
	},
	0,
}

func walkTree(n *Node, path string, f func(path string, n *Node)) {
	f(path, n)
	for _, e := range n.entries {
		walkTree(e, filepath.Join(path, e.name), f)
	}
}

func makeTree(t *testing.T, fs FileSystem) {
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.entries == nil {
			fd, err := fs.Create(path)
			if err != nil {
				t.Errorf("makeTree: %v", err)
				return
			}

			if closer, ok := fd.(io.Closer); ok {
				closer.Close()
			}
		} else {
			err := fs.Mkdir(path, 0770)
			if err != nil {
				t.Errorf("makeTree: %v", err)
			}
		}
	})
}

func mark(info os.FileInfo, err error, errors *[]error, clear bool) error {
	name := info.Name()
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.name == name {
			n.mark++
		}
	})
	if err != nil {
		*errors = append(*errors, err)
		if clear {
			return nil
		}
		return err
	}
	return nil
}

func checkMarks(t *testing.T, report bool) {
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.mark != 1 && report {
			t.Errorf("node %s mark = %d; expected 1", path, n.mark)
		}
		n.mark = 0
	})
}

func TestWalk(t *testing.T) {
	fs := NewMemFs()
	makeTree(t, fs)
	errors := make([]error, 0, 10)
	clear := true
	markFn := func(path string, info os.FileInfo, err error) error {
		return mark(info, err, &errors, clear)
	}
	// Expect no errors.
	err := Walk(fs, tree.name, markFn)
	if err != nil {
		t.Fatalf("no error expected, found: %s", err)
	}
	if len(errors) != 0 {
		t.Fatalf("unexpected errors: %s", errors)
	}
	checkMarks(t, true)
	errors = errors[0:0]
}
