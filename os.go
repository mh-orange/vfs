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
)

type osFs struct {
	root string
}

func NewOsFs(root string) FileSystem {
	return &osFs{filepath.Clean(root)}
}

func (osfs *osFs) Chmod(filename string, mode os.FileMode) error {
	return os.Chmod(osfs.path(filename), mode)
}

func (osfs *osFs) Open(filename string) (io.ReadSeeker, error) {
	return os.Open(osfs.path(filename))
}

func (osfs *osFs) OpenFile(filename string, flag OpenFlag, perm os.FileMode) (io.ReadWriteSeeker, error) {
	return os.OpenFile(osfs.path(filename), int(flag), perm)
}

func (osfs *osFs) path(filename string) string {
	if len(filename) == 0 {
		return osfs.root
	}

	if []rune(filename)[0] != filepath.Separator {
		filename = string(append([]rune{filepath.Separator}, []rune(filename)...))
	}
	return filepath.Join(osfs.root, filepath.Clean(filename))
}

func (osfs *osFs) ReadFile(filename string) (content []byte, err error) {
	return readFile(osfs, filename)
}

func (osfs *osFs) Stat(filename string) (os.FileInfo, error) {
	return os.Stat(osfs.path(filename))
}

func (osfs *osFs) WriteFile(filename string, content []byte, perm os.FileMode) error {
	return writeFile(osfs, filename, content, perm)
}
