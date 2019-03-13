package vfs

import (
	"io"
	"testing"
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
		rws := &memFile{inode: &memInode{size: test.size}, offset: test.current}
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
