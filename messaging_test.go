package main

import (
	"bytes"
	"sync"
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
	msgData := DataSession{
		Token: testToken,
	}
	msg := NewMessage(0, msgData)

	deserializedData := DataSession{}
	// TODO: Minimize allocations
	msg.Decode(&deserializedData)
	if deserializedData.Token != testToken {
		t.Fatal("Deserialized data not equal")
	}
}

type MockClient struct {
	// Populated by client.Send
	messages []*Message
	// Tracks whether files were enumerated
	enumeratedFiles bool
	// Tracks when file enumeration finishes
	wg sync.WaitGroup
	// The following populated by InitializeFile
	files         []string
	dedupeStore   map[string]FileDedupeHash
	fileInfoStore map[string]MockFileInfo
}

var _ ClientInterface = (*MockClient)(nil)

func NewMockClient() *MockClient {
	return &MockClient{
		dedupeStore:   make(map[string]FileDedupeHash),
		fileInfoStore: make(map[string]MockFileInfo),
	}
}

func (client *MockClient) Enumerate() <-chan string {
	client.enumeratedFiles = true
	ch := make(chan string)
	client.wg.Add(len(client.files))
	go func() {
		for _, filename := range client.files {
			ch <- filename
			client.wg.Done()
		}
		close(ch)
	}()
	return ch
}

func (client *MockClient) GetName() string {
	return ""
}

func (client *MockClient) GetClientInfo() (ClientInfo, error) {
	return ClientInfo{}, nil
}

// Test helper
func (client *MockClient) WaitForEnumerationFinish() {
	client.wg.Wait()
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
		t.Logf("Client expected to send %v messages but sent %v", len(msgs), len(client.messages))
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
	fileInfo, ok := client.fileInfoStore[filename]
	if ok {
		return fileInfo
	}
	return MockFileInfo{}
}

func (client *MockClient) GetDedupeHash(filename string) FileDedupeHash {
	dedupe, ok := client.dedupeStore[filename]
	if ok {
		return dedupe
	}
	return FileDedupeHash{}
}

func (client *MockClient) GetFileChunk(filename string, size int64, offset int64) []byte {
	chunk := make([]byte, size)
	nameLen := int64(len(filename))

	for i := int64(0); i < size; i++ {
		chunk[i] = filename[(i+offset)%nameLen]
	}

	return chunk
}

// Testing helper
func (client *MockClient) InitializeFile(filename string, fileInfo MockFileInfo, dedupe FileDedupeHash) {
	client.files = append(client.files, filename)
	client.dedupeStore[filename] = dedupe
	client.fileInfoStore[filename] = fileInfo
}

func (cs *ClientState) InitializeFile(filename string, state ClientStateEnum) {
	cfs := NewClientFileState(cs, filename)
	cfs.state = state
	cs.fileState[cfs.id] = cfs
}

type MockServer struct {
	messages      []*Message
	files         []string
	clientInfo    ClientInfo
	dedupeStore   map[FileDedupeHash]struct{}
	fileInfoStore map[string]MockFileInfo
	storedFiles   map[string][]byte
}

var _ ServerInterface = (*MockServer)(nil)

func NewMockServer() *MockServer {
	return &MockServer{
		dedupeStore:   make(map[FileDedupeHash]struct{}),
		fileInfoStore: make(map[string]MockFileInfo),
		storedFiles:   make(map[string][]byte),
	}
}

func (server *MockServer) Send(msg *Message) {
	server.messages = append(server.messages, msg)
}

func (server *MockServer) assertSentMessages(t *testing.T, msgs []*Message) {
	if len(server.messages) != len(msgs) {
		t.Fail()
		t.Logf("Server expected to send %v messages but sent %v", len(msgs), len(server.messages))
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

func (server *MockServer) GetVerification(filename string) FileVerificationHash {
	return FileVerificationHash{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
}

func (server *MockServer) LinkExisting(filename string) (bool, error) {
	_, ok := server.fileInfoStore[filename]
	return ok, nil
}

func (server *MockServer) LinkDedupe(filename string, hash FileDedupeHash) (bool, error) {
	_, ok := server.dedupeStore[hash]
	return ok, nil
}

func (server *MockServer) WriteChunk(filename string, chunk []byte, last bool) error {
	server.storedFiles[filename] = append(server.storedFiles[filename], chunk...)
	return nil
}

func (server *MockServer) SetClientInfo(clientInfo ClientInfo) {
	server.clientInfo = clientInfo
}

// Test helper
func (server *MockServer) InitializeFile(filename string, fileInfo MockFileInfo, dedupe FileDedupeHash) {
	server.files = append(server.files, filename)
	server.fileInfoStore[filename] = fileInfo
	server.dedupeStore[dedupe] = struct{}{}
	server.storedFiles[filename] = []byte{}
}

func (ss *ServerState) InitializeFile(filename string, state ServerStateEnum) {
	sfs := NewServerFileState(ss.server, filename)
	sfs.state = state
	ss.fileState[sfs.id] = sfs
}

// Test all side effects

func TestClientStateInitializesToken(t *testing.T) {
	client := NewMockClient()
	cs := NewClientState(client)
	emptyToken := ClientToken{}

	if cs.token == emptyToken {
		t.Fatal("Client state has zero-value token")
	}
}

func TestClientRequestsSession(t *testing.T) {
	client := NewMockClient()
	clientState := NewClientState(client)
	clientState.requestSession()

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
	server := NewMockServer()
	serverState := NewServerState(server)
	clientToken := ClientToken{1, 2, 3, 4, 5}
	clientInfo := ClientInfo{
		Identity:   "identity",
		BackupName: "name",
		Session:    "session",
	}

	serverState.handleMessage(NewMessage(MessageRequestSession, &DataRequestSession{
		Token:      clientToken,
		ClientInfo: clientInfo,
	}))

	server.assertSentMessages(t, []*Message{
		NewMessage(MessageSession, &DataSession{
			Token: clientToken,
		}),
	})

	if server.clientInfo != clientInfo {
		t.Fatal("Server failed to store ClientInfo")
	}
}

func TestClientDoesNotAcceptSessionWithIncorrectToken(t *testing.T) {
	client := NewMockClient()
	clientState := NewClientState(client)
	clientState.state = ClientStateGettingSession
	validToken := ClientToken{1, 2, 3, 4, 5}
	invalidToken := ClientToken{5, 4, 3, 2, 1}
	clientState.token = validToken

	clientState.handleMessage(NewMessage(MessageSession, &DataSession{
		Token: invalidToken,
	}))

	if clientState.state != ClientStateGettingSession {
		t.Fatalf("Client in state %v, expected %v", clientState.state, ClientStateGettingSession)
	}
}

func TestClientChecksFilesAfterGettingSession(t *testing.T) {
	client := NewMockClient()
	clientState := NewClientState(client)
	clientState.state = ClientStateGettingSession
	token := ClientToken{1, 2, 3, 4, 5}
	clientState.token = token

	clientState.handleMessage(NewMessage(MessageSession, &DataSession{
		Token: token,
	}))

	if clientState.state != ClientStateCheckingFiles {
		t.Fatalf("Client in state %v, expected %v", clientState.state, ClientStateCheckingFiles)
	}

	if client.enumeratedFiles != true {
		t.Fatal("Client did not start enumerating files")
	}
}

func TestClientSendsMessagesEnumeratingFiles(t *testing.T) {
	client := NewMockClient()
	client.InitializeFile("file1", MockFileInfo{}, FileDedupeHash{})
	client.InitializeFile("file2", MockFileInfo{}, FileDedupeHash{})
	client.InitializeFile("file3", MockFileInfo{}, FileDedupeHash{})

	clientState := NewClientState(client)
	clientState.state = ClientStateCheckingFiles
	clientState.sendFiles()

	client.WaitForEnumerationFinish()
	client.assertSentMessages(t, []*Message{
		NewMessage(MessageStartFile, &DataStartFile{
			Id:       NewFileId("file1"),
			Filename: "file1",
			Modified: time.Time{},
		}),
		NewMessage(MessageStartFile, &DataStartFile{
			Id:       NewFileId("file2"),
			Filename: "file2",
			Modified: time.Time{},
		}),
		NewMessage(MessageStartFile, &DataStartFile{
			Id:       NewFileId("file3"),
			Filename: "file3",
			Modified: time.Time{},
		}),
	})
}

func TestServerRequiresValidSession(t *testing.T) {

}

func TestServerRequestsDedupeForMissingFile(t *testing.T) {
	filename := "file1"
	fileId := NewFileId(filename)

	server := NewMockServer()
	serverState := NewServerState(server)

	serverState.handleMessage(NewMessage(MessageStartFile, &DataStartFile{
		Id:       fileId,
		Filename: filename,
		Modified: time.Time{},
	}))

	server.assertSentMessages(t, []*Message{
		NewMessage(MessageFileMissing, &DataFileMissing{
			Id: fileId,
		}),
	})
}

func TestServerSendsOKWhenFileExists(t *testing.T) {
	filename := "file1"
	fileId := NewFileId(filename)

	server := NewMockServer()
	server.InitializeFile(filename, MockFileInfo{}, FileDedupeHash{})
	serverState := NewServerState(server)

	serverState.handleMessage(NewMessage(MessageStartFile, &DataStartFile{
		Id:       fileId,
		Filename: filename,
		Modified: time.Time{},
	}))

	server.assertSentMessages(t, []*Message{
		NewMessage(MessageFileOK, &DataFileOK{
			Id: fileId,
		}),
	})
}

func TestClientSendsDedupeWhenFileMissing(t *testing.T) {
	filename := "file1"
	fileId := NewFileId(filename)
	dedupe := FileDedupeHash{1, 2, 3, 4, 5}

	// Setup client to just after sending files
	client := NewMockClient()
	client.InitializeFile(filename, MockFileInfo{}, dedupe)
	clientState := NewClientState(client)
	clientState.state = ClientStateCheckingFiles
	clientState.InitializeFile(filename, ClientStateCheckingStatus)

	clientState.handleMessage(NewMessage(MessageFileMissing, &DataFileMissing{
		Id: fileId,
	}))

	client.assertSentMessages(t, []*Message{
		NewMessage(MessageDedupeHash, &DataDedupeHash{
			Id:   fileId,
			Hash: dedupe,
		}),
	})
}

func TestServerRequestsBinaryWhenDedupeDifferent(t *testing.T) {
	filename := "file1"
	fileId := NewFileId(filename)
	dedupe := FileDedupeHash{1, 2, 3, 4, 5}
	differentDedupe := FileDedupeHash{5, 4, 3, 2, 1}

	server := NewMockServer()
	server.InitializeFile(filename, MockFileInfo{}, differentDedupe)
	serverState := NewServerState(server)
	serverState.InitializeFile(filename, ServerStateCheckingDedupeHash)

	serverState.handleMessage(NewMessage(MessageDedupeHash, &DataDedupeHash{
		Id:   fileId,
		Hash: dedupe,
	}))

	server.assertSentMessages(t, []*Message{
		NewMessage(MessageRequestBinary, &DataRequestBinary{
			Id:        fileId,
			ChunkSize: 1000,
		}),
	})
}

func TestServerSendsOKWhenDedupeExists(t *testing.T) {
	filename := "file1"
	fileId := NewFileId(filename)
	dedupe := FileDedupeHash{1, 2, 3, 4, 5}

	server := NewMockServer()
	server.InitializeFile(filename, MockFileInfo{}, dedupe)
	serverState := NewServerState(server)
	serverState.InitializeFile(filename, ServerStateCheckingDedupeHash)

	serverState.handleMessage(NewMessage(MessageDedupeHash, &DataDedupeHash{
		Id:   fileId,
		Hash: dedupe,
	}))

	server.assertSentMessages(t, []*Message{
		NewMessage(MessageFileOK, &DataFileOK{
			Id: fileId,
		}),
	})
}

func TestClientSendsDataWhenServerRequestsBinary(t *testing.T) {
	filename := "file"
	fileId := NewFileId(filename)

	client := NewMockClient()
	client.InitializeFile(filename, MockFileInfo{
		size:    15,
		modTime: time.Time{},
	}, FileDedupeHash{})
	clientState := NewClientState(client)
	clientState.state = ClientStateCheckingFiles
	clientState.InitializeFile(filename, ClientStateCheckingStatus)

	clientState.handleMessage(NewMessage(MessageRequestBinary, &DataRequestBinary{
		Id:        fileId,
		ChunkSize: 5,
	}))

	clientState.handleMessage(NewMessage(MessageRequestBinary, &DataRequestBinary{
		Id:        fileId,
		ChunkSize: 6,
	}))

	clientState.handleMessage(NewMessage(MessageRequestBinary, &DataRequestBinary{
		Id:        fileId,
		ChunkSize: 200,
	}))

	client.assertSentMessages(t, []*Message{
		NewMessage(MessageFileChunk, &DataFileChunk{
			Id:       fileId,
			Filesize: 15,
			Offset:   0,
			Chunk:    []byte{'f', 'i', 'l', 'e', 'f'},
		}),
		NewMessage(MessageFileChunk, &DataFileChunk{
			Id:       fileId,
			Filesize: 15,
			Offset:   5,
			Chunk:    []byte{'i', 'l', 'e', 'f', 'i', 'l'},
		}),
		NewMessage(MessageFileChunk, &DataFileChunk{
			Id:       fileId,
			Filesize: 15,
			Offset:   11,
			Chunk:    []byte{'e', 'f', 'i', 'l'},
		}),
	})
}

func TestServerSavesDataWhenClientSendsIt(t *testing.T) {
	filename := "file"
	fileId := NewFileId(filename)

	server := NewMockServer()
	server.InitializeFile(filename, MockFileInfo{}, FileDedupeHash{})
	serverState := NewServerState(server)
	serverState.InitializeFile(filename, ServerStateGettingBinary)

	serverState.handleMessage(NewMessage(MessageFileChunk, &DataFileChunk{
		Id:       fileId,
		Filesize: 15,
		Offset:   0,
		Chunk:    []byte{'f', 'i', 'l', 'e', 'f'},
	}))

	if !bytes.Equal(server.storedFiles[filename], []byte{'f', 'i', 'l', 'e', 'f'}) {
		t.Fatal("Server stored wrong information")
	}
}
