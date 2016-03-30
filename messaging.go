package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/kalafut/imohash"
	"log"
	"sync"
	"time"
)

type FileId string // TODO: Fixed size bytes, host integration

func (id FileId) String() string { return string(id[:4]) }

func NewFileId(str string) FileId {
	hasher := md5.New()
	hasher.Write([]byte(str))
	return FileId(hex.EncodeToString(hasher.Sum(nil)))
}

type FileDedupeHash [imohash.Size]byte

const VerificationHashSize = 32

type FileVerificationHash [VerificationHashSize]byte

const FingerprintSize = 32

type Fingerprint [FingerprintSize]byte

// Message information
type MessageType int

const (
	MessageRequestSession   MessageType = iota // Client -> Server {nil}
	MessageSession                             // Client <- Server {session}
	MessageStartFile                           // Client -> Server {id, filename, modified}
	MessageFileOK                              // Client <- Server {id}
	MessageFileVerification                    // Client <- Server {id, file date, md5}
	MessageFileMissing                         // Client <- Server {id}
	MessageDedupeHash                          // Client -> Server {id, dedupehash}
	MessageRequestBinary                       // Client <- Server {id}
	MessageFileChunk                           // Client -> Server {filesize, offset, chunk}
	MessageDone                                // Client -> Server
)

func (mt MessageType) String() string {
	switch mt {
	case MessageRequestSession:
		return "Request Session"
	case MessageSession:
		return "Session"
	case MessageStartFile:
		return "Start File"
	case MessageFileOK:
		return "File OK"
	case MessageFileVerification:
		return "File Verification"
	case MessageFileMissing:
		return "File Missing"
	case MessageDedupeHash:
		return "Deduplication Hash"
	case MessageRequestBinary:
		return "Request Binary"
	case MessageFileChunk:
		return "File Chunk"
	case MessageDone:
		return "Done"
	}
	return "Unknown"
}

type Message struct {
	Type MessageType
	d    []byte
}

func (msg Message) String() string {
	return fmt.Sprintf("M %v", msg.Type)
}

func NewMessage(t MessageType, data interface{}) *Message {
	msg := &Message{
		Type: t,
	}

	if data != nil {
		msg.encode(data)
	}

	return msg
}

func (msg *Message) encode(data interface{}) *Message {
	buf := bytes.NewBuffer(msg.d)
	enc := gob.NewEncoder(buf)
	enc.Encode(data)
	msg.d = buf.Bytes()
	return msg
}

func (msg *Message) Decode(data interface{}) error {
	enc := gob.NewDecoder(bytes.NewBuffer(msg.d))
	return enc.Decode(data)
}

type DataRequestSession struct {
	Token       ClientToken
	BackupName  string
	Fingerprint Fingerprint
}

type DataSession struct {
	Token ClientToken
}

type DataFileId struct {
	Id FileId
}

type DataStartFile struct {
	Id       FileId
	Filename string
	Modified time.Time
}

type DataFileOK struct {
	Id FileId
}

type DataFileVerification struct {
	Id   FileId
	Hash FileVerificationHash
}

type DataFileMissing struct {
	Id FileId
}

type DataDedupeHash struct {
	Id   FileId
	Hash FileDedupeHash
}

type DataRequestBinary struct {
	Id        FileId
	ChunkSize int64
}

type DataFileChunk struct {
	Id       FileId
	Filesize int64
	Offset   int64
	Chunk    []byte
}

const ChunkSize = 1

// Client state
type ClientStateEnum int

const (
	// Client state
	ClientStateInit           ClientStateEnum = iota // No messages sent
	ClientStateGettingSession                        // Pulling session from server
	ClientStateCheckingFiles                         // Checking files
	ClientStateDone                                  // Backup done

	// Client file state
	ClientStateCheckingStatus // Sent modified time to server
	ClientStateFileOK         // File has been received by server
	ClientStateSendingBinary  // Server requested binary
)

func (state ClientStateEnum) String() string {
	switch state {
	case ClientStateInit:
		return "Init"
	case ClientStateGettingSession:
		return "Getting Session"
	case ClientStateCheckingFiles:
		return "Checking Files"
	case ClientStateDone:
		return "Done"
	case ClientStateCheckingStatus:
		return "Checking File Status"
	case ClientStateFileOK:
		return "File OK"
	case ClientStateSendingBinary:
		return "Sending Binary"
	}
	return "Unknown"
}

func NewRandomBytes(size int) []byte {
	d := make([]byte, size)
	_, err := rand.Read([]byte(d))
	if err != nil {
		log.Fatal("error:", err)
	}
	return d
}

func NewClientToken() ClientToken {
	// TODO: Sign token
	b := [ClientTokenSize]byte{}
	copy(b[:], NewRandomBytes(ClientTokenSize))
	return ClientToken(b)

}

type ClientInterface interface {
	Send(*Message)
	GetFileInfo(string) PartialFileInfo
	GetDedupeHash(string) FileDedupeHash
	GetFileChunk(filename string, size int64, offset int64) []byte
	Enumerate() <-chan string
	GetName() string
	GetFingerprint() (Fingerprint, error)
}

const ClientTokenSize = 32

type ClientToken [ClientTokenSize]byte

type ClientState struct {
	client         ClientInterface
	state          ClientStateEnum
	token          ClientToken
	fileState      map[FileId]*ClientFileState
	fileStateMutex sync.Mutex
}

func (cs ClientState) String() string { return fmt.Sprintf("C %v", cs.state) }

func NewClientState(client ClientInterface) *ClientState {
	cs := &ClientState{
		client:    client,
		state:     ClientStateInit,
		token:     NewClientToken(),
		fileState: make(map[FileId]*ClientFileState),
	}

	return cs
}

func (cs *ClientState) requestSession() error {
	if cs.state != ClientStateInit {
		return errors.New("Requesting session when not init")
	}
	cs.state = ClientStateGettingSession
	return cs.sendRequestSession()
}

func (cs *ClientState) handleMessage(msg *Message) error {
	if cs.state == ClientStateGettingSession {
		if msg.Type == MessageSession {
			sessionData := DataSession{}
			msg.Decode(&sessionData)
			if sessionData.Token != cs.token {
				return errors.New("Recieved invalid token")
			}
			cs.state = ClientStateCheckingFiles
			cs.sendFiles()
			return nil
		}
	} else if cs.state == ClientStateCheckingFiles {
		if msg.Type == MessageFileOK ||
			msg.Type == MessageFileVerification ||
			msg.Type == MessageFileMissing ||
			msg.Type == MessageRequestBinary {
			return cs.handleFileMessage(msg)
		}
	}
	return errors.New(fmt.Sprintf("CS unhandled message: %v", msg))
}

func (cs *ClientState) handleFileMessage(msg *Message) error {
	fileIdData := DataFileId{}
	err := msg.Decode(&fileIdData)
	if err != nil {
		return err
	}
	fileId := fileIdData.Id
	cs.fileStateMutex.Lock()
	cfs, ok := cs.fileState[fileId]
	cs.fileStateMutex.Unlock()

	if ok {
		preState := cfs.state
		err := cfs.handleMessage(msg)
		if err != nil {
			return err
		}

		if preState != cfs.state {
			log.Printf("%v transitioned from %v", cfs, preState)
		}

		return nil
	}
	return errors.New("FileId not found")
}

func (cs *ClientState) sendRequestSession() error {
	fingerprint, err := cs.client.GetFingerprint()
	if err != nil {
		return err
	}
	cs.client.Send(NewMessage(MessageRequestSession, &DataRequestSession{
		Token:       cs.token,
		BackupName:  cs.client.GetName(),
		Fingerprint: fingerprint,
	}))
	return nil
}

func (cs *ClientState) sendFiles() {
	for filename := range cs.client.Enumerate() {
		cfs := NewClientFileState(cs, filename)
		cs.fileStateMutex.Lock()
		cs.fileState[cfs.id] = cfs
		cs.fileStateMutex.Unlock()
		cfs.startFile()
	}
}

type ClientFileState struct {
	cs       *ClientState
	id       FileId
	state    ClientStateEnum
	filename string
	filesize int64
	offset   int64
	modTime  time.Time
}

func (cfs ClientFileState) String() string {
	if cfs.state != ClientStateSendingBinary {
		return fmt.Sprintf("CF[%v] %v", cfs.id, cfs.state)
	} else {
		return fmt.Sprintf("CF[%v] %v %v/%v", cfs.id, cfs.state, cfs.offset, cfs.filesize)
	}
}

func NewClientFileState(cs *ClientState, filename string) *ClientFileState {
	// Assert file exists

	fileInfo := cs.client.GetFileInfo(filename)

	cfs := &ClientFileState{
		cs:       cs,
		id:       NewFileId(filename),
		state:    ClientStateInit,
		filename: filename,
		filesize: fileInfo.Size(),
		modTime:  fileInfo.ModTime(),
		offset:   0,
	}

	return cfs
}

func (cfs *ClientFileState) startFile() {
	cfs.state = ClientStateCheckingStatus
	cfs.sendStartFile()
}

func (cfs *ClientFileState) handleMessage(msg *Message) error {
	if cfs.state == ClientStateCheckingStatus {
		if msg.Type == MessageFileOK {
			cfs.state = ClientStateFileOK
			return nil
		}
		if msg.Type == MessageFileVerification {
			// TODO: hash file for verification
			return nil
		}
		if msg.Type == MessageFileMissing {
			cfs.sendDedupeHash()
			return nil
		}
		if msg.Type == MessageRequestBinary {
			cfs.state = ClientStateSendingBinary
			fileChunkData := DataRequestBinary{}
			msg.Decode(&fileChunkData)
			cfs.sendFileChunk(fileChunkData.ChunkSize)
			return nil
		}
	} else if cfs.state == ClientStateSendingBinary {
		if msg.Type == MessageRequestBinary {
			fileChunkData := DataRequestBinary{}
			msg.Decode(&fileChunkData)
			cfs.sendFileChunk(fileChunkData.ChunkSize)
			return nil
		}
		if msg.Type == MessageFileOK {
			cfs.state = ClientStateFileOK
			return nil
		}
	}
	return errors.New(fmt.Sprintf("CFS unhandled message: %v", msg))
}

func (cfs *ClientFileState) sendStartFile() {
	cfs.cs.client.Send(NewMessage(MessageStartFile, &DataStartFile{
		Id:       cfs.id,
		Filename: cfs.filename,
		Modified: cfs.modTime,
	}))
}

func (cfs *ClientFileState) sendDedupeHash() {
	cfs.cs.client.Send(NewMessage(MessageDedupeHash, &DataDedupeHash{
		Id:   cfs.id,
		Hash: cfs.cs.client.GetDedupeHash(cfs.filename),
	}))
}

func (cfs *ClientFileState) sendFileChunk(chunkSize int64) {
	if chunkSize+cfs.offset > cfs.filesize {
		chunkSize = cfs.filesize - cfs.offset
	}
	data := cfs.cs.client.GetFileChunk(cfs.filename, chunkSize, cfs.offset)
	msgData := DataFileChunk{
		Id:       cfs.id,
		Filesize: cfs.filesize,
		Offset:   cfs.offset,
		Chunk:    data,
	}
	msg := NewMessage(MessageFileChunk, msgData)
	cfs.offset += int64(len(data))

	cfs.cs.client.Send(msg)
}

// Server state
type ServerStateEnum int

type ServerInterface interface {
	Send(*Message)
	HasFile(string) bool
	IsExpired(string) bool
	GetVerification(FileId) FileVerificationHash
	HasDedupeHash(FileDedupeHash) bool
	HasVerificationHash(FileVerificationHash) bool
	StoreBinary(string, []byte)
}

const (
	ServerStateInit                     ServerStateEnum = iota // Sent file, waiting for end or hash
	ServerStateCheckingDedupeHash                              // Awaiting file hash from client
	ServerStateCheckingVerificationHash                        // Awaiting file hash from client
	ServerStateGettingBinary                                   // Awaiting binary from client
	ServerStateEndLink                                         // Link file to existing file
	ServerStateEndBinary                                       // Write new binary data
)

func (state ServerStateEnum) String() string {
	switch state {
	case ServerStateInit:
		return "Init"
	case ServerStateCheckingDedupeHash:
		return "Checking Dedupe Hash"
	case ServerStateCheckingVerificationHash:
		return "Checking Verif Hash"
	case ServerStateGettingBinary:
		return "Getting Binary"
	case ServerStateEndLink:
		return "Finished Link"
	case ServerStateEndBinary:
		return "Finished Binary"
	}
	return "Unknown"
}

type ServerState struct {
	server         ServerInterface
	name           string
	fingerprint    Fingerprint
	fileState      map[FileId]*ServerFileState
	fileStateMutex sync.Mutex
}

func NewServerState(server ServerInterface) *ServerState {
	ss := &ServerState{
		server:    server,
		fileState: make(map[FileId]*ServerFileState),
	}

	return ss
}

func (ss *ServerState) handleMessage(msg *Message) error {
	if msg.Type == MessageRequestSession {
		dataRequestSession := DataRequestSession{}
		msg.Decode(&dataRequestSession)
		ss.name = dataRequestSession.BackupName
		if len(ss.name) == 0 {
			return errors.New("Zero length backup name")
		}
		ss.fingerprint = dataRequestSession.Fingerprint
		// TODO: Err on invalid fingerprint
		ss.sendSession(dataRequestSession.Token)
		return nil
	}

	// If starting a file create the FileState to handle it
	if msg.Type == MessageStartFile {
		fileData := DataStartFile{}
		msg.Decode(&fileData)
		sfs := NewServerFileState(ss.server, fileData.Filename)
		// TODO: Fix private
		if sfs.id != fileData.Id {
			log.Fatal("ServerFileState and message id disagree")
		}

		ss.fileStateMutex.Lock()
		ss.fileState[sfs.id] = sfs
		ss.fileStateMutex.Unlock()
	}

	if msg.Type == MessageStartFile ||
		msg.Type == MessageDedupeHash ||
		msg.Type == MessageFileVerification ||
		msg.Type == MessageFileChunk {
		return ss.dispatchMessageToFileState(msg)
	}
	return errors.New(fmt.Sprintf("SS unhandled message: %v", msg))
}

func (ss *ServerState) dispatchMessageToFileState(msg *Message) error {
	// Known valid session, not known valid id
	idData := DataFileId{}
	msg.Decode(&idData)
	ss.fileStateMutex.Lock()
	sfs, ok := ss.fileState[idData.Id]
	ss.fileStateMutex.Unlock()
	if !ok {
		return errors.New("Dispatching message to nonexistant id")
	}
	preState := sfs.state
	err := sfs.handleMessage(msg)
	if sfs.state != preState {
		log.Printf("%v transitioned from %v", sfs, preState)
	}
	return err
}

func (ss *ServerState) sendSession(token ClientToken) {
	ss.server.Send(NewMessage(MessageSession, &DataSession{
		Token: token,
	}))
}

type ServerFileState struct {
	server     ServerInterface
	id         FileId
	state      ServerStateEnum
	filename   string
	dedupeHash FileDedupeHash
	filesize   int64
	offset     int64
	network    NetworkInterface
}

func (sfs ServerFileState) String() string {
	if sfs.state != ServerStateGettingBinary {
		return fmt.Sprintf("SF[%v] %v", sfs.id, sfs.state)
	} else {
		return fmt.Sprintf("SF[%v] %v %v/%v", sfs.id, sfs.state, sfs.offset, sfs.filesize)
	}
}

func NewServerFileState(server ServerInterface, filename string) *ServerFileState {

	return &ServerFileState{
		server:   server,
		state:    ServerStateInit,
		filename: filename,
		id:       NewFileId(filename),
	}
}

func (sfs *ServerFileState) handleMessage(msg *Message) error {
	if sfs.state == ServerStateInit {
		if msg.Type == MessageStartFile {
			fileData := DataStartFile{}
			if err := msg.Decode(&fileData); err != nil {
				return err
			}
			if !sfs.server.HasFile(fileData.Filename) {
				// TODO: return error from inside message to here
				sfs.state = ServerStateCheckingDedupeHash
				sfs.sendFileMissing()
			} else if sfs.server.IsExpired(fileData.Filename) {
				sfs.state = ServerStateCheckingVerificationHash
				sfs.sendFileVerification()
			} else {
				sfs.state = ServerStateEndLink
				sfs.sendFileOK()
			}
			return nil
		}
	} else if sfs.state == ServerStateCheckingDedupeHash {
		if msg.Type == MessageDedupeHash {
			dedupeData := DataDedupeHash{}
			if err := msg.Decode(&dedupeData); err != nil {
				return err
			}
			if sfs.server.HasDedupeHash(dedupeData.Hash) {
				sfs.state = ServerStateEndLink
				sfs.sendFileOK()
			} else {
				sfs.state = ServerStateGettingBinary
				sfs.sendRequestBinary()
			}
			return nil
		}
	} else if sfs.state == ServerStateCheckingVerificationHash {
		if msg.Type == MessageFileVerification {
			verificationData := DataFileVerification{}
			if err := msg.Decode(&verificationData); err != nil {
				return err
			}
			if sfs.server.HasVerificationHash(verificationData.Hash) {
				sfs.state = ServerStateEndLink
				sfs.sendFileOK()
			} else {
				sfs.state = ServerStateGettingBinary
				sfs.sendRequestBinary()
			}
			return nil
		}
	} else if sfs.state == ServerStateGettingBinary {
		if msg.Type == MessageFileChunk {
			dataChunk := DataFileChunk{}
			msg.Decode(&dataChunk)
			sfs.server.StoreBinary(sfs.filename, dataChunk.Chunk)
			if dataChunk.Filesize == dataChunk.Offset+int64(len(dataChunk.Chunk)) {
				sfs.state = ServerStateEndBinary
				sfs.sendFileOK()
			} else {
				sfs.sendRequestBinary()
			}
			return nil
		}
	}
	return errors.New(fmt.Sprintf("%v unhandled message: %v", sfs, msg))
}

func (sfs *ServerFileState) sendFileOK() {
	sfs.server.Send(NewMessage(MessageFileOK, &DataFileOK{
		Id: sfs.id,
	}))
}

func (sfs *ServerFileState) sendFileVerification() {
	sfs.server.Send(NewMessage(MessageFileVerification, &DataFileVerification{
		Id:   sfs.id,
		Hash: sfs.server.GetVerification(sfs.id),
	}))
}

func (sfs *ServerFileState) sendFileMissing() {
	sfs.server.Send(NewMessage(MessageFileMissing, &DataFileMissing{
		Id: sfs.id,
	}))
}

func (sfs *ServerFileState) sendRequestBinary() {
	sfs.server.Send(NewMessage(MessageRequestBinary, &DataRequestBinary{
		Id:        sfs.id,
		ChunkSize: 1000,
	}))
}
