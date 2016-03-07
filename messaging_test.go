package main

import (
	"testing"
	"os"
)

func TestFileId(t *testing.T) {
	filename := "/a/filename"
	fileId := NewFileId(filename)
	expected := "4768b44606fdaf3f8ed23d1aace64d4a"
	if string(fileId) != expected {
		t.Fatalf("FileId incorrect, got %s expected %s", fileId, expected)
	}
}

/*
type ClientInterface interface {
	Send(Message)
	GetFileInfo(string) os.FileInfo
	GetDedupeHash(string) FileDedupeHash
	GetFileChunk(string, int) []byte
}
 */

type MockClient struct {}

func (client *MockClient) Send(msg Message) {}
func (client *MockClient) GetFileInfo(filename string) os.FileInfo {}
func (client *MockClient) GetDedupeHash(filename string) FileDedupeHash {}
func (client *MockClient) GetFileChunk(filename string, offset int) []byte {}


/*
type ServerSessionInterface interface {
	Send(Message)
	HasFile(string) bool
	IsExpired(string) bool
	GetVerification(FileId) DataFileVerification
	HasDedupeHash(FileDedupeHash) bool
	HasVerificationHash(FileVerificationHash) bool
	StoreBinary([]byte)
}
*/

type MockServer struct{}
func (server *MockServer) Send(Message) {}
func (server *MockServer) HasFile(string) bool {}
func (server *MockServer) IsExpired(string) bool {}
func (server *MockServer) GetVerification(FileId) DataFileVerification {}
func (server *MockServer) HasDedupeHash(FileDedupeHash) bool {}
func (server *MockServer) HasVerificationHash(FileVerificationHash) bool {}
func (server *MockServer) StoreBinary([]byte) {}

// Test all side effects

// Client: Test starting backup requests session
// Server: Test getting session request sends session
// Client: Test getting session starts enumeration

// Client: Test enumeration sends file message
// Server: Test unmodified file
// Client: Test handles fileok
// Server: Test unknown file sends dedupe
// Client: Test responds with dedupe
// Server: Test dedupe exists
// Server: Test dedupe not exists
// Client: Test


