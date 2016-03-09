package main

import (
	"bytes"
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
	testToken := ClientToken{1, 2, 3, 4, 5}
	testSession := Session{1, 2, 3, 4, 5}
	msgData := DataSession{
		Token:   testToken,
		Session: testSession,
	}
	msg := NewMessage(0, msgData)

	deserializedData := DataSession{}
	// TODO: Minimize allocations
	msg.Decode(&deserializedData)
	if deserializedData.Session != testSession ||
		deserializedData.Token != testToken {
		t.Fatal("Deserialized data not equal")
	}
}

type MockClient struct {
	messages []*Message
	files []string
	enumeratedFiles bool
}

var _ ClientInterface = (*MockClient)(nil)

func (client *MockClient) Enumerate() <-chan string {
	client.enumeratedFiles = true
	ch := make(chan string)
	go func() {
		for _, filename := range client.files {
			ch <- filename
		}
		close(ch)
	}()
	return ch
}

func (client *MockClient) Send(msg *Message) {
	client.messages = append(client.messages, msg)
}

type MockFileInfo struct {
	size    int64
	modTime time.Time
}

func (info MockFileInfo) Size() int64 {
	return info.size
}

func (info MockFileInfo) ModTime() time.Time {
	return info.modTime
}

func (client *MockClient) assertSentMessages(t *testing.T, msgs []*Message) {
	if len(client.messages) != len(msgs) {
		t.Fail()
		t.Logf("Client expected %v messages but recieved %v", len(msgs), len(client.messages))
	}

	for i, msg := range client.messages {
		if i >= len(msgs) {
			t.Fail()
			t.Logf("Client got an unexpected %v message", msg.Type)
			continue
		}
		if msg.Type != msgs[i].Type {
			t.Fail()
			t.Logf("Client expected %v but got %v", msgs[i].Type, msg.Type)
			continue
		}
		if !bytes.Equal(msg.d, msgs[i].d) {
			t.Fail()
			t.Logf("Client message %v has invalid data", msg.Type)
			continue
		}
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

type MockServer struct {
	messages []*Message
	session  Session
}

var _ ServerInterface = (*MockServer)(nil)

func (server *MockServer) Send(msg *Message) {
	server.messages = append(server.messages, msg)
}

func (server *MockServer) assertSentMessages(t *testing.T, msgs []*Message) {
	if len(server.messages) != len(msgs) {
		t.Fail()
		t.Logf("Server expected %v messages but recieved %v", len(msgs), len(server.messages))
	}

	for i, msg := range server.messages {
		if i >= len(msgs) {
			t.Fail()
			t.Logf("Server got an unexpected %v message", msg.Type)
			continue
		}
		if msg.Type != msgs[i].Type {
			t.Fail()
			t.Logf("Server expected %v but got %v", msgs[i].Type, msg.Type)
			continue
		}
		if !bytes.Equal(msg.d, msgs[i].d) {
			t.Fail()
			t.Logf("Server message %v has invalid data", msg.Type)
			continue
		}
	}
}

func (server *MockServer) HasFile(string) bool {
	return false
}

func (server *MockServer) IsExpired(string) bool {
	return false
}

func (server *MockServer) GetVerification(FileId) FileVerificationHash {
	return FileVerificationHash{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
}

func (server *MockServer) HasDedupeHash(FileDedupeHash) bool {
	return false
}

func (server *MockServer) HasVerificationHash(FileVerificationHash) bool {
	return false
}

func (server *MockServer) StoreBinary([]byte) {}

func (server *MockServer) NewSession() Session {
	server.session = NewSession()
	return server.session
}

// Test all side effects

func TestClientRequestsSession(t *testing.T) {
	client := &MockClient{}
	clientState := NewClientState(client)

	if clientState.state != ClientStateGettingSession {
		t.Fatalf("Client in state %v, expected %v", clientState.state, ClientStateGettingSession)
	}

	client.assertSentMessages(t, []*Message{
		NewMessage(MessageRequestSession, &DataRequestSession{
			Token: clientState.token,
		}),
	})

}

func TestServerRespondsWithSession(t *testing.T) {
	server := &MockServer{}
	serverState := NewServerState(server)
	clientToken := ClientToken{1, 2, 3, 4, 5}

	serverState.handleMessage(NewMessage(MessageRequestSession, &DataRequestSession{
		Token: clientToken,
	}))

	server.assertSentMessages(t, []*Message{
		NewMessage(MessageSession, &DataSession{
			Token:   clientToken,
			Session: server.session,
		}),
	})
}

func TestClientDoesNotAcceptSessionWithIncorrectToken(t *testing.T) {
	client := &MockClient{}
	clientState := NewClientState(client)
	clientState.state = ClientStateGettingSession
	session := Session{1,2,3,4,5}
	validToken := ClientToken{1,2,3,4,5}
	invalidToken := ClientToken{5,4,3,2,1}
	clientState.token = validToken

	clientState.handleMessage(NewMessage(MessageSession, &DataSession{
		Session: session,
		Token: invalidToken,
	}))

	if clientState.session == session {
		t.Fatal("Client accept session with invalid token")
	}

	if clientState.state != ClientStateGettingSession {
		t.Fatalf("Client in state %v, expected %v", clientState.state, ClientStateGettingSession)
	}
}

func TestClientChecksFilesAfterGettingSession(t *testing.T) {
	client := &MockClient{}
	clientState := NewClientState(client)
	clientState.state = ClientStateGettingSession
	session := Session{1,2,3,4,5}
	token := ClientToken{1,2,3,4,5}
	clientState.token = token

	clientState.handleMessage(NewMessage(MessageSession, &DataSession{
		Session: session,
		Token: token,
	}))

	if clientState.session != session {
		t.Fatal("Client state did not store session")
	}

	if clientState.state != ClientStateCheckingFiles {
		t.Fatalf("Client in state %v, expected %v", clientState.state, ClientStateCheckingFiles)
	}

	if client.enumeratedFiles != true {
		t.Fatal("Client did not start enumerating files")
	}
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
