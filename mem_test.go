package vfs

import (
	"io"
	"reflect"
	"testing"
)

type testBlockManager struct {
	freeBlocks    []int64
	retrieveBlock int64
	allocBlock    int64
}

func (tbm *testBlockManager) free(free ...int64) {
	tbm.freeBlocks = free
}

func (tbm *testBlockManager) block(block int64) []byte {
	tbm.retrieveBlock = block
	return make([]byte, blocksize)
}

func (tbm *testBlockManager) alloc() int64 {
	return tbm.allocBlock
}

func TestMemInodeTrunc(t *testing.T) {
	tests := []struct {
		name          string
		initialBlocks []int64
		initialSize   int64
		truncSize     int64
		wantBlocks    []int64
		wantFree      []int64
	}{
		{"one block", []int64{1}, blocksize - 10, 10, []int64{1}, []int64{}},
		{"two blocks, size 10", []int64{1, 2}, 2*blocksize - 10, 10, []int64{1}, []int64{2}},
		{"two blocks, size blocksize+1", []int64{1, 2}, 2*blocksize - 10, blocksize + 1, []int64{1, 2}, []int64{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tbm := &testBlockManager{}
			inode := &memInode{
				fs:     tbm,
				blocks: test.initialBlocks,
				size:   test.initialSize,
			}

			inode.trunc(test.truncSize)
			if !reflect.DeepEqual(tbm.freeBlocks, test.wantFree) {
				t.Errorf("Wanted to free blocks %v actually freed blocks %v", test.wantFree, tbm.freeBlocks)
			}

			if !reflect.DeepEqual(inode.blocks, test.wantBlocks) {
				t.Errorf("Wanted blocks %v got %v", test.wantBlocks, inode.blocks)
			}
		})
	}
}

func TestMemFileReaddirnames(t *testing.T) {
	file := &memFile{}
	_, gotErr := file.Readdirnames(0)
	wantErr := ErrNotDir
	if wantErr != gotErr {
		t.Errorf("Wanted error %v got %v", wantErr, gotErr)
	}
}

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
