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
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const blocksize = int64(1024)

type memInode struct {
	sync.Mutex
	fs *memfs

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

func (inode *memInode) trunc() {
	inode.Lock()
	inode.fs.freen(inode.blocks)
	inode.blocks = nil
	inode.Unlock()
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

type memFile struct {
	mu        sync.Mutex
	readOnly  bool
	writeOnly bool
	inode     *memInode
	offset    int64
}

func (file *memFile) Readdirnames(n int) (names []string, err error) {
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
	copied := 0
	for n < maxN && err == nil {
		block := file.offset / blocksize
		offset := file.offset - (block * blocksize)
		copied, err = file.inode.readBlock(block, offset, p)
		n += copied
		if n < len(p) {
			p = p[copied:]
		}
		file.offset += int64(copied)
	}
	return
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
	return
}

type dirent struct {
	inode int64
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

type memDir struct {
	file *memFile
}

func (*memDir) Read(p []byte) (int, error)                           { return 0, ErrIsDir }
func (*memDir) Write(p []byte) (int, error)                          { return 0, ErrIsDir }
func (*memDir) Seek(offset int64, whence int) (end int64, err error) { return 0, ErrIsDir }

// next returns the next directory entry
func (dir *memDir) next() (*dirent, error) {
	ent := &dirent{}
	return ent, ent.read(dir.file)
}

func (dir *memDir) find(name string) (inode int64, err error) {
	err = ErrNotExist
	var ent *dirent
	for ent, err = dir.next(); err == nil; ent, err = dir.next() {
		if ent.name == name {
			inode = ent.inode
			err = nil
			break
		}
	}
	return
}

func (dir *memDir) append(inode int64, filename string) error {
	oldOffset := dir.file.offset
	_, err := dir.file.Seek(0, io.SeekEnd)
	if err == nil {
		ent := &dirent{inode, filename}
		err = ent.write(dir.file)
	}

	if err == nil {
		_, err = dir.file.Seek(oldOffset, io.SeekStart)
	}
	return err
}

func (dir *memDir) Readdirnames(n int) (names []string, err error) {
	for err == nil && n <= 0 {
		var ent *dirent
		ent, err = dir.next()
		if err == nil {
			names = append(names, ent.name)
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
	freeBlocks []int64
	blocks     [][]byte
}

// NewMemfs will instantiate a new in-memory virtual filesystem
func NewMemFs() FileSystem {
	fs := &memfs{}

	root := &memInode{
		fs:      fs,
		mode:    os.ModeDir,
		modTime: time.Now(),
	}

	fs.inodes = []*memInode{root}
	return fs
}

func (fs *memfs) block(n int64) []byte { fs.Lock(); defer fs.Unlock(); return fs.blocks[n] }

func (fs *memfs) freen(blocks []int64) {
	fs.Lock()
	for _, block := range blocks {
		fs.freeBlocks = append(fs.freeBlocks, block)
	}
	fs.Unlock()
}

func (fs *memfs) free(block int64) {
	fs.Lock()
	fs.freeBlocks = append(fs.freeBlocks, block)
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
	if strings.HasPrefix(filename, "/") {
		filename = strings.TrimPrefix(filename, "/")
	}

	// inode[0] is always root directory
	n := int64(0)
	if len(filename) == 0 {
		inode = fs.inodes[n]
	} else {
		path := []string{""}
		names := strings.Split(filename, string(os.PathSeparator))
		inode = fs.inodes[n]
		for i, name := range names {
			path = append(path, name)
			if inode.Mode().IsDir() {
				dir := &memDir{&memFile{inode: inode}}
				n, err = dir.find(name)
				if err != nil {
					break
				}
				inode = fs.inodes[n]
			} else if i < len(names)-1 {
				err = &PathError{"find", strings.Join(names, "/"), ErrNotDir}
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

	var file File
	var inode *memInode
	err := flag.check()
	if err == nil {
		inode, err = fs.find(filename)
		if err == nil {
			if flag.has(CreateFlag) && flag.has(ExclFlag) {
				err = ErrExist
			}
		} else {
			inode, err = fs.find(filepath.Dir(filename))
			if err == nil {
				if inode.Mode().IsDir() {
					dir := &memDir{&memFile{inode: inode}}
					if flag.has(CreateFlag) && (flag.has(RdWrFlag) || flag.has(WrOnlyFlag)) {
						// create a new inode
						inode = &memInode{
							fs:   fs,
							mode: perm,
						}
						fs.Lock()
						fs.inodes = append(fs.inodes, inode)
						inodeNum := int64(len(fs.inodes) - 1)
						fs.Unlock()
						inode.touch()
						dir.append(inodeNum, filepath.Base(filename))
					} else {
						err = ErrNotExist
					}
				} else {
					err = ErrNotDir
				}
			}
		}

		if err == nil {
			if inode.Mode().IsDir() {
				if flag.has(WrOnlyFlag) || flag.has(RdWrFlag) || flag.has(AppendFlag) || flag.has(CreateFlag) || flag.has(TruncFlag) {
					err = ErrIsDir
				} else {
					file = &memDir{&memFile{inode: inode}}
				}
			} else {
				mf := &memFile{inode: inode}
				if flag.has(RdOnlyFlag) {
					mf.readOnly = true
				} else if flag.has(WrOnlyFlag) {
					mf.writeOnly = true
				}

				if flag.has(TruncFlag) {
					inode.trunc()
				}

				if flag.has(AppendFlag) {
					_, err = mf.Seek(0, io.SeekEnd)
				}

				file = mf
			}
		}
	}
	return file, err
}

func (fs *memfs) Remove(name string) error {
	return nil
}

func (fs *memfs) Mkdir(name string, perm os.FileMode) error {
	if !strings.HasPrefix(name, "/") {
		name = fmt.Sprintf("/%s", name)
	}
	inode, err := fs.find(filepath.Dir(name))
	if err == nil {
		if inode.Mode().IsDir() {
			newInode := &memInode{
				fs:   fs,
				mode: os.ModeDir | perm,
			}
			fs.Lock()
			fs.inodes = append(fs.inodes, newInode)
			inodeNum := int64(len(fs.inodes) - 1)
			fs.Unlock()
			newInode.touch()
			dir := &memDir{&memFile{inode: inode}}
			dir.append(inodeNum, filepath.Base(name))
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
			name:     filepath.Base(filename),
		}
	}
	return fi, err
}

// Stat returns the FileInfo structure describing file.
func (fs *memfs) Stat(filename string) (os.FileInfo, error) {
	inode, err := fs.find(filename)
	if inode.Mode()&os.ModeSymlink == os.ModeSymlink {
		return fs.Stat(inode.link)
	}

	if err == nil {
		fi := &memFileInfo{
			memInode: inode,
			name:     filepath.Base(filename),
		}
		return fi, nil
	}
	return nil, err
}
