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
	"errors"
	"io"
	"io/ioutil"
	"os"
)

var (
	// ErrInvalidFlags indicates that the OpenFlags are set to an invalid combination.  For instance,
	// the O_WRONLY and O_RDWR flags were both set
	ErrInvalidFlags = errors.New("Invalid combination of flags")

	// ErrInvalidSeek indicates a seek that moves the offset before the beginning of the file
	ErrInvalidSeek = errors.New("seek before beginning of file")

	// ErrReadOnly indicates an operation that requires write flags was attempted on a file that
	// is open read-only
	ErrReadOnly = errors.New("file is open read only")

	// ErrWriteOnly is returned when an operation requiring read-only flags was attempted on a
	// file that was opened for writing
	ErrWriteOnly = errors.New("file is open write only")

	// ErrWhence is a seek error returned when an invalid whence value was passed to Seek.  The
	// valid whence values are io.SeekStart, io.SeekCurrent and io.SeekEnd
	ErrWhence = errors.New("invalid value for whence")

	// ErrExist is returned when a file exists but an exclusive create was attempted
	ErrExist = errors.New("file already exists")

	// ErrNotExist indicates a file was not found
	ErrNotExist = errors.New("file does not exist")
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

// FileSystem is the primary interface that must be satisfied.  A FileSystem abstracts away
// the basic operations of interacting with files including reading and writing files
type FileSystem interface {
	// Chmod changes the mode of the named file to mode.
	Chmod(filename string, mode os.FileMode) error

	// Create creates the named file with mode 0666 (before umask), truncating it if it already exists.  If
	// successful, an io.ReadWriteSeeker is returned
	Create(name string) (io.ReadWriteSeeker, error)

	// Open opens the named file for reading.  If successful, an io.ReadSeeker is returned
	Open(filename string) (io.ReadSeeker, error)

	// OpenFile is the generalized open call; most users will use Open or Create instead.
	// It opens the named file with specified flag (O_RDONLY etc.) and perm (before umask),
	// if applicable. If successful, an io.ReadWriteSeeker is returned.  If the OpenFlag was
	// set to O_RDONLY then the io.ReadWriteSeeker itself may not be writable.  This is
	// dependent on the implementation
	OpenFile(filename string, flag OpenFlag, perm os.FileMode) (io.ReadWriteSeeker, error)

	// ReadFile reads the file named by filename and returns the contents.
	ReadFile(filename string) ([]byte, error)

	// Stat returns the FileInfo structure describing file.
	Stat(filename string) (os.FileInfo, error)

	// WriteFile writes data to a file named by filename. If the file does not exist, WriteFile
	// creates it with permissions perm; otherwise WriteFile truncates it before writing.
	WriteFile(filename string, content []byte, mode os.FileMode) error
}

// readFile reads the file named by filename, from the given filesystem, and returns the content
// A successful call returns err == nil, not err == EOF. Because ReadFile reads the whole file,
// it does not treat an EOF from Read as an error to be reported.
func readFile(fs FileSystem, filename string) (data []byte, err error) {
	reader, err := fs.Open(filename)
	if err == nil {
		// ioutil.ReadAll satisfies the need to prevent returning io.EOF
		data, err = ioutil.ReadAll(reader)
		if closer, ok := reader.(io.Closer); ok {
			if err1 := closer.Close(); err == nil {
				err = err1
			}
		}
	}

	return data, err
}

// writeFile writes data to a file named by filename. If the file does not exist, WriteFile
// creates it with permissions perm; otherwise WriteFile truncates it before writing.
func writeFile(fs FileSystem, filename string, content []byte, perm os.FileMode) error {
	writer, err := fs.OpenFile(filename, WrOnlyFlag|CreateFlag|TruncFlag, perm)
	if err == nil {
		var n int
		n, err = writer.Write(content)
		if err == nil && n < len(content) {
			err = io.ErrShortWrite
		}

		if closer, ok := writer.(io.Closer); ok {
			if err1 := closer.Close(); err == nil {
				err = err1
			}
		}
	}
	return err
}
