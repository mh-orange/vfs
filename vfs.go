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
	ErrInvalidFlags = errors.New("Invalid combination of flags")
)

type OpenFlag int

const (
	// Exactly one of O_RDONLY, O_WRONLY, or O_RDWR must be specified.
	O_RDONLY OpenFlag = OpenFlag(os.O_RDONLY) // open the file read-only.
	O_WRONLY          = OpenFlag(os.O_WRONLY) // open the file write-only.
	O_RDWR            = OpenFlag(os.O_RDWR)   // open the file read-write.
	// The remaining values may be or'ed in to control behavior.
	O_APPEND OpenFlag = OpenFlag(os.O_APPEND) // append data to the file when writing.
	O_CREATE          = OpenFlag(os.O_CREATE) // create a new file if none exists.
	O_EXCL            = OpenFlag(os.O_EXCL)   // used with O_CREATE, file must not exist.
	O_SYNC            = OpenFlag(os.O_SYNC)   // open for synchronous I/O.
	O_TRUNC           = OpenFlag(os.O_TRUNC)  // if possible, truncate file when opened.
)

func (of OpenFlag) has(flag OpenFlag) bool {
	if flag == 0x00 {
		return of == 0x00
	}
	return of&flag == flag
}

func (of OpenFlag) check() (err error) {
	if of.has(O_WRONLY) && of.has(O_RDWR) {
		err = ErrInvalidFlags
	} else if of != 0 {
		// only write mode can use create, append and truncate
		if of.has(O_APPEND) || of.has(O_CREATE) || of.has(O_TRUNC) || of.has(O_EXCL) || of.has(O_SYNC) {
			if !of.has(O_WRONLY) && !of.has(O_RDWR) {
				err = ErrInvalidFlags
			}
		}
	}

	return
}

type FileSystem interface {
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, content []byte, mode os.FileMode) error
	Stat(filename string) (os.FileInfo, error)
	Open(filename string) (io.ReadSeeker, error)
	OpenFile(filename string, flag OpenFlag, perm os.FileMode) (io.ReadWriteSeeker, error)
	Chmod(filename string, mode os.FileMode) error
}

func readFile(fs FileSystem, filename string) (data []byte, err error) {
	reader, err := fs.Open(filename)
	if err == nil {
		data, err = ioutil.ReadAll(reader)
		if closer, ok := reader.(io.Closer); ok {
			if err1 := closer.Close(); err == nil {
				err = err1
			}
		}
	}
	return data, err
}

func writeFile(fs FileSystem, filename string, content []byte, perm os.FileMode) error {
	writer, err := fs.OpenFile(filename, O_WRONLY|O_CREATE|O_TRUNC, perm)
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
