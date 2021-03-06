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
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// osfs is a VFS backed by the operating system filesystem
type osfs struct {
	root string
}

// NewOsFs will return a new FileSystem that is backed by the operating
// system functions in the 'os' package.  The osfs filesystem will be
// rooted in the given path
func NewOsFs(root string) FileSystem {
	root, _ = filepath.Abs(root)
	return &osfs{filepath.Clean(root)}
}

// Chmod changes the mode of the named file to mode.
func (ofs *osfs) Chmod(filename string, mode os.FileMode) error {
	return os.Chmod(ofs.path(filename), mode)
}

// Create creates the named file with mode 0666 (before umask), truncating it if it already exists.  If
// successful, an io.ReadWriteSeeker is returned
func (ofs *osfs) Create(filename string) (File, error) {
	return os.Create(ofs.path(filename))
}

// Open opens the named file for reading.  If successful, an io.ReadSeeker is returned
func (ofs *osfs) Open(filename string) (File, error) {
	return os.Open(ofs.path(filename))
}

// OpenFile is the generalized open call; most users will use Open or Create instead.
// It opens the named file with specified flag (O_RDONLY etc.) and perm (before umask),
// if applicable. If successful, an io.ReadWriteSeeker is returned.  If the OpenFlag was
// set to O_RDONLY then the io.ReadWriteSeeker itself may not be writable.  This is
// dependent on the implementation
func (ofs *osfs) OpenFile(filename string, flag OpenFlag, perm os.FileMode) (File, error) {
	return os.OpenFile(ofs.path(filename), int(flag), perm)
}

func (ofs *osfs) path(filename string) string {
	if len(filename) == 0 {
		return ofs.root
	}

	if []rune(filename)[0] != filepath.Separator {
		filename = string(append([]rune{filepath.Separator}, []rune(filename)...))
	}
	return filepath.Join(ofs.root, filepath.Clean(filename))
}

// Mkdir creates a new directory with the specified name and permission bits
// (before umask). If there is an error, it will be of type *PathError.
func (ofs *osfs) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(ofs.path(name), perm)
}

// Remove removes the named file or (empty) directory. If there is an error,
// it will be of type *PathError.
func (ofs *osfs) Remove(name string) error {
	return os.Remove(ofs.path(name))
}

// Rename renames (moves) oldpath to newpath.
// If newpath already exists and is not a directory, Rename replaces it.
// OS-specific restrictions may apply when oldpath and newpath are in different directories.
// If there is an error, it will be of type *LinkError.
func (ofs *osfs) Rename(oldpath, newpath string) error {
	return os.Rename(ofs.path(oldpath), ofs.path(newpath))
}

// Lstat returns a FileInfo describing the named file. If the file is a
// symbolic link, the returned FileInfo describes the symbolic link.
// Lstat makes no attempt to follow the link. If there is an error, it
// will be of type *PathError.
func (ofs *osfs) Lstat(filename string) (os.FileInfo, error) {
	return os.Lstat(ofs.path(filename))
}

// Stat returns the FileInfo structure describing file.
func (ofs *osfs) Stat(filename string) (os.FileInfo, error) {
	return os.Stat(ofs.path(filename))
}

func (ofs *osfs) Close() error { return nil }

func (ofs *osfs) Watcher(events chan<- Event) (Watcher, error) {
	fswatcher, err := fsnotify.NewWatcher()
	watcher := &osWatcher{
		fs:      ofs,
		watcher: fswatcher,
		events:  events,
		closer:  make(chan bool, 2),
	}
	go watcher.eventLoop()
	go watcher.errorLoop()
	return watcher, err
}
