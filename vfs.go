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
)

// OpenFlag is passed to Open functions to indicate any actions taken
// while opening a file
type OpenFlag int

const (
	// Exactly one of O_RDONLY, O_WRONLY, or O_RDWR must be specified.

	// RdOnlyFlag opens the file in read-only mode
	RdOnlyFlag OpenFlag = OpenFlag(os.O_RDONLY)

	// WrOnlyFlag opens the file in write-only mode
	WrOnlyFlag = OpenFlag(os.O_WRONLY)

	// RdWrFlag opens the file for reading and writing
	RdWrFlag = OpenFlag(os.O_RDWR)

	// The remaining values may be or'ed in to control behavior.

	// AppendFlag will seek the open file to the end
	AppendFlag OpenFlag = OpenFlag(os.O_APPEND)

	// CreateFlag will create the file if it does not exist
	CreateFlag = OpenFlag(os.O_CREATE)

	// ExclFlag will prevent an existing file from being overwritten when CreateFlag is given
	ExclFlag = OpenFlag(os.O_EXCL)

	// TruncFlag will truncate a file when it is opened for writing
	TruncFlag = OpenFlag(os.O_TRUNC)
)

const (
	// PathSeparator is the slash character
	PathSeparator = "/" // OS-specific path separator
)

// has checks to see if the OpenFlag has a specific flag set. If flag is zero
// then it checks to make sure the receiver itself is zero
func (of OpenFlag) has(flag OpenFlag) bool {
	if flag == 0x00 {
		return of == 0x00
	}
	return of&flag == flag
}

// check determines if the set of flags given are valid.  For instance, if both
// O_WRONLY and O_RDWR are set that is an invalid state (can't open a file both
// write only and read/write)
func (of OpenFlag) check() (err error) {
	if of.has(WrOnlyFlag) && of.has(RdWrFlag) {
		err = ErrInvalidFlags
	} else if of != 0 {
		// only write mode can use create, append and truncate
		if of.has(AppendFlag) || of.has(CreateFlag) || of.has(TruncFlag) || of.has(ExclFlag) {
			if !of.has(WrOnlyFlag) && !of.has(RdWrFlag) {
				err = ErrInvalidFlags
			}
		}
	}

	return
}

// File represents an object that has most of the behaviour of an os.File
type File interface {
	io.ReadWriteSeeker

	// Name returns the name of the file as presented to Open.
	Name() string

	// Readdirnames reads and returns a slice of names from the directory f.
	//
	// If n > 0, Readdirnames returns at most n names. In this case, if
	// Readdirnames returns an empty slice, it will return a non-nil error
	// explaining why. At the end of a directory, the error is io.EOF.
	//
	// If n <= 0, Readdirnames returns all the names from the directory in
	// a single slice. In this case, if Readdirnames succeeds (reads all
	// the way to the end of the directory), it returns the slice and a
	// nil error. If it encounters an error before the end of the
	// directory, Readdirnames returns the names read until that point and
	// a non-nil error.
	Readdirnames(n int) (names []string, err error)

	// Readdir reads the contents of the directory associated with file and
	// returns a slice of up to n FileInfo values, as would be returned
	// by Lstat, in directory order. Subsequent calls on the same file will yield
	// further FileInfos.
	//
	// If n > 0, Readdir returns at most n FileInfo structures. In this case, if
	// Readdir returns an empty slice, it will return a non-nil error
	// explaining why. At the end of a directory, the error is io.EOF.
	//
	// If n <= 0, Readdir returns all the FileInfo from the directory in
	// a single slice. In this case, if Readdir succeeds (reads all
	// the way to the end of the directory), it returns the slice and a
	// nil error. If it encounters an error before the end of the
	// directory, Readdir returns the FileInfo read until that point
	// and a non-nil error.
	Readdir(n int) ([]os.FileInfo, error)
}

// Opener is a FileSystem that has the ability to open files
type Opener interface {
	// Open opens the named file for reading.  If successful, an io.ReadSeeker is returned
	Open(filename string) (File, error)

	// OpenFile is the generalized open call; most users will use Open or Create instead.
	// It opens the named file with specified flag (O_RDONLY etc.) and perm (before umask),
	// if applicable. If successful, an io.ReadWriteSeeker is returned.  If the OpenFlag was
	// set to O_RDONLY then the io.ReadWriteSeeker itself may not be writable.  This is
	// dependent on the implementation
	OpenFile(filename string, flag OpenFlag, perm os.FileMode) (File, error)
}

// FileSystem is the primary interface that must be satisfied.  A FileSystem abstracts away
// the basic operations of interacting with files including reading and writing files
type FileSystem interface {
	Opener

	// Chmod changes the mode of the named file to mode.
	Chmod(filename string, mode os.FileMode) error

	// Create creates the named file with mode 0666 (before umask), truncating it if it already exists.  If
	// successful, an io.ReadWriteSeeker is returned
	Create(name string) (File, error)

	// Mkdir creates a new directory with the specified name and permission bits
	// (before umask). If there is an error, it will be of type *PathError.
	Mkdir(name string, perm os.FileMode) error

	// Remove removes the named file or (empty) directory. If there is an error,
	// it will be of type *PathError.
	Remove(name string) error

	// Rename renames (moves) oldpath to newpath.
	// If newpath already exists and is not a directory, Rename replaces it.
	// OS-specific restrictions may apply when oldpath and newpath are in different directories.
	// If there is an error, it will be of type *LinkError.
	Rename(oldpath, newpath string) error

	// Lstat returns a FileInfo describing the named file. If the file is a
	// symbolic link, the returned FileInfo describes the symbolic link.
	// Lstat makes no attempt to follow the link. If there is an error, it
	// will be of type *PathError.
	Lstat(name string) (os.FileInfo, error)

	// Stat returns the FileInfo structure describing file.
	Stat(filename string) (os.FileInfo, error)

	// Close will perform any implementation specific cleanup work and close the
	// filesystem.  It is assumed that the filesystem is unusable after being closed
	Close() error

	// Watcher will create a file watcher instance that can be used to watch
	// for events on paths of the file system.  The provided Event channel
	// must be initialized by the caller sized appropriately for buffering
	// events at the rate expected.  If the channel buffer becomes full a
	// Watcher will drop the event.  The provided channel will be closed by
	// the watcher instance when the watcher itself is closed
	Watcher(chan<- *Event) (Watcher, error)
}
