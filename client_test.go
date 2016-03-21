package main

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"
)

type TempFileManager struct {
	dir string
}

func NewTempFileManager() *TempFileManager {
	tfm := TempFileManager{}
	var err error
	tfm.dir, err = ioutil.TempDir("", "testing_temp")
	if err != nil {
		log.Fatal("Failed to create temp dir")
	}

	return &tfm
}

func (tfm *TempFileManager) CreateFile(filename string, size int64) {
	os.MkdirAll(path.Dir(filename), 0755)

	chunk := make([]byte, size)
	nameLen := int64(len(filename))

	var i int64
	for i = 0; i < size; i++ {
		chunk[i] = filename[i%nameLen]
	}

	if err := ioutil.WriteFile(tfm.Prefix(filename), chunk, 0666); err != nil {
		log.Fatal(err)
	}
}

func (tfm *TempFileManager) Cleanup() {
	os.RemoveAll(tfm.dir)
}

func (tfm *TempFileManager) Prefix(filename string) string {
	return filepath.Join(tfm.dir, filename)
}

func TestClientGetsInfo(t *testing.T) {
	filename := "file"
	filesize := int64(128)
	tfm := NewTempFileManager()
	tfm.CreateFile(filename, filesize)

	client := NewClient(BackupSpec{}, &ChannelNetwork{})
	info := client.GetFileInfo(tfm.Prefix(filename))

	if info.Size() != filesize {
		t.Fatal("Client reported incorrect filesize")
	}

	diff := info.ModTime().Sub(time.Now()).Seconds()
	if diff < -.5 || diff > .5 {
		t.Fatal("Client modtime is too different")
	}
}
