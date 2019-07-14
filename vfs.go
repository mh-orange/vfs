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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

	// ErrNotDir indicates a file is not a directory when a directory operation was
	// called (such as Readdirnames)
	ErrNotDir = errors.New("The path specified is not a directory")

	// ErrIsDir indicates a file is a directory, not a regular file.  This is returned
	// by directories when file I/O operations (read, write, seek) are called
	ErrIsDir = errors.New("The path specified is a directory")

	// ErrBadPattern indicates a pattern was malformed.
	ErrBadPattern = errors.New("syntax error in pattern")
)

// IsExist returns a boolean indicating whether the error is known to report
// that a file or directory already exists. It is satisfied by ErrExist as
// well as some syscall errors.
func IsExist(err error) bool {
	// accomodate OsFs
	return IsError(err, ErrExist) || os.IsExist(err)
}

// IsNotExist returns a boolean indicating whether the error is known to
// report that a file or directory does not exist. It is satisfied by
// ErrNotExist as well as some syscall errors.
func IsNotExist(err error) bool {
	// accomodate OsFs
	return IsError(err, ErrNotExist) || os.IsNotExist(err)
}

// PathError represents an error that occured while performing an operation
// on a given path
type PathError struct {
	// Op is the name of the operation where the error occurred
	Op string

	// Path is the string path that caused the error
	Path string

	// Cause is the underlying error that occurred (ErrNotDir, ErrIsDir, etc)
	Cause error
}

// Error returns information about the operation and path where an error occurred
func (pe *PathError) Error() string {
	return fmt.Sprintf("%s %s: %v", pe.Op, pe.Path, pe.Cause)
}

func (pe *PathError) cause() error {
	err := pe.Cause
	if pe, ok := err.(*PathError); ok {
		err = pe.cause()
	}
	return err
}

// IsError will determine if `check` is wrapping an underlying error.
// If so, the underlying error is compared to `err`.
func IsError(err, is error) bool {
	if pe, ok := err.(*PathError); ok {
		err = pe.cause()
	}
	return err == is
}

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

type File interface {
	io.ReadWriteSeeker

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
}

// FileSystem is the primary interface that must be satisfied.  A FileSystem abstracts away
// the basic operations of interacting with files including reading and writing files
type FileSystem interface {
	// Chmod changes the mode of the named file to mode.
	Chmod(filename string, mode os.FileMode) error

	// Create creates the named file with mode 0666 (before umask), truncating it if it already exists.  If
	// successful, an io.ReadWriteSeeker is returned
	Create(name string) (File, error)

	// Open opens the named file for reading.  If successful, an io.ReadSeeker is returned
	Open(filename string) (File, error)

	// OpenFile is the generalized open call; most users will use Open or Create instead.
	// It opens the named file with specified flag (O_RDONLY etc.) and perm (before umask),
	// if applicable. If successful, an io.ReadWriteSeeker is returned.  If the OpenFlag was
	// set to O_RDONLY then the io.ReadWriteSeeker itself may not be writable.  This is
	// dependent on the implementation
	OpenFile(filename string, flag OpenFlag, perm os.FileMode) (File, error)

	// Mkdir creates a new directory with the specified name and permission bits
	// (before umask). If there is an error, it will be of type *PathError.
	Mkdir(name string, perm os.FileMode) error

	// Remove removes the named file or (empty) directory. If there is an error,
	// it will be of type *PathError.
	Remove(name string) error

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
}

// ReadFile reads the file named by filename, from the given filesystem, and returns the content
// A successful call returns err == nil, not err == EOF. Because ReadFile reads the whole file,
// it does not treat an EOF from Read as an error to be reported.
func ReadFile(fs FileSystem, filename string) (data []byte, err error) {
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

// WriteFile writes data to a file named by filename. If the file does not exist, WriteFile
// creates it with permissions perm; otherwise WriteFile truncates it before writing.
func WriteFile(fs FileSystem, filename string, content []byte, perm os.FileMode) error {
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

// readDirNames reads the directory named by dirname and returns
// a sorted list of directory entries.
func readDirNames(fs FileSystem, dirname string) ([]string, error) {
	f, err := fs.Open(dirname)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)

	if closer, ok := f.(io.Closer); ok {
		closer.Close()
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// walk recursively descends path, calling walkFn.
func walk(fs FileSystem, path string, info os.FileInfo, walkFn filepath.WalkFunc) error {
	if !info.IsDir() {
		return walkFn(path, info, nil)
	}

	names, err := readDirNames(fs, path)
	err1 := walkFn(path, info, err)
	// If err != nil, walk can't walk into this directory.
	// err1 != nil means walkFn want walk to skip this directory or stop walking.
	// Therefore, if one of err and err1 isn't nil, walk will return.
	if err != nil || err1 != nil {
		// The caller's behavior is controlled by the return value, which is decided
		// by walkFn. walkFn may ignore err and return nil.
		// If walkFn returns SkipDir, it will be handled by the caller.
		// So walk should return whatever walkFn returns.
		return err1
	}

	for _, name := range names {
		filename := filepath.Join(path, name)
		fileInfo, err := fs.Lstat(filename)
		if err != nil {
			if err := walkFn(filename, fileInfo, err); err != nil && err != filepath.SkipDir {
				return err
			}
		} else {
			err = walk(fs, filename, fileInfo, walkFn)
			if err != nil {
				if !fileInfo.IsDir() || err != filepath.SkipDir {
					return err
				}
			}
		}
	}
	return nil
}

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn. The files are walked in lexical
// order, which makes the output deterministic but means that for very
// large directories Walk can be inefficient.
// Walk does not follow symbolic links.
func Walk(fs FileSystem, root string, walkFn filepath.WalkFunc) error {
	info, err := fs.Lstat(root)
	if err != nil {
		err = walkFn(root, nil, err)
	} else {
		err = walk(fs, root, info, walkFn)
	}
	if err == filepath.SkipDir {
		return nil
	}
	return err
}

// Glob returns the names of all files matching pattern or nil
// if there is no matching file.
// The pattern syntax is:
//
//	pattern:
//		{ term }
//	term:
//		'*'         matches any sequence of non-Separator characters
//		'?'         matches any single non-Separator character
//		'[' [ '^' ] { character-range } ']'
//		            character class (must be non-empty)
//		c           matches character c (c != '*', '?', '\\', '[')
//		'\\' c      matches character c
//
//	character-range:
//		c           matches character c (c != '\\', '-', ']')
//		'\\' c      matches character c
//		lo '-' hi   matches character c for lo <= c <= hi
//
// The pattern may describe hierarchical names such as
// /usr/*/bin/ed (assuming the Separator is '/').
//
// Glob ignores file system errors such as I/O errors reading directories.
// The only possible returned error is ErrBadPattern, when pattern
// is malformed.
func Glob(fs FileSystem, pattern string) (matches []string, err error) {
	if !hasMeta(pattern) {
		if _, err = fs.Lstat(pattern); err != nil {
			return nil, nil
		}
		return []string{pattern}, nil
	}

	dir, file := filepath.Split(pattern)
	volumeLen := 0
	dir = cleanGlobPath(dir)

	if !hasMeta(dir[volumeLen:]) {
		return glob(fs, dir, file, nil)
	}

	// Prevent infinite recursion. See issue 15879.
	if dir == pattern {
		return nil, ErrBadPattern
	}

	var m []string
	m, err = Glob(fs, dir)
	if err != nil {
		return
	}
	for _, d := range m {
		matches, err = glob(fs, d, file, matches)
		if err != nil {
			return
		}
	}
	return
}

// glob searches for files matching pattern in the directory dir
// and appends them to matches. If the directory cannot be
// opened, it returns the existing matches. New matches are
// added in lexicographical order.
func glob(fs FileSystem, dir, pattern string, matches []string) (m []string, e error) {
	m = matches
	fi, err := fs.Stat(dir)
	if err != nil {
		return
	}
	if !fi.IsDir() {
		return
	}
	d, err := fs.Open(dir)
	if err != nil {
		return
	}
	if closer, ok := d.(io.Closer); ok {
		defer closer.Close()
	}

	names, _ := d.Readdirnames(-1)
	sort.Strings(names)

	for _, n := range names {
		matched, err := filepath.Match(pattern, n)
		if err != nil {
			return m, err
		}
		if matched {
			m = append(m, filepath.Join(dir, n))
		}
	}
	return
}

// cleanGlobPath prepares path for glob matching.
func cleanGlobPath(path string) string {
	switch path {
	case "":
		return "."
	case string(filepath.Separator):
		// do nothing to the path
		return path
	default:
		return path[0 : len(path)-1] // chop off trailing separator
	}
}

// hasMeta reports whether path contains any of the magic characters
// recognized by Match.
func hasMeta(path string) bool {
	magicChars := `*?[`
	return strings.ContainsAny(path, magicChars)
}
