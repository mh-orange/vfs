package vfs

import (
	"errors"
	"fmt"
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
	ErrNotExist = errors.New("no such file or directory")

	// ErrNotDir indicates a file is not a directory when a directory operation was
	// called (such as Readdirnames)
	ErrNotDir = errors.New("The path specified is not a directory")

	// ErrIsDir indicates a file is a directory, not a regular file.  This is returned
	// by directories when file I/O operations (read, write, seek) are called
	ErrIsDir = errors.New("The path specified is a directory")

	// ErrBadPattern indicates a pattern was malformed.
	ErrBadPattern = errors.New("syntax error in pattern")

	// ErrSize is returned when an invalid size is used.  For example, a negative size
	// given to the Truncate function
	ErrSize = errors.New("invalid size")

	// ErrClosed indicates a file was already closed and cannot be closed again
	ErrClosed = errors.New("file already closed")
)

// IsExist returns a boolean indicating whether the error is known to report
// that a file or directory already exists. It is satisfied by ErrExist as
// well as some syscall errors.
func IsExist(err error) bool {
	// accomodate OsFs
	return IsError(ErrExist, err) || os.IsExist(err)
}

// IsNotExist returns a boolean indicating whether the error is known to
// report that a file or directory does not exist. It is satisfied by
// ErrNotExist as well as some syscall errors.
func IsNotExist(err error) bool {
	// accomodate OsFs
	return IsError(ErrNotExist, err) || os.IsNotExist(err)
}

// IsError will check to see if got is the same type of
// error as want.  If got is a *PathError then IsError will
// compare the underlying *PathError.Cause
func IsError(want, got error) bool {
	if pe, ok := got.(*PathError); ok {
		got = pe.cause()
	}
	return want == got
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
