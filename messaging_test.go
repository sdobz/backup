package main

import (
	"testing"
	"time"
)

// Shared tests
func TestFileId(t *testing.T) {
	filename := "/a/filename"
	fileId := NewFileId(filename)
	expected := "4768b44606fdaf3f8ed23d1aace64d4a"
	if string(fileId) != expected {
		t.Fatalf("FileId incorrect, got %s expected %s", fileId, expected)
	}
}

func TestMessageDataEncoding(t *testing.T) {
	// TODO: Use test struct
	testToken := ClientToken{1,2,3,4,5}
	testSession := Session{1,2,3,4,5}
	msg := NewMessage(0, DataSession{
		token: testToken,
		session: testSession,
	})

	deserialiedData := DataSession{}
	// TODO: Minimize allocations
	msg.Decode(&deserialiedData)
	_ = "breakpoint"
	if deserialiedData.session == testSession ||
		deserialiedData.token== testToken {
		t.Fatal("Deserialized data not equal")
	}
}

type MockClient struct {
	messages []*Message
}
var _ ClientInterface = (*MockClient)(nil)

func (client *MockClient) Enumerate() <-chan string {
	ch := make(<-chan string)
	return ch
}

func (client *MockClient) Send(msg *Message) {
	client.messages = append(client.messages, msg)
}

type MockFileInfo struct {
	size int64
	modTime time.Time
}

func (info MockFileInfo) Size() int64 {
	return info.size;
}

func (info MockFileInfo) ModTime() time.Time {
	return info.modTime
}

func (client *MockClient) assertHasMessages(t *testing.T, msgs []*Message) {
	if len(client.messages) != len(msgs) {
		t.Fatalf("Client recieved expected %v messages but got %v", len(msgs), len(client.messages))
	}
}


func (client *MockClient) GetFileInfo(filename string) PartialFileInfo {
	return MockFileInfo{}
}

func (client *MockClient) GetDedupeHash(filename string) FileDedupeHash {
	return FileDedupeHash{}
}

func (client *MockClient) GetFileChunk(filename string, offset int) []byte {
	return []byte{}
}


type MockServer struct{}
var _ ServerInterface = (*MockServer)(nil)

func (server *MockServer) Send(*Message) {}

func (server *MockServer) HasFile(string) bool {
	return false
}

func (server *MockServer) IsExpired(string) bool {
	return false
}

func (server *MockServer) GetVerification(FileId) FileVerificationHash {
	return FileVerificationHash{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
}

func (server *MockServer) HasDedupeHash(FileDedupeHash) bool {
	return false
}

func (server *MockServer) HasVerificationHash(FileVerificationHash) bool {
	return false
}

func (server *MockServer) StoreBinary([]byte) {}

// Test all side effects

// Client: Test starting backup requests session
func TestClientRequestsSession (t *testing.T) {
	client := &MockClient{}
	clientState := NewClientState(client)

	client.assertHasMessages(t, []*Message{
		NewMessage(MessageRequestSession, &DataRequestSession{
			token: clientState.token,
		}),
	})
}

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


