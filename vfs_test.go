package vfs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestConstructors(t *testing.T) {
	tests := []struct {
		obj interface{}
	}{
		{NewOsFs("/")},
		{NewMemFs()},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%T", test.obj), func(t *testing.T) {
			if _, ok := test.obj.(FileSystem); !ok {
				t.Errorf("does not implement FileSystem")
			}
		})
	}
}

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
	closed bool
	got    []byte
	want   []byte
}

func (tf *testFs) Chmod(filename string, perm os.FileMode) error { return nil }
func (tf *testFs) Create(filename string) (io.ReadWriteSeeker, error) {
	return tf.OpenFile(filename, 0, 0)
}
func (tf *testFs) Open(filename string) (io.ReadSeeker, error) { return tf.OpenFile(filename, 0, 0) }

func (tf *testFs) OpenFile(filename string, flags OpenFlag, perm os.FileMode) (io.ReadWriteSeeker, error) {
	return tf, nil
}

func (tf *testFs) ReadFile(filename string) ([]byte, error)  { return tf.want, nil }
func (tf *testFs) Stat(filename string) (os.FileInfo, error) { return nil, nil }
func (tf *testFs) WriteFile(filename string, content []byte, perm os.FileMode) error {
	tf.got = make([]byte, len(content))
	copy(tf.got, content)
	return nil
}

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
		err := writeFile(fs, "test file", test.want, 0)
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
		got, err := readFile(fs, "test file")
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
