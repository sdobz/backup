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
	"os"
	"time"
)


const SessionSize = 32
type Session [SessionSize]byte

func NewSession() Session {
	// TODO: C style copy?
	session := Session{}
	copy(session[:], NewRandomBytes(SessionSize))
	return session
}

func (session Session) String() string { return string(session[:4]) }

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
	MessageDone				   // Client -> Server
)

func (mt MessageType) String() string {
	switch mt {
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
	Session Session
	Type    MessageType
	d       []byte
}

func (msg Message) String() string {
	return fmt.Sprintf("M[%s] %v", msg.Session[:4], msg.Type)
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
	token ClientToken
}

type DataSession struct {
	token ClientToken
	session Session
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
	Id FileId
}

type DataFileChunk struct {
	Id       FileId
	Filesize int64
	Offset   int
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
	ClientStateFileOK         // File received by server
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
	return ClientToken(NewRandomBytes(ClientTokenSize))
}

type ClientInterface interface {
	Send(*Message)
	GetFileInfo(string) os.FileInfo
	GetDedupeHash(string) FileDedupeHash
	GetFileChunk(string, int) []byte
	Enumerate() <-chan string
}

const ClientTokenSize = 32

type ClientToken []byte

type ClientState struct {
	session   Session
	state     ClientStateEnum
	token     ClientToken
	fileState map[FileId]ClientFileState
}

func (cs *ClientState) String() string { return fmt.Sprintf("C[%v] %v", cs.session, cs.state) }

func NewClientState(client ClientInterface) *ClientState {
	cs := &ClientState{
		// no session
		state:     ClientStateInit,
		token:     NewClientToken(),
		fileState: make(map[FileId]ClientFileState),
	}

	cs.sendRequestSession(client)
	cs.state = ClientStateGettingSession

	return cs
}

func (cs *ClientState) HandleMessage(client ClientInterface, msg Message) error {
	if cs.state == ClientStateGettingSession {
		if msg.Type == MessageSession {
			sessionData := DataSession{}
			msg.Decode(&sessionData)
			cs.session = sessionData.session
			cs.state = ClientStateCheckingFiles
			return nil
		}
	} else if cs.state == ClientStateCheckingFiles {
		if msg.Type == MessageFileOK ||
			msg.Type == MessageFileVerification ||
			msg.Type == MessageFileMissing ||
			msg.Type == MessageRequestBinary {
			err := cs.handleFileMessage(client, msg)
			if err != nil {
				return err
			}
			return nil
		}
	}
	return errors.New("Unhandled Client State")
}

func (cs *ClientState) handleFileMessage(client ClientInterface, msg Message) error {
	fileIdData := DataFileId{}
	err := msg.Decode(&fileIdData)
	if err != nil {
		return err
	}
	fileId := fileIdData.Id
	if cfs, ok := cs.fileState[fileId]; ok {
		err := cfs.handleMessage(client, msg)
		if err != nil {
			return err
		}
		return nil
	}
	return errors.New("FileId not found")
}

func (cs *ClientState) sendRequestSession(client ClientInterface) {
	client.Send(NewMessage(MessageRequestSession, &DataRequestSession{
		token: cs.token,
	}))
}

func (cs *ClientState) sendFiles(client ClientInterface) {
	for filename := range client.Enumerate() {
		cfs, err := NewClientFileState(client, filename)
		if err != nil {
			log.Fatal(err)
		}

		cs.fileState[cfs.id] = *cfs
	}
}

type ClientFileState struct {
	id       FileId
	state    ClientStateEnum
	filename string
	filesize int64
	offset   int
}

func (cfs *ClientFileState) String() string {
	if cfs.state != ClientStateSendingBinary {
		return fmt.Sprintf("CF[%v] %v", cfs.id, cfs.state)
	} else {
		return fmt.Sprintf("CF[%v] %v %v/%v", cfs.id, cfs.state, cfs.offset, cfs.filesize)
	}
}

func NewClientFileState(client ClientInterface, filename string) (cfs *ClientFileState, err error) {
	// Assert file exists

	fileInfo := client.GetFileInfo(filename)

	cfs = &ClientFileState{
		id:       NewFileId(filename),
		state:    ClientStateInit,
		filename: filename,
		filesize: fileInfo.Size(),
		offset:   0,
	}

	cfs.sendStartFile(client, fileInfo.ModTime())
	cfs.state = ClientStateCheckingStatus

	return cfs, nil
}

func (cfs *ClientFileState) handleMessage(client ClientInterface, msg Message) error {
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
			cfs.sendDedupeHash(client)
			return nil
		}
		if msg.Type == MessageRequestBinary {
			cfs.state = ClientStateSendingBinary
			cfs.sendFileChunk(client)
			return nil
		}
	} else if cfs.state == ClientStateSendingBinary {
		if msg.Type == MessageRequestBinary {
			cfs.sendFileChunk(client)
			return nil
		}
		if msg.Type == MessageFileOK {
			cfs.state = ClientStateFileOK
			return nil
		}
	}
	return errors.New("Unhandled message")
}

func (cfs *ClientFileState) sendStartFile(client ClientInterface, modTime time.Time) {
	client.Send(NewMessage(MessageStartFile, &DataStartFile{
		Id:       cfs.id,
		Filename: cfs.filename,
		Modified: modTime,
	}))
}

func (cfs *ClientFileState) sendDedupeHash(client ClientInterface) {
	client.Send(NewMessage(MessageDedupeHash, &DataDedupeHash{
		Id:   cfs.id,
		Hash: client.GetDedupeHash(cfs.filename),
	}))
}

func (cfs *ClientFileState) sendFileChunk(client ClientInterface) {
	data := client.GetFileChunk(cfs.filename, cfs.offset)
	msg := NewMessage(MessageFileChunk, DataFileChunk{
		Filesize: cfs.filesize,
		Offset:   cfs.offset,
		Chunk:    data,
	})
	cfs.offset += len(data)

	client.Send(msg)
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
	StoreBinary([]byte)
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
	fileState map[Session]map[FileId]ServerFileState
}

func NewServerState(server ServerInterface) *ServerState {
	ss := &ServerState{
		fileState: make(map[Session]map[FileId]ServerFileState),
	}

	return ss
}

func (ss *ServerState) handleMessage(server ServerInterface, msg *Message) error {
	if msg.Type == MessageRequestSession {
		dataRequestSession := DataRequestSession{}
		msg.Decode(dataRequestSession)
		ss.initSession(server, dataRequestSession.token)
		return nil
	}

	// Everything below requires session
	if !ss.isValidSession(msg.Session) {
		return errors.New("Invalid session")
	}

	if msg.Type == MessageStartFile {
		fileData := DataStartFile{}
		msg.Decode(&fileData)
		sfs, err := NewServerFileState(fileData)
		if err != nil {
			log.Fatal(err)
		}
		// TODO: Fix private
		if sfs.id != fileData.Id {
			log.Fatal("ServerFileState and message id disagree")
		}

		ss.fileState[msg.Session][sfs.id] = *sfs
	}
	return errors.New("Unhandled server message")
}

func (ss *ServerState) initSession(server ServerInterface, token ClientToken) {
	session := NewSession()
	ss.fileState[session] = make(map[FileId]ServerFileState)
	ss.sendSession(server, token, session)
}

func (ss *ServerState) isValidSession(session Session) bool {
	_, ok := ss.fileState[session]
	return ok
}

func (ss *ServerState) sendSession(server ServerInterface, token ClientToken, session Session) {
	server.Send(NewMessage(MessageSession, &DataSession{
		token: token,
		session: session,
	}))
}

type ServerFileState struct {
	id         FileId
	state      ServerStateEnum
	filename   string
	dedupeHash FileDedupeHash
	filesize   int64
	offset     int
	network NetworkInterface
}

func (sfs *ServerFileState) String() string {
	if sfs.state != ServerStateGettingBinary {
		return fmt.Sprintf("S[%v] %v", sfs.id, sfs.state)
	} else {
		return fmt.Sprintf("S[%v] %v %v/%v", sfs.id, sfs.state, sfs.offset, sfs.filesize)
	}
}

func NewServerFileState(fileData DataStartFile) (sfs *ServerFileState, err error) {

	sfs = &ServerFileState{
		state:    ServerStateInit,
		filename: fileData.Filename,
		id:       NewFileId(fileData.Filename),
	}

	return sfs, nil
}

func (sfs *ServerFileState) handleMessage(server ServerInterface, msg Message) error {
	if sfs.state == ServerStateInit {
		if msg.Type == MessageStartFile {
			fileData := DataStartFile{}
			if err := msg.Decode(&fileData); err != nil {
				return err
			}
			if !server.HasFile(fileData.Filename) {
				sfs.sendFileMissing(server)
				sfs.state = ServerStateCheckingDedupeHash
			} else if server.IsExpired(fileData.Filename) {
				sfs.sendFileVerification(server)
				sfs.state = ServerStateCheckingVerificationHash
			} else {
				sfs.sendFileOK(server)
				sfs.state = ServerStateEndLink
			}
			return nil
		}
	} else if sfs.state == ServerStateCheckingDedupeHash {
		if msg.Type == MessageDedupeHash {
			dedupeData := DataDedupeHash{}
			if err := msg.Decode(&dedupeData); err != nil {
				return err
			}
			if server.HasDedupeHash(dedupeData.Hash) {
				sfs.sendFileOK(server)
				sfs.state = ServerStateEndLink
			} else {
				sfs.sendRequestBinary(server)
				sfs.state = ServerStateGettingBinary
			}
			return nil
		}
	} else if sfs.state == ServerStateCheckingVerificationHash {
		if msg.Type == MessageFileVerification {
			verificationData := DataFileVerification{}
			if err := msg.Decode(&verificationData); err != nil {
				return err
			}
			if server.HasVerificationHash(verificationData.Hash) {
				sfs.sendFileOK(server)
				sfs.state = ServerStateEndLink
			} else {
				sfs.sendRequestBinary(server)
				sfs.state = ServerStateGettingBinary
			}
			return nil
		}
	} else if sfs.state == ServerStateGettingBinary {
		if msg.Type == MessageFileChunk {
			dataChunk := DataFileChunk{}
			msg.Decode(&dataChunk)
			server.StoreBinary(dataChunk.Chunk)
			if dataChunk.Filesize == int64(dataChunk.Offset+len(dataChunk.Chunk)) {
				sfs.sendFileOK(server)
				sfs.state = ServerStateEndBinary
			} else {
				sfs.sendRequestBinary(server)
			}
			return nil
		}
	}
	return errors.New("Unhandled message")
}

func (sfs *ServerFileState) sendFileOK(server ServerInterface) {
	server.Send(NewMessage(MessageFileOK, &DataFileOK{
		Id: sfs.id,
	}))
}

func (sfs *ServerFileState) sendFileVerification(server ServerInterface) {
	server.Send(NewMessage(MessageFileVerification, &DataFileVerification{
		Id:   sfs.id,
		Hash: server.GetVerification(sfs.id),
	}))
}

func (sfs *ServerFileState) sendFileMissing(server ServerInterface) {
	server.Send(NewMessage(MessageFileMissing, &DataFileMissing{
		Id: sfs.id,
	}))
}

func (sfs *ServerFileState) sendRequestBinary(server ServerInterface) {
	server.Send(NewMessage(MessageRequestBinary, &DataRequestBinary{
		Id: sfs.id,
	}))
}
