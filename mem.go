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
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const blocksize = 1024

type memFile struct {
	size    int64
	mode    os.FileMode
	modTime time.Time
	blocks  [][]byte
	mu      sync.Mutex
}

func (mf *memFile) touch() {
	mf.mu.Lock()
	mf.modTime = time.Now()
	mf.mu.Unlock()
}

func (mf *memFile) blockSize() (size int64) {
	mf.mu.Lock()
	if len(mf.blocks) > 0 {
		size = int64(blocksize*(len(mf.blocks)-1) + len(mf.blocks[len(mf.blocks)-1]))
	}
	mf.mu.Unlock()
	return
}

type readWriteSeeker struct {
	mu        sync.Mutex
	readOnly  bool
	writeOnly bool
	file      *memFile
	offset    int64
}

func (rws *readWriteSeeker) Seek(offset int64, whence int) (end int64, err error) {
	rws.mu.Lock()
	defer rws.mu.Unlock()
	if whence == io.SeekStart {
	} else if whence == io.SeekCurrent {
		offset = rws.offset + offset
	} else if whence == io.SeekEnd {
		rws.file.mu.Lock()
		offset = rws.file.size + offset
		rws.file.mu.Unlock()
	} else {
		err = ErrWhence
	}

	if offset >= 0 {
		rws.offset = offset
	} else {
		err = ErrInvalidSeek
	}

	return rws.offset, err
}

func (rws *readWriteSeeker) Read(p []byte) (n int, err error) {
	rws.mu.Lock()
	defer rws.mu.Unlock()
	if rws.writeOnly {
		return 0, ErrWriteOnly
	}

	rws.file.mu.Lock()
	size := rws.file.size
	rws.file.mu.Unlock()
	if rws.offset >= size {
		return 0, io.EOF
	}

	maxN := len(p)
	for n < maxN && err == nil {
		rws.file.mu.Lock()
		size = rws.file.size
		rws.file.mu.Unlock()
		if rws.offset < size {
			row := rws.offset / blocksize
			rowOffset := rws.offset - (row * blocksize)
			rws.file.mu.Lock()
			copied := 0
			if rws.file.size < (row+1)*blocksize {
				sizeOffset := rws.file.size - (row * blocksize)
				copied = copy(p, rws.file.blocks[row][rowOffset:sizeOffset])
			} else {
				copied = copy(p, rws.file.blocks[row][rowOffset:])
			}
			rws.file.mu.Unlock()
			n += copied
			if n < len(p) {
				p = p[copied:]
			}
			rws.offset += int64(copied)
		} else {
			err = io.EOF
		}
	}
	return
}

func (rws *readWriteSeeker) Write(p []byte) (n int, err error) {
	rws.mu.Lock()
	defer rws.mu.Unlock()
	if rws.readOnly {
		return 0, ErrReadOnly
	}

	for {
		size := rws.file.blockSize()
		if rws.offset < size {
			break
		}
		rws.file.mu.Lock()
		rws.file.blocks = append(rws.file.blocks, make([]byte, blocksize))
		rws.file.mu.Unlock()
	}

	rws.file.mu.Lock()
	if rws.offset > rws.file.size {
		rws.file.size = rws.offset
	}
	rws.file.mu.Unlock()

	for len(p) > 0 && err == nil {
		size := rws.file.blockSize()
		rws.file.mu.Lock()
		if rws.offset >= size {
			rws.file.blocks = append(rws.file.blocks, make([]byte, blocksize))
		}
		row := rws.offset / blocksize
		rowOffset := int64(0)
		if rws.offset < (row+1)*blocksize {
			rowOffset = rws.offset - (row * blocksize)
		}
		copied := copy(rws.file.blocks[row][rowOffset:], p)
		rws.file.size += int64(copied)
		rws.file.mu.Unlock()
		p = p[copied:]
		rws.offset += int64(copied)
		n += copied
	}
	return
}

type fileStat struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

// Name returns the base name of the file
func (fs *fileStat) Name() string { return fs.name }

// Size returns length of the file in bytes
func (fs *fileStat) Size() int64 { return fs.size }

// Mode is the file mode bits
func (fs *fileStat) Mode() os.FileMode { return fs.mode }

// ModTime is the modification time of the file
func (fs *fileStat) ModTime() time.Time { return fs.modTime }

// IsDir is an abbreviation for Mode().IsDir()
func (fs *fileStat) IsDir() bool { return fs.Mode().IsDir() }

// Sys returns the underlying data source.  For MemFs this is nil
func (fs *fileStat) Sys() interface{} { return nil }

// MemFs is a completely in-memory filesystem.  This filesystem is good for
// use in unit tests and that is its primary motivation
type MemFs struct {
	files map[string]*memFile
	mu    sync.Mutex
}

// NewMemFs will instantiate a new in-memory virtual filesystem
func NewMemFs() FileSystem {
	return &MemFs{files: make(map[string]*memFile)}
}

// Chmod changes the mode of the named file to mode.
func (memfs *MemFs) Chmod(filename string, mode os.FileMode) (err error) {
	memfs.mu.Lock()
	defer memfs.mu.Unlock()

	if file, found := memfs.files[filename]; found {
		file.mode = mode
	} else {
		err = ErrNotExist
	}
	return err
}

// Create creates the named file with mode 0666 (before umask), truncating it if it already exists.  If
// successful, an io.ReadWriteSeeker is returned
func (memfs *MemFs) Create(filename string) (io.ReadWriteSeeker, error) {
	return memfs.OpenFile(filename, RdWrFlag|CreateFlag|TruncFlag, 0666)
}

// Open opens the named file for reading.  If successful, an io.ReadSeeker is returned
func (memfs *MemFs) Open(filename string) (io.ReadSeeker, error) {
	return memfs.OpenFile(filename, RdOnlyFlag, 0)
}

// OpenFile is the generalized open call; most users will use Open or Create instead.
// It opens the named file with specified flag (O_RDONLY etc.) and perm (before umask),
// if applicable. If successful, an io.ReadWriteSeeker is returned.  If the OpenFlag was
// set to O_RDONLY then the io.ReadWriteSeeker itself may not be writable.  This is
// dependent on the implementation
func (memfs *MemFs) OpenFile(filename string, flag OpenFlag, perm os.FileMode) (io.ReadWriteSeeker, error) {
	memfs.mu.Lock()
	defer memfs.mu.Unlock()

	var rws *readWriteSeeker
	err := flag.check()
	if err == nil {
		file, found := memfs.files[filename]
		if found {
			if flag.has(CreateFlag) && flag.has(ExclFlag) {
				err = ErrExist
			}
		} else {
			if flag.has(CreateFlag) && (flag.has(RdWrFlag) || flag.has(WrOnlyFlag)) {
				file = &memFile{mode: perm}
				file.touch()
				memfs.files[filename] = file
			} else {
				err = ErrNotExist
			}
		}

		if err == nil {
			rws = &readWriteSeeker{file: file}
			if flag.has(RdOnlyFlag) {
				rws.readOnly = true
			} else if flag.has(WrOnlyFlag) {
				rws.writeOnly = true
			}

			if flag.has(TruncFlag) {
				file.mu.Lock()
				file.blocks = nil
				file.mu.Unlock()
			}

			if flag.has(AppendFlag) {
				_, err = rws.Seek(0, io.SeekEnd)
			}
		}
	}
	return rws, err
}

// ReadFile reads the file named by filename and returns the contents.
func (memfs *MemFs) ReadFile(filename string) (data []byte, err error) {
	return readFile(memfs, filename)
}

// Stat returns the FileInfo structure describing file.
func (memfs *MemFs) Stat(filename string) (os.FileInfo, error) {
	if file, found := memfs.files[filename]; found {
		file.mu.Lock()
		fi := &fileStat{
			name:    filepath.Base(filename),
			size:    file.size,
			mode:    file.mode,
			modTime: file.modTime,
		}
		file.mu.Unlock()
		return fi, nil
	}
	return nil, ErrNotExist
}

// WriteFile writes data to a file named by filename. If the file does not exist, WriteFile
// creates it with permissions perm; otherwise WriteFile truncates it before writing.
func (memfs *MemFs) WriteFile(filename string, content []byte, perm os.FileMode) error {
	return writeFile(memfs, filename, content, perm)
}
