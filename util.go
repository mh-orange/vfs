package vfs

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strings"
)

// convert os.PathError to vfs.PathError
func fixErr(err error) error {
	if pe, ok := err.(*os.PathError); ok {
		cause := pe.Err
		switch cause {
		case os.ErrExist:
			cause = ErrExist
		case os.ErrNotExist:
			cause = ErrNotExist
		case os.ErrClosed:
			cause = ErrClosed
		default:
			if _, ok := cause.(*os.PathError); ok {
				cause = fixErr(cause)
			}
		}
		err = &PathError{Op: pe.Op, Path: pe.Path, Cause: cause}
	}
	return err
}

// ErrSkipDir is used as a return value from WalkFuncs to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var ErrSkipDir = errors.New("skip this directory")

// ReadFile reads the file named by filename, from the given filesystem, and returns the content
// A successful call returns err == nil, not err == EOF. Because ReadFile reads the whole file,
// it does not treat an EOF from Read as an error to be reported.
func ReadFile(opener Opener, filename string) (data []byte, err error) {
	reader, err := opener.Open(filename)
	if err == nil {
		// ioutil.ReadAll satisfies the need to prevent returning io.EOF
		data, err = ioutil.ReadAll(reader)
		if closer, ok := reader.(io.Closer); ok {
			if err1 := closer.Close(); err == nil {
				err = err1
			}
		}
	}

	return data, fixErr(err)
}

// WriteFile writes data to a file named by filename. If the file does not exist, WriteFile
// creates it with permissions perm; otherwise WriteFile truncates it before writing.
func WriteFile(opener Opener, filename string, content []byte, perm os.FileMode) error {
	writer, err := opener.OpenFile(filename, WrOnlyFlag|CreateFlag|TruncFlag, perm)
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
	return fixErr(err)
}

// readDirNames reads the directory named by dirname and returns
// a sorted list of directory entries.
func readDirNames(fs FileSystem, dirname string) (names []string, err error) {
	f, err := fs.Open(dirname)
	if err == nil {
		names, err = f.Readdirnames(-1)
		if closer, ok := f.(io.Closer); ok {
			closer.Close()
		}
	}

	if err == nil {
		sort.Strings(names)
	}
	return names, fixErr(err)
}

// walk recursively descends path, calling walkFn.
func walk(fs FileSystem, dir string, info os.FileInfo, walkFn WalkFunc, err error) error {
	if info != nil && !info.IsDir() {
		return walkFn(dir, info, err)
	}

	names, err := readDirNames(fs, dir)
	err1 := walkFn(dir, info, err)
	// If err != nil, walk can't walk into this directory.
	// err1 != nil means walkFn want walk to skip this directory or stop walking.
	// Therefore, if one of err and err1 isn't nil, walk will return.
	if err != nil || err1 != nil {
		// The caller's behavior is controlled by the return value, which is decided
		// by walkFn. walkFn may ignore err and return nil.
		// If walkFn returns ErrSkipDir, it will be handled by the caller.
		// So walk should return whatever walkFn returns.
		return err1
	}

	for _, name := range names {
		filename := path.Join(dir, name)
		fileInfo, err := fs.Lstat(filename)
		err = walk(fs, filename, fileInfo, walkFn, err)
		if err != nil {
			if err != ErrSkipDir {
				return err
			}
		}
	}
	return err
}

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn. The files are walked in lexical
// order, which makes the output deterministic but means that for very
// large directories Walk can be inefficient.
// Walk does not follow symbolic links.
func Walk(fs FileSystem, root string, walkFn WalkFunc) error {
	info, err := fs.Lstat(root)
	err = walk(fs, root, info, walkFn, err)
	if err == ErrSkipDir {
		return nil
	}
	return fixErr(err)
}

// WalkFunc is the type of the function called for each file or directory
// visited by Walk. The path argument contains the argument to Walk as a
// prefix; that is, if Walk is called with "dir", which is a directory
// containing the file "a", the walk function will be called with argument
// "dir/a". The info argument is the os.FileInfo for the named path.
//
// If there was a problem walking to the file or directory named by path, the
// incoming error will describe the problem and the function can decide how
// to handle that error (and Walk will not descend into that directory). In the
// case of an error, the info argument will be nil. If an error is returned,
// processing stops. The sole exception is when the function returns the special
// value ErrSkipDir. If the function returns ErrSkipDir when invoked on a directory,
// Walk skips the directory's contents entirely. If the function returns ErrSkipDir
// when invoked on a non-directory file, Walk skips the remaining files in the
// containing directory.
type WalkFunc func(path string, info os.FileInfo, err error) error

// MkdirAll creates a directory named path,
// along with any necessary parents, and returns nil,
// or else returns an error.
// The permission bits perm (before umask) are used for all
// directories that MkdirAll creates.
// If path is already a directory, MkdirAll does nothing
// and returns nil.
func MkdirAll(fs FileSystem, dirname string, perm os.FileMode) error {
	// Fast path: if we can tell whether path is a directory or file, stop with success or error.
	dir, err := fs.Stat(dirname)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return &PathError{"mkdir", dirname, ErrNotDir}
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	err = MkdirAll(fs, path.Dir(dirname), perm)
	if err == nil {
		// Parent now exists; invoke Mkdir and use its result.
		err = fs.Mkdir(dirname, perm)
		if err != nil {
			// Handle arguments like "foo/." by
			// double-checking that directory doesn't exist.
			dir, err1 := fs.Lstat(dirname)
			if err1 == nil && dir.IsDir() {
				err = nil
			}
		}
	}
	return fixErr(err)
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

	dir, file := path.Split(pattern)
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
		matched, err := path.Match(pattern, n)
		if err != nil {
			return m, err
		}
		if matched {
			m = append(m, path.Join(dir, n))
		}
	}
	return
}

// cleanGlobPath prepares path for glob matching.
func cleanGlobPath(path string) string {
	switch path {
	case "":
		return "."
	case string(PathSeparator):
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

// Watch will setup a Watcher recursively watching the path and
// sending events down to the events channel.  If a new directory
// is created in the tree then the watcher will be automatically updated
func Watch(fs FileSystem, path string, events chan<- Event) (watcher Watcher, err error) {
	fi, err := fs.Stat(path)
	if err == nil {
		ev := make(chan Event, len(events))
		watcher, err = fs.Watcher(ev)

		if err == nil {
			if fi.IsDir() {
				Walk(fs, path, func(path string, info os.FileInfo, err error) error {
					if err == nil {
						if info.IsDir() {
							watcher.Watch(path)
						}
					}
					return err
				})
			}

			go func() {
				for event := range ev {
					events <- event
					// don't wait
					go func(event Event) {
						if fi, err := fs.Stat(event.Path); err == nil && fi.IsDir() {
							watcher.Watch(event.Path)
						}
					}(event)
				}
			}()
		}
	}
	return watcher, err
}
