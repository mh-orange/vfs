package vfs

import (
	"io/ioutil"
	"os"
)

type tempfs struct {
	FileSystem
	tempdir string
}

func NewTempFs() FileSystem {
	tempdir, _ := ioutil.TempDir("", "osfs_test")
	return &tempfs{
		FileSystem: NewOsFs(tempdir),
		tempdir:    tempdir,
	}
}

func (tfs *tempfs) Close() error {
	err := tfs.FileSystem.Close()
	if err == nil {
		tfs.FileSystem = nil
		err = os.RemoveAll(tfs.tempdir)
	}
	return err
}
