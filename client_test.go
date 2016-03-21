package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
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
	// Creates a file with the given filename containing the filename repeated to the correct filesize
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
	if diff < -2 || diff > 2 {
		t.Fatal("Client modtime is too different")
	}

	tfm.Cleanup()
}

func TestSmallFileHasDeterministicDedupeHash(t *testing.T) {
	filename := "file"
	filesize := int64(128)
	tfm := NewTempFileManager()
	tfm.CreateFile(filename, filesize)

	client := NewClient(BackupSpec{}, &ChannelNetwork{})
	dedupe := client.GetDedupeHash(tfm.Prefix(filename))

	// Gotten by running this function with an incorrect hash
	expected := FileDedupeHash{128, 1, 109, 158, 23, 121, 104, 84, 243, 195, 166, 138, 179, 49, 134, 0}

	if dedupe != expected {
		t.Fatalf("Incorrect dedupe hash, got: %v", dedupe)
	}

	filename2 := "file2"
	tfm.CreateFile(filename2, filesize)
	dedupe = client.GetDedupeHash(tfm.Prefix(filename2))

	if dedupe == expected {
		t.Fatal("Expected file hash to change with different contents")
	}

	tfm.Cleanup()
}

func TestLargeFileHasDeterministicDedupeHash(t *testing.T) {
	filename := "file"
	// imohash changes strategies on files larger than 128k
	filesize := int64(1000000)
	tfm := NewTempFileManager()
	tfm.CreateFile(filename, filesize)

	client := NewClient(BackupSpec{}, &ChannelNetwork{})
	dedupe := client.GetDedupeHash(tfm.Prefix(filename))

	// Gotten by running this function with an incorrect hash
	expected := FileDedupeHash{192, 132, 61, 22, 46, 84, 78, 89, 30, 218, 156, 68, 51, 194, 43, 15}

	if dedupe != expected {
		t.Fatalf("Incorrect dedupe hash, got: %v", dedupe)
	}

	filename2 := "file2"
	tfm.CreateFile(filename2, filesize)
	dedupe = client.GetDedupeHash(tfm.Prefix(filename2))

	if dedupe == expected {
		t.Fatal("Expected file hash to change with different contents")
	}

	tfm.Cleanup()
}

func TestFileChunkInFile(t *testing.T) {
	filename := "file"
	filesize := int64(100)
	tfm := NewTempFileManager()
	tfm.CreateFile(filename, filesize)

	client := NewClient(BackupSpec{}, &ChannelNetwork{})
	chunk := client.GetFileChunk(tfm.Prefix(filename), 10, 1)
	expected := []byte{'i', 'l', 'e', 'f', 'i', 'l', 'e', 'f', 'i', 'l'}

	if !bytes.Equal(chunk, expected) {
		t.Fatalf("Got: %v expected: %v", chunk, expected)
	}

	// As a side effect this also tests that TempFileManager creates files properly

	tfm.Cleanup()
}

func TestFileChunkReachesFileEnd(t *testing.T) {
	filename := "file"
	filesize := int64(100)
	tfm := NewTempFileManager()
	tfm.CreateFile(filename, filesize)

	client := NewClient(BackupSpec{}, &ChannelNetwork{})
	chunk := client.GetFileChunk(tfm.Prefix(filename), 10, 95)
	expected := []byte{'e', 'f', 'i', 'l', 'e'}

	if !bytes.Equal(chunk, expected) {
		t.Fatalf("Got: %v expected: %v", chunk, expected)
	}

	// As a side effect this also tests that TempFileManager creates files properly

	tfm.Cleanup()
}

func TestParseBackupSpec(t *testing.T) {
	specString := `
	# Commnts are ignored
	filename
	+ filename_plus
	+    filename_plus_whitespace
	filename_trailing_space
	# - Marks not inclusion
	- not_included
	- /absolute/filename
	/absolute/*/subdirs
	/absolute/path/
	`
	globs, _ := parseBackupSpec(strings.NewReader(specString))

	expected := BackupSpec{[]BackupGlob{
		BackupGlob{
			include:  true,
			root:     "",
			origGlob: "filename",
		},
		BackupGlob{
			include:  true,
			root:     "",
			origGlob: "filename_plus",
		},
		BackupGlob{
			include:  true,
			root:     "",
			origGlob: "filename_plus_whitespace",
		},
		BackupGlob{
			include:  true,
			root:     "",
			origGlob: "filename_trailing_space",
		},
		BackupGlob{
			include:  false,
			root:     "",
			origGlob: "not_included",
		},
		BackupGlob{
			include:  false,
			root:     "/absolute/filename",
			origGlob: "/absolute/filename",
		},
		BackupGlob{
			include:  true,
			root:     "/absolute/",
			origGlob: "/absolute/*/subdirs",
		},
		BackupGlob{
			include:  true,
			root:     "/absolute/path/",
			origGlob: "/absolute/path/**",
		},
	}}

	if len(globs.globs) != len(expected.globs) {
		t.Fatal("Produced wrong number of globs")
	}

	for i := 0; i < len(globs.globs); i++ {
		actual := globs.globs[i]
		// We aren't testing this
		actual.glob = nil
		expectedGlob := expected.globs[i]
		if actual != expectedGlob {
			t.Fail()
			t.Logf("Expected %+v got %+v", expectedGlob, actual)
		}
	}
}
