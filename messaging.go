package main

import (
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/kalafut/imohash"
	"os"
	"time"
)

type Session string

func (session Session) String() string { return string(session[:4]) }

type FileId string // TODO: Fixed size bytes, host integration

func (id FileId) String() string { return string(id[:4]) }

func NewFileId(str string) FileId {
	hasher := md5.New()
	hasher.Write([]byte(str))
	return FileId(hex.EncodeToString(hasher.Sum(nil)))
}

type FileDedupeHash [imohash.Size]byte
type FileVerificationHash string

// Message information
type MessageType int

const (
	// All messages include id
	MessageRequestSession   MessageType = iota // Client -> Server
	MessageStartFile                           // Client -> Server {filename, modified}
	MessageFileOK                              // Client <- Server
	MessageFileVerification                    // Client <- Server {file date, md5}
	MessageFileMissing                         // Client <- Server
	MessageDedupeHash                          // Client -> Server {dedupehash}
	MessageRequestBinary                       // Client <- Server
	MessageFileChunk                           // Client -> Server {filesize, offset, chunk}
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
	}
	return "Unknown"
}

type Message struct {
	Session Session
	T       MessageType
	d       []byte
}

func (msg Message) String() string {
	return fmt.Sprintf("M[%s] %v", msg.Session[:4], msg.T)
}

func NewMessage(t MessageType, d *interface{}) *Message {
	msg := &Message{
		T: t,
	}

	if d != nil {
		msg.encode(d)
	}

	return msg
}

func (msg *Message) encode(data *interface{}) *Message {
	buf := bytes.NewBuffer(&msg.d)
	enc := gob.NewEncoder(&buf)
	enc.Encode(data)
	msg.d = buf.Bytes()
	return msg
}

func (msg *Message) Decode() (d *interface{}, err error) {
	enc := gob.NewDecoder(bytes.NewBuffer(msg.d))
	err = enc.Decode(d)
	if err != nil {
		return nil, err
	}
	return d, nil
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

type ClientInterface interface {
	Send(Message)
	GetFileInfo(string) os.FileInfo
	GetDedupeHash(string) FileDedupeHash
	GetFileChunk(string, int) []byte
}

type ClientState struct {
	session   Session
	state     ClientStateEnum
	fileState map[FileId]ClientFileState
}

func (cs *ClientState) String() string { return fmt.Sprintf("C[%v] %v", cs.session, cs.state) }

// TODO: figure out client/server session
// TODO: Implement ClientState handleMessage

func NewClientState(client ClientInterface) *ClientState {
	cs := ClientState{
		session:   "",
		state:     ClientStateInit,
		fileState: make(map[FileId]ClientFileState),
	}

	cs.sendRequestSession(client)
	cs.state = ClientStateGettingSession

	return cs
}

func (cs *ClientState) sendRequestSession(client ClientInterface) {
	client.Send(NewMessage(MessageRequestSession, nil))
}

type ClientFileState struct {
	id       FileId
	state    ClientStateEnum
	filename string
	filesize int64
	offset   int64
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
		if msg.T == MessageFileOK {
			cfs.state = ClientStateFileOK
			return nil
		}
		if msg.T == MessageFileVerification {
			// TODO: hash file for verification
			return nil
		}
		if msg.T == MessageFileMissing {
			cfs.sendDedupeHash(client)
			return nil
		}
		if msg.T == MessageRequestBinary {
			cfs.state = ClientStateSendingBinary
			cfs.sendFileChunk(client)
			return nil
		}
	} else if cfs.state == ClientStateSendingBinary {
		if msg.T == MessageRequestBinary {
			cfs.sendFileChunk(client)
			return nil
		}
		if msg.T == MessageFileOK {
			cfs.state = ClientStateFileOK
			return nil
		}
	}
	return errors.New("Unhandled message")
}

func (cfs *ClientFileState) sendStartFile(client ClientInterface, modTime time.Time) {
	msg := NewMessage(MessageStartFile, &DataStartFile{
		Id:       cfs.id,
		Filename: cfs.filename,
		Modified: modTime,
	})
	client.Send(*msg)
}

func (cfs *ClientFileState) sendDedupeHash(client ClientInterface) {
	msg := NewMessage(MessageDedupeHash, &DataDedupeHash{
		Id:   cfs.id,
		Hash: client.GetDedupeHash(cfs.filename),
	})
	client.Send(*msg)
}

func (cfs *ClientFileState) sendFileChunk(client ClientInterface) {
	data := client.GetFileChunk(cfs.filename, cfs.offset)
	msg := NewMessage(MessageFileChunk, DataFileChunk{
		Filesize: cfs.filesize,
		Offset:   cfs.offset,
		Chunk:    data,
	})
	cfs.offset += len(data)

	client.Send(*msg)
}

// Server state
type ServerStateEnum int

type ServerSessionInterface interface {
	Send(Message)
	HasFile(string) bool
	IsExpired(string) bool
	GetVerification(FileId) DataFileVerification
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

type ServerSessionState struct {
	session   Session
	fileState map[FileId]ServerFileState
}

type ServerFileState struct {
	id         FileId
	state      ServerStateEnum
	filename   string
	dedupeHash FileDedupeHash
	filesize   int64
	offset     int64
}

func (sfs *ServerFileState) String() string {
	if sfs.state != ServerStateGettingBinary {
		return fmt.Sprintf("S[%v] %v", sfs.id, sfs.state)
	} else {
		return fmt.Sprintf("S[%v] %v %v/%v", sfs.id, sfs.state, sfs.offset, sfs.filesize)
	}
}

func NewServerFileState(fileData DataStartFile, network NetworkInterface) (sfs *ServerFileState, err error) {

	sfs = &ServerFileState{
		state:    ServerStateInit,
		filename: fileData.Filename,
		id:       NewFileId(fileData.Filename),
		network:  network,
	}

	return sfs, nil
}

func (sfs *ServerFileState) handleMessage(server ServerSessionInterface, msg Message) error {
	if sfs.state == ServerStateInit {
		if msg.T == MessageStartFile {
			fileData := DataStartFile(msg.Decode())
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
		if msg.T == MessageDedupeHash {
			dedupeData := DataDedupeHash(msg.Decode())
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
		if msg.T == MessageFileVerification {
			verificationData := DataFileVerification(msg.Decode())
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
		if msg.T == MessageFileChunk {
			dataChunk := DataFileChunk(msg.Decode())
			server.StoreBinary(dataChunk.Chunk)
			if dataChunk.Filesize == dataChunk.Offset+int64(len(dataChunk.Chunk)) {
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

func (sfs *ServerFileState) sendFileOK(server ServerSessionInterface) {
	server.Send(NewMessage(MessageFileOK, &DataFileOK{
		Id: sfs.id,
	}))
}

func (sfs *ServerFileState) sendFileVerification(server ServerSessionInterface) {
	server.Send(NewMessage(MessageFileVerification, &DataFileVerification{
		Id:   sfs.id,
		Hash: server.GetVerification(sfs.id),
	}))
}

func (sfs *ServerFileState) sendFileMissing(server ServerSessionInterface) {
	server.Send(NewMessage(MessageFileMissing, &DataFileMissing{
		Id: sfs.id,
	}))
}

func (sfs *ServerFileState) sendRequestBinary(server ServerSessionInterface) {
	server.Send(NewMessage(MessageRequestBinary, &DataRequestBinary{
		Id: sfs.id,
	}))
}
