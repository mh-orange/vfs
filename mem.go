// Copyright 2019 Andrew Bates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vfs

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

const blocksize = int64(1024)

type blockManager interface {
	free(...int64)
	block(int64) []byte
	alloc() int64
}

type memInodeNum int64

type memInode struct {
	sync.Mutex
	fs     blockManager
	num    memInodeNum
	parent memInodeNum

	// attributes
	size    int64
	mode    os.FileMode
	modTime time.Time
	link    string // what a symlink points to
	blocks  []int64
}

func (inode *memInode) touch()                   { inode.Lock(); inode.modTime = time.Now(); inode.Unlock() }
func (inode *memInode) Size() int64              { inode.Lock(); defer inode.Unlock(); return inode.size }
func (inode *memInode) setMode(mode os.FileMode) { inode.Lock(); inode.mode = mode; inode.Unlock() }
func (inode *memInode) Mode() os.FileMode        { inode.Lock(); defer inode.Unlock(); return inode.mode }
func (inode *memInode) IsDir() bool              { return inode.Mode().IsDir() }

func (inode *memInode) ModTime() time.Time {
	inode.Lock()
	defer inode.Unlock()
	return inode.modTime
}

func (inode *memInode) trunc(size int64) {
	// determine number of blocks required for the new size
	n := int(size / blocksize)
	if size%blocksize > 0 {
		n++
	}
	inode.fs.free(inode.blocks[n:]...)
	inode.size = size
	inode.blocks = inode.blocks[0:n]
}

func (inode *memInode) readBlock(block, offset int64, p []byte) (n int, err error) {
	inode.Lock()
	defer inode.Unlock()
	if (block*blocksize)+offset < inode.size {
		if inode.size < (block+1)*blocksize {
			sizeOffset := inode.size - (block * blocksize)
			n = copy(p, inode.fs.block(inode.blocks[block])[offset:sizeOffset])
		} else {
			n = copy(p, inode.fs.block(inode.blocks[block])[offset:])
		}
	} else {
		err = io.EOF
	}
	return
}

func (inode *memInode) writeBlock(block, offset int64, p []byte) (n int, err error) {
	inode.Lock()
	defer inode.Unlock()

	for {
		bsize := blocksize * int64(len(inode.blocks))
		if inode.size < bsize {
			break
		}
		inode.blocks = append(inode.blocks, inode.fs.alloc())
	}

	n = copy(inode.fs.block(inode.blocks[block])[offset:], p)
	inode.size += int64(n)
	return
}

type memNotifier interface {
	notify(EventType, memInodeNum, string)
}

type memFile struct {
	mu        sync.Mutex
	notifier  memNotifier
	readOnly  bool
	writeOnly bool
	inode     *memInode
	offset    int64
	closed    bool
	name      string
}

func (file *memFile) Name() string {
	return file.name
}

func (file *memFile) Readdirnames(n int) ([]string, error) {
	return nil, ErrNotDir
}

func (file *memFile) Readdir(n int) ([]os.FileInfo, error) {
	return nil, ErrNotDir
}

func (file *memFile) Seek(offset int64, whence int) (end int64, err error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if whence == io.SeekStart {
	} else if whence == io.SeekCurrent {
		offset = file.offset + offset
	} else if whence == io.SeekEnd {
		offset = file.inode.Size() + offset
	} else {
		err = ErrWhence
	}

	if offset >= 0 {
		file.offset = offset
	} else {
		err = ErrInvalidSeek
	}

	return file.offset, err
}

func (file *memFile) Read(p []byte) (n int, err error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.writeOnly {
		return 0, ErrWriteOnly
	}

	maxN := len(p)
	n = maxN
	for n > 0 && err == nil {
		copied := 0
		block := file.offset / blocksize
		offset := file.offset - (block * blocksize)
		copied, err = file.inode.readBlock(block, offset, p)
		n -= copied
		if n > 0 {
			p = p[copied:]
		}
		file.offset += int64(copied)
	}
	return maxN - n, err
}

func (file *memFile) Write(p []byte) (n int, err error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.readOnly {
		return 0, ErrReadOnly
	}

	for len(p) > 0 && err == nil {
		copied := 0
		block := file.offset / blocksize
		offset := int64(0)
		if file.offset < (block+1)*blocksize {
			offset = file.offset - (block * blocksize)
		}
		copied, err = file.inode.writeBlock(block, offset, p)
		p = p[copied:]
		file.offset += int64(copied)
		n += copied
	}
	if !file.inode.IsDir() {
		file.notifier.notify(ModifyEvent, file.inode.parent, file.name)
	}
	return
}

func (file *memFile) trunc(size int64) (err error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.readOnly {
		return ErrReadOnly
	}
	if size < 0 || size > file.inode.Size() {
		err = ErrSize
	}
	file.inode.trunc(size)
	return
}

func (file *memFile) Close() (err error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.closed {
		err = ErrClosed
	} else {
		file.closed = true
	}
	return
}

func (file *memFile) flags(flag OpenFlag) (err error) {
	if file.inode.Mode().IsDir() {
		if flag.has(WrOnlyFlag) || flag.has(RdWrFlag) || flag.has(AppendFlag) || flag.has(CreateFlag) || flag.has(TruncFlag) {
			err = ErrIsDir
		}
	} else {
		if flag.has(RdOnlyFlag) {
			file.readOnly = true
		} else if flag.has(WrOnlyFlag) {
			file.writeOnly = true
		}

		if flag.has(TruncFlag) {
			file.inode.trunc(0)
		}

		if flag.has(AppendFlag) {
			_, err = file.Seek(0, io.SeekEnd)
		}
	}
	return err

}

type dirent struct {
	inode memInodeNum
	name  string
}

func (ent *dirent) read(reader io.Reader) error {
	err := binary.Read(reader, binary.BigEndian, &ent.inode)
	if err == nil {
		length := int64(0)
		err = binary.Read(reader, binary.BigEndian, &length)
		if err == nil {
			buf := make([]byte, length)
			_, err := io.ReadFull(reader, buf)
			if err == nil {
				ent.name = string(buf)
			}
		}
	}
	return err
}

func (ent *dirent) write(writer io.Writer) error {
	err := binary.Write(writer, binary.BigEndian, ent.inode)
	if err == nil {
		name := []byte(ent.name)
		length := int64(len(name))
		err = binary.Write(writer, binary.BigEndian, length)
		if err == nil {
			_, err = writer.Write(name)
		}
	}
	return err
}

// size returns the number of bytes that this entry takes up in
// the file
func (ent *dirent) size() int64 {
	// 8 bytes for inode number
	// 8 bytes for name length
	// n bytes for name
	return int64(16 + len(ent.name))
}

type inodeManager interface {
	inode(memInodeNum) *memInode
}

type memDir struct {
	fs   inodeManager
	file *memFile
}

func (dir *memDir) Name() string                                     { return dir.file.Name() }
func (*memDir) Read(p []byte) (int, error)                           { return 0, ErrIsDir }
func (*memDir) Write(p []byte) (int, error)                          { return 0, ErrIsDir }
func (*memDir) Seek(offset int64, whence int) (end int64, err error) { return 0, ErrIsDir }

// next returns the next directory entry
func (dir *memDir) next() (*dirent, error) {
	ent := &dirent{}
	return ent, ent.read(dir.file)
}

func (dir *memDir) findEntry(name string) (ent *dirent, err error) {
	err = ErrNotExist
	for ent, err = dir.next(); err == nil; ent, err = dir.next() {
		if ent.name == name {
			err = nil
			break
		}
	}
	return
}

func (dir *memDir) find(name string) (inode memInodeNum, err error) {
	ent, err := dir.findEntry(name)
	if err == nil {
		inode = ent.inode
	}
	return
}

func (dir *memDir) rename(oldname, newname string) error {
	ent, err := dir.unlink(oldname)
	if err == nil {
		err = dir.append(ent.inode, newname)
	}
	dir.file.notifier.notify(RenameEvent, dir.file.inode.num, oldname)
	return err
}

func (dir *memDir) remove(filename string) (*dirent, error) {
	ent, err := dir.unlink(filename)
	if err == nil {
		dir.file.notifier.notify(RemoveEvent, dir.file.inode.num, filename)
	}
	return ent, err
}

func (dir *memDir) unlink(filename string) (*dirent, error) {
	ent, err := dir.findEntry(filename)
	if err == nil {
		reader := &memFile{notifier: dir.file.notifier, inode: dir.file.inode, offset: dir.file.offset}
		writer := dir.file
		_, err = writer.Seek(-ent.size(), io.SeekCurrent)
		if err == nil {
			_, err = io.Copy(writer, reader)
			if err == nil {
				dir.file.trunc(dir.file.inode.Size() - ent.size())
			}
		}
	}
	return ent, err
}

func (dir *memDir) append(inode memInodeNum, filename string) error {
	oldOffset := dir.file.offset
	_, err := dir.file.Seek(0, io.SeekEnd)
	if err == nil {
		ent := &dirent{inode, filename}
		err = ent.write(dir.file)
	}

	if err == nil {
		_, err = dir.file.Seek(oldOffset, io.SeekStart)
	}
	dir.file.notifier.notify(CreateEvent, dir.file.inode.num, filename)
	return err
}

func (dir *memDir) Readdirnames(n int) (names []string, err error) {
	entries, err := dir.Readdir(n)
	if err == nil {
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
	}
	return
}

func (dir *memDir) Readdir(n int) (entries []os.FileInfo, err error) {
	for err == nil && n <= 0 {
		var ent *dirent
		ent, err = dir.next()
		if err == nil {
			entries = append(entries, &memFileInfo{name: ent.name, memInode: dir.fs.inode(ent.inode)})
			if n != -1 {
				n--
			}
		}
	}

	if n == -1 && err == io.EOF {
		err = nil
	}
	return
}

type memFileInfo struct {
	*memInode
	name string
}

// Name returns the base name of the file
func (fi *memFileInfo) Name() string { return fi.name }

// Sys returns the underlying data source.  For memfs this is nil
func (fi *memFileInfo) Sys() interface{} { return nil }

// memfs is a completely in-memory filesystem.  This filesystem is good for
// use in unit tests and that is its primary motivation
type memfs struct {
	sync.Mutex

	inodes     []*memInode
	freeInodes []memInodeNum

	freeBlocks []int64
	blocks     [][]byte
	watchers   map[memInodeNum]map[*memWatcher]string
}

// NewMemFs will instantiate a new in-memory virtual filesystem
func NewMemFs() FileSystem {
	fs := &memfs{
		watchers: make(map[memInodeNum]map[*memWatcher]string),
	}

	root := &memInode{
		fs:      fs,
		num:     0,
		mode:    os.ModeDir,
		modTime: time.Now(),
	}

	fs.inodes = []*memInode{root}
	return fs
}

func (fs *memfs) notify(t EventType, inode memInodeNum, name string) {
	fs.Lock()
	defer fs.Unlock()
	if watchers, found := fs.watchers[inode]; found {
		for watcher, dir := range watchers {
			select {
			case watcher.events <- &Event{Type: t, Path: path.Join(dir, name)}:
			default:
			}
		}
	}
}

func (fs *memfs) Watcher(events chan<- *Event) (Watcher, error) {
	mw := &memWatcher{
		fs:     fs,
		events: events,
	}
	return mw, nil
}

func (fs *memfs) removeWatch(watcher *memWatcher, path string) error {
	inode, err := fs.find(path)
	if err == nil {
		fs.Lock()
		if watchers, found := fs.watchers[inode.num]; found {
			delete(watchers, watcher)
		}
		fs.Unlock()
	}
	return err
}

func (fs *memfs) watch(watcher *memWatcher, path string) error {
	inode, err := fs.find(path)
	if err == nil {
		fs.Lock()
		if _, found := fs.watchers[inode.num]; !found {
			fs.watchers[inode.num] = make(map[*memWatcher]string)
		}
		fs.watchers[inode.num][watcher] = path
		fs.Unlock()
	}
	return err
}

func (fs *memfs) inode(n memInodeNum) *memInode { return fs.inodes[n] }

func (fs *memfs) block(n int64) []byte { fs.Lock(); defer fs.Unlock(); return fs.blocks[n] }

func (fs *memfs) free(blocks ...int64) {
	fs.Lock()
	for _, block := range blocks {
		fs.freeBlocks = append(fs.freeBlocks, block)
	}
	fs.Unlock()
}

func (fs *memfs) freeInode(inode memInodeNum) {
	fs.Lock()
	for _, block := range fs.inodes[inode].blocks {
		fs.freeBlocks = append(fs.freeBlocks, block)
	}

	fs.inodes[inode].parent = 0
	fs.inodes[inode].size = 0
	fs.inodes[inode].mode = 0
	fs.inodes[inode].modTime = time.Time{}
	fs.inodes[inode].link = ""
	fs.inodes[inode].blocks = nil

	fs.freeInodes = append(fs.freeInodes, inode)
	fs.Unlock()
}

func (fs *memfs) alloc() (block int64) {
	fs.Lock()
	if len(fs.freeBlocks) > 0 {
		block = fs.freeBlocks[0]
		fs.freeBlocks = fs.freeBlocks[1:]
	} else {
		fs.blocks = append(fs.blocks, make([]byte, blocksize))
		block = int64(len(fs.blocks) - 1)
	}
	fs.Unlock()
	return
}

func (fs *memfs) find(filename string) (inode *memInode, err error) {
	if strings.HasPrefix(filename, PathSeparator) {
		filename = strings.TrimPrefix(filename, PathSeparator)
	}

	if strings.HasSuffix(filename, PathSeparator) {
		filename = strings.TrimSuffix(filename, PathSeparator)
	}

	// inode[0] is always root directory
	n := memInodeNum(0)
	if len(filename) == 0 {
		inode = fs.inodes[n]
	} else {
		// TODO: change this to use path.Split or something safer than
		// strings.Split
		names := strings.Split(filename, string(PathSeparator))
		inode = fs.inodes[n]
		for i, name := range names {
			if inode.Mode().IsDir() {
				dir := &memDir{fs: fs, file: &memFile{notifier: fs, inode: inode}}
				n, err = dir.find(name)
				if err != nil {
					break
				}
				inode = fs.inodes[n]
			} else if i <= len(names)-1 {
				err = ErrNotDir
			}
		}

		if err == io.EOF {
			err = ErrNotExist
		}
	}
	return inode, err
}

// Chmod changes the mode of the named file to mode.
func (fs *memfs) Chmod(filename string, mode os.FileMode) error {
	inode, err := fs.find(filename)
	if err == nil {
		inode.setMode(mode)
	}
	return err
}

func (fs *memfs) create(name string, parent *memInode, perm os.FileMode) (inode *memInode, file *memFile) {
	dir := &memDir{fs: fs, file: &memFile{notifier: fs, inode: parent}}
	// create a new inode
	fs.Lock()
	if len(fs.freeInodes) > 0 {
		inodeNum := fs.freeInodes[0]
		inode = fs.inodes[inodeNum]
		fs.freeInodes = fs.freeInodes[1:]
		inode.mode = perm
	} else {
		inode = &memInode{
			fs:   fs,
			mode: perm,
		}
		fs.inodes = append(fs.inodes, inode)
		inode.num = memInodeNum(len(fs.inodes) - 1)
	}
	fs.Unlock()
	inode.parent = parent.num
	dir.append(inode.num, name)
	inode.touch()
	file = &memFile{notifier: fs, inode: inode}
	return inode, file
}

// Create creates the named file with mode 0666 (before umask), truncating it if it already exists.  If
// successful, an io.ReadWriteSeeker is returned
func (fs *memfs) Create(filename string) (File, error) {
	return fs.OpenFile(filename, RdWrFlag|CreateFlag|TruncFlag, 0666)
}

// Open opens the named file for reading.  If successful, an io.ReadSeeker is returned
func (fs *memfs) Open(filename string) (File, error) {
	return fs.OpenFile(filename, RdOnlyFlag, 0)
}

// OpenFile is the generalized open call; most users will use Open or Create instead.
// It opens the named file with specified flag (O_RDONLY etc.) and perm (before umask),
// if applicable. If successful, an io.ReadWriteSeeker is returned.  If the OpenFlag was
// set to O_RDONLY then the io.ReadWriteSeeker itself may not be writable.  This is
// dependent on the implementation
func (fs *memfs) OpenFile(filename string, flag OpenFlag, perm os.FileMode) (File, error) {
	if !strings.HasPrefix(filename, "/") {
		filename = fmt.Sprintf("/%s", filename)
	}

	var file *memFile
	var inode *memInode
	err := flag.check()
	if err == nil {
		inode, err = fs.find(filename)
		if err == nil {
			file = &memFile{notifier: fs, inode: inode}
			file.flags(flag)
			if flag.has(CreateFlag) && flag.has(ExclFlag) {
				file = nil
				err = ErrExist
			}
		} else {
			var parent *memInode
			parent, err = fs.find(path.Dir(filename))
			if err == nil {
				if parent.Mode().IsDir() {
					if flag.has(CreateFlag) && (flag.has(RdWrFlag) || flag.has(WrOnlyFlag)) {
						inode, file = fs.create(path.Base(filename), parent, perm)
						file.flags(flag)
					} else {
						err = ErrNotExist
					}
				} else {
					err = ErrNotDir
				}
			}
		}
	}

	if err == nil {
		file.name = filename
		if inode.IsDir() {
			return &memDir{fs: fs, file: file}, nil
		}
		return file, nil
	}
	return nil, err
}

func (fs *memfs) Remove(name string) error {
	dirname, filename := path.Split(name)
	parentInode, err := fs.find(dirname)
	if err == nil {
		var ent *dirent
		parent := &memDir{fs: fs, file: &memFile{notifier: fs, inode: parentInode}}
		ent, err = parent.remove(filename)
		fs.freeInode(ent.inode)
	}
	return err
}

func (fs *memfs) Rename(oldpath, newpath string) error {
	olddir, oldfile := path.Split(oldpath)
	newdir, newfile := path.Split(newpath)
	inode, err := fs.find(olddir)
	if err == nil {
		oldParent := &memDir{fs: fs, file: &memFile{notifier: fs, inode: inode}}
		if olddir == newdir {
			oldParent.rename(oldfile, newfile)
		} else {
			inode, err = fs.find(newdir)
			if err == nil {
				newParent := &memDir{fs: fs, file: &memFile{notifier: fs, inode: inode}}
				var ent *dirent
				ent, err = oldParent.remove(oldfile)
				if err == nil {
					newParent.append(ent.inode, newfile)
				}
			} else {
				err = &PathError{Op: "rename", Path: newdir, Cause: err}
			}
		}
	} else {
		err = &PathError{Op: "rename", Path: olddir, Cause: err}
	}
	return err
}

func (fs *memfs) Mkdir(name string, perm os.FileMode) error {
	if !strings.HasPrefix(name, "/") {
		name = fmt.Sprintf("/%s", name)
	}

	// check for existing file
	_, err := fs.find(name)
	if err == nil {
		return &PathError{"mkdir", name, ErrExist}
	}

	inode, err := fs.find(path.Dir(name))
	if err == nil {
		if inode.Mode().IsDir() {
			fs.create(path.Base(name), inode, os.ModeDir|perm)
		} else {
			err = &PathError{"mkdir", name, ErrNotDir}
		}
	} else {
		err = &PathError{"mkdir", name, err}
	}
	return err
}

func (fs *memfs) Lstat(filename string) (fi os.FileInfo, err error) {
	inode, err := fs.find(filename)
	if err == nil {
		fi = &memFileInfo{
			memInode: inode,
			name:     path.Base(filename),
		}
	}
	return fi, err
}

// Stat returns the FileInfo structure describing file.
func (fs *memfs) Stat(filename string) (fi os.FileInfo, err error) {
	inode, err := fs.find(filename)
	if err == nil && inode.Mode()&os.ModeSymlink == os.ModeSymlink {
		fi, err = fs.Stat(inode.link)
	} else if err == nil {
		fi = &memFileInfo{
			memInode: inode,
			name:     path.Base(filename),
		}
	}

	return fi, err
}

func (fs *memfs) Close() error {
	fs.Lock()
	defer fs.Unlock()
	fs.inodes = nil
	fs.freeBlocks = nil
	fs.blocks = nil
	return nil
}
