package vfs

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

type tempfs struct {
	*OsFs
	tempdir string
}

func NewTempFs() FileSystem {
	tempdir, _ := ioutil.TempDir("", "osfs_test")
	return &tempfs{
		OsFs:    &OsFs{filepath.Clean(tempdir)},
		tempdir: tempdir,
	}
}

func (tfs *tempfs) Close() error {
	tfs.OsFs = nil
	return os.RemoveAll(tfs.tempdir)
}
