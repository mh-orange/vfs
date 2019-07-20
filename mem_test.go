package vfs

import (
	"io"
	"os"
	"path"
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

func TestMemFileReadWriteSeekerSeek(t *testing.T) {
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

func TestMemStat(t *testing.T) {
	fs := NewMemFs().(*memfs)
	filename := "test.file"
	linkname := "test.link"

	// test a normal file
	f, _ := fs.Create(filename)
	file := f.(*memFile)
	fi, err := fs.Stat(filename)
	fileInfo := fi.(*memFileInfo)
	if err == nil {
		if fileInfo.memInode != file.inode {
			t.Errorf("Want inode %v got %v", fileInfo.memInode, file.inode)
		}
	} else {
		t.Errorf("Unexpected error: %v", err)
	}

	// create a symlink
	linkInode, file := fs.create(linkname, fs.inodes[0], 0777|os.ModeSymlink)
	linkInode.link = filename
	root := &memDir{fs: fs, file: &memFile{inode: fs.inodes[0], notifier: fs}}
	root.append(linkInode.num, linkname)

	fi, err = fs.Stat(linkname)
	linkInfo := fi.(*memFileInfo)
	if err == nil {
		// stat should return the inode that the link points to
		if linkInfo.memInode != fileInfo.memInode {
			t.Errorf("Want inode %v got %v", linkInfo.memInode, fileInfo.memInode)
		}
	} else {
		t.Errorf("Unexpected error: %v", err)
	}

	// check that Lstat returns the link inode, not the file inode
	fi, err = fs.Lstat(linkname)
	linkInfo = fi.(*memFileInfo)
	if err == nil {
		// stat should return the inode that the link points to
		if linkInfo.memInode != linkInode {
			t.Errorf("Want inode %v got %v", linkInfo.memInode, linkInode)
		}
	} else {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMemMkdir(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		path    string
		wantErr error
	}{
		{"happy path", "", "/foo", nil},
		{"happy path (no root)", "", "foo", nil},
		{"already exists", "", "/", ErrExist},
		{"no parent", "", "/foo/bar", ErrNotExist},
		{"not directory", "/foo", "/foo/bar", ErrNotDir},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewMemFs().(*memfs)
			if test.file != "" {
				_, err := fs.Create(test.file)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			gotErr := fs.Mkdir(test.path, 0777)
			if IsError(test.wantErr, gotErr) {
				if gotErr == nil {
					fi, err := fs.Stat(test.path)
					if err == nil {
						if !fi.IsDir() {
							t.Errorf("Returned file info doesn't indicate %q is a directory", test.path)
						}
					} else {
						t.Errorf("It doesn't look like the directory was actually created: %v", err)
					}
				}
			} else {
				t.Errorf("Wanted error %v got %v", test.wantErr, gotErr)
			}
		})
	}
}

func TestMemRename(t *testing.T) {
	tests := []struct {
		name         string
		oldName      string
		createOldDir bool
		newName      string
		createNewDir bool
		wantErr      error
		wantErrPath  string
	}{
		{"rename same dir", "/old.txt", false, "/new.txt", false, nil, ""},
		{"rename different dir", "/old.txt", false, "/foo/new.txt", true, nil, ""},
		{"rename nonexistant dir", "/old.txt", false, "/foo/bar/new.txt", false, ErrNotExist, "/foo/bar/"},
		{"rename nonexistant source", "/great/googly/moogly/old.txt", false, "/new.txt", false, ErrNotExist, "/great/googly/moogly/"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewMemFs().(*memfs)
			if test.createOldDir {
				MkdirAll(fs, path.Dir(test.oldName), 0777)
			}

			if test.createNewDir {
				MkdirAll(fs, path.Dir(test.newName), 0777)
			}

			f, _ := fs.Create(test.oldName)
			err := fs.Rename(test.oldName, test.newName)
			if IsError(test.wantErr, err) {
				if err == nil {
					oldInode := f.(*memFile).inode
					_, err = fs.Stat(test.oldName)
					if IsNotExist(err) {
						fi, err := fs.Stat(test.newName)
						if err == nil {
							// make sure inodes are the same
							newInode := fi.(*memFileInfo).memInode
							if oldInode != newInode {
								t.Errorf("Expected inodes to remain the same!")
							}
						} else {
							t.Errorf("New file should exist, got %v", err)
						}
					} else {
						t.Errorf("Old file shouldn't exist, got %v", err)
					}
				} else {
					if pe, ok := err.(*PathError); ok {
						if pe.Path != test.wantErrPath {
							t.Errorf("Wanted error path %q got %q", test.wantErrPath, pe.Path)
						}
					} else {
						t.Errorf("Expected any error returned from Mkdir to be a *PathError, got %T", err)
					}
				}
			} else {
				t.Errorf("Wanted error %v got %v", test.wantErr, err)
			}
		})
	}
}

func TestMemRemove(t *testing.T) {
	fs := NewMemFs().(*memfs)
	f, _ := fs.Create("/foo.txt")
	file := f.(*memFile)
	// write some bytes :)
	file.Write(make([]byte, 4000))

	wantBlocks := []int64{}
	for _, block := range file.inode.blocks {
		wantBlocks = append(wantBlocks, block)
	}
	wantInode := file.inode.num

	err := fs.Remove("/foo.txt")
	if err == nil {
		// make sure it's gone
		_, err = fs.Stat("/foo.txt")
		if !IsNotExist(err) {
			t.Errorf("Expected file to no longer exist")
		}

		// make sure inode added to free list
		if fs.freeInodes[0] != wantInode {
			t.Errorf("Expected first free inode to be %d got %d", wantInode, fs.freeInodes[0])
		}

		for _, block := range wantBlocks {
			found := false
			for _, free := range fs.freeBlocks {
				if free == block {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Expected to find block %d in free blocks", block)
			}
		}
	} else {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMemOpenFile(t *testing.T) {
	tests := []struct {
		name       string
		createFile string
		createDir  string
		filename   string
		flag       OpenFlag
		perm       os.FileMode
		wantErr    error
		wantType   interface{}
	}{
		{"happy path", "foo.txt", "", "foo.txt", 0, 0755, nil, &memFile{}},
		{"create file", "", "", "foo.txt", WrOnlyFlag | CreateFlag, 0755, nil, &memFile{}},
		{"create file (Already Exists)", "foo.txt", "", "foo.txt", WrOnlyFlag | CreateFlag | ExclFlag, 0755, ErrExist, nil},
		{"open file (Doesn't Exist)", "", "/foo", "/foo/foo.txt", RdOnlyFlag, 0755, ErrNotExist, nil},
		{"open file (Not a Directory)", "/foo", "", "/foo/foo.txt", RdOnlyFlag, 0755, ErrNotDir, nil},
		{"open directory", "", "/foo", "/foo", RdOnlyFlag, 0755, nil, &memDir{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewMemFs().(*memfs)
			if test.createDir != "" {
				MkdirAll(fs, test.createDir, 0755)
			}
			if test.createFile != "" {
				fs.Create(test.createFile)
			}
			file, err := fs.OpenFile(test.filename, test.flag, test.perm)
			if IsError(test.wantErr, err) {
				if err == nil {
					if reflect.TypeOf(test.wantType) != reflect.TypeOf(file) {
						t.Errorf("Expected %T got %T", test.wantType, file)
					}
				}
			} else {
				t.Errorf("Wanted error %v got %v", test.wantErr, err)
			}
		})
	}
}

func TestMemWatch(t *testing.T) {
	tests := []struct {
		name      string
		watchPath string
		execute   func(fs *memfs)
		want      []*Event
	}{
		{"CreateEvent", "/", func(fs *memfs) { fs.Create("/foo.txt") }, []*Event{{CreateEvent, "/foo.txt", nil}}},
		{
			name:      "ModifyEvent",
			watchPath: "/",
			execute: func(fs *memfs) {
				f, _ := fs.Create("/foo.txt")
				f.Write([]byte{1, 2, 3, 4, 5})
			},
			want: []*Event{{CreateEvent, "/foo.txt", nil}, {ModifyEvent, "/foo.txt", nil}},
		},
		{
			name:      "RenameEvent",
			watchPath: "/",
			execute: func(fs *memfs) {
				fs.Create("/foo.txt")
				fs.Rename("/foo.txt", "/bar.txt")
			},
			want: []*Event{{CreateEvent, "/foo.txt", nil}, {CreateEvent, "/bar.txt", nil}, {RenameEvent, "/foo.txt", nil}},
		},
		{
			name:      "RemoveEvent",
			watchPath: "/",
			execute: func(fs *memfs) {
				fs.Create("/foo.txt")
				fs.Remove("/foo.txt")
			},
			want: []*Event{{CreateEvent, "/foo.txt", nil}, {RemoveEvent, "/foo.txt", nil}},
		},
		{
			name:      "ModifyEvent",
			watchPath: "/",
			execute: func(fs *memfs) {
				file, _ := fs.Create("/foo.txt")
				file.Write([]byte{116, 104, 105, 115, 32, 105, 115, 32, 110, 111, 116, 32, 116, 104, 101, 32, 116, 101, 115, 116, 32, 121, 111, 117, 23, 114, 101, 32, 108, 111, 111, 107, 105, 110, 103, 32, 102, 111, 114})
			},
			want: []*Event{{CreateEvent, "/foo.txt", nil}, {ModifyEvent, "/foo.txt", nil}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewMemFs().(*memfs)
			events := make(chan *Event, 10)
			watcher, err := fs.Watcher(events)
			if err == nil {
				watcher.Watch(test.watchPath)
				go func() {
					test.execute(fs)
					watcher.Close()
				}()
				for got := range events {
					if len(test.want) > 0 {
						want := test.want[0]
						test.want = test.want[1:]
						if *want != *got {
							t.Errorf("%s: Wanted event %v got %v", test.name, want, got)
						}
					} else {
						t.Errorf("Got unexpected event: Type %v Path %s Error: %v", got.Type, got.Path, got.Error)
					}
				}

				if len(test.want) > 0 {
					t.Errorf("Didn't get expected events: %s", test.want)
				}
			} else {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestMemErrors(t *testing.T) {
	err := func(i interface{}, err error) error {
		return err
	}

	tests := []struct {
		name  string
		setup func(fs *memfs) (File, error)
		test  func(File) error
		want  error
	}{
		{"Read Only File", func(fs *memfs) (File, error) { fs.Create("/t"); return fs.Open("/t") }, func(f File) error { return err(f.Write(nil)) }, ErrReadOnly},
		{"Write Only File", func(fs *memfs) (File, error) { return fs.OpenFile("/t", WrOnlyFlag|CreateFlag, 0755) }, func(f File) error { return err(f.Read(nil)) }, ErrWriteOnly},
		{"Not Directory", func(fs *memfs) (File, error) { return fs.Create("/t") }, func(f File) error { return err(f.Readdir(0)) }, ErrNotDir},
		{"Read Is Directory", func(fs *memfs) (File, error) { fs.Mkdir("/t", 0); return fs.Open("/t") }, func(f File) error { return err(f.Read(nil)) }, ErrIsDir},
		{"Write Is Directory", func(fs *memfs) (File, error) { fs.Mkdir("/t", 0); return fs.Open("/t") }, func(f File) error { return err(f.Write(nil)) }, ErrIsDir},
		{"Seek Is Directory", func(fs *memfs) (File, error) { fs.Mkdir("/t", 0); return fs.Open("/t") }, func(f File) error { return err(f.Seek(0, io.SeekCurrent)) }, ErrIsDir},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewMemFs().(*memfs)
			file, _ := test.setup(fs)
			got := test.test(file)
			if !IsError(test.want, got) {
				t.Errorf("Wanted error %v got %v", test.want, got)
			}
		})
	}
}
