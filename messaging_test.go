package main

import (
	"testing"
)

func TestFileId(t *testing.T) {
	filename := "/a/filename"
	fileId := NewFileId(filename)
	expected := "4768b44606fdaf3f8ed23d1aace64d4a"
	if string(fileId) != expected {
		t.Fatalf("FileId incorrect, got %s expected %s", fileId, expected)
	}
}
