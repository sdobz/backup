package main

import (
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"github.com/kalafut/imohash"
	"log"
	"os"
	"time"
	"fmt"
	"io"
)

// File state
type FileId string // TODO: Fixed size bytes

func (id FileId) String() string {
	return string(id[:4])
}

type FileDedupeHash [imohash.Size]byte
type FileVerificationHash string

func NewFileId(str string) FileId {
	hasher := md5.New()
	hasher.Write([]byte(str))
	return FileId(hex.EncodeToString(hasher.Sum(nil)))
}

// Message information
type MessageType int

const (
	// All messages include id
	MessageStartFile        MessageType = iota // Client -> Server {filename, modified}
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
	id FileId
	t  MessageType
	d  []byte
}

func (msg Message) String() string {
	return fmt.Sprintf("M[%s] %v", msg.id[:4], msg.t)
}

func NewMessage(id FileId, t MessageType) *Message {
	return &Message{
		id: id,
		t:  t,
	}
}

func (msg *Message) encode(data interface{}) *Message {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	enc.Encode(data)
	msg.d = buf.Bytes()
	return msg
}

func (msg *Message) decode(data interface{}) {
	enc := gob.NewDecoder(bytes.NewBuffer(msg.d))
	enc.Decode(data)
}

type DataStartFile struct {
	Filename string
	Modified time.Time
}

type DataFileVerification struct {
	Hash FileVerificationHash
}

type DataDedupeHash struct {
	Hash FileDedupeHash
}

type DataFileChunk struct {
	Filesize int64
	Offset   int64
	Chunk    []byte
}

const ChunkSize = 1

// Client state
type ClientState int

const (
	ClientStateInit           ClientState = iota // No messages sent
	ClientStateCheckingStatus                    // Sent modified time to server
	ClientStateFileOK                            // File received by server
	ClientStateSendingBinary                     // Server requested binary
)

func (state ClientState) String() string {
	switch state {
	case ClientStateInit:
		return "Init"
	case ClientStateCheckingStatus:
		return "Checking File Status"
	case ClientStateFileOK:
		return "File OK"
	case ClientStateSendingBinary:
		return "Sending Binary"
	}
	return "Unknown"
}

type BinaryState struct {
	filesize int64
	offset   int64
}

func (bs BinaryState) String() string {
	return fmt.Sprintf("%v/%v", bs.offset, bs.filesize)
}

type ClientFileState struct {
	state       ClientState
	filename    string
	id          FileId
	binaryState BinaryState
	network     NetworkInterface // TODO: Factor into handleMessage
}

func (cfs *ClientFileState) String() string {
	if cfs.state != ClientStateSendingBinary {
		return fmt.Sprintf("C[%v] %v", cfs.id, cfs.state)
	} else {
		return fmt.Sprintf("C[%v] %v %v", cfs.id, cfs.state, cfs.binaryState)
	}
}

func NewClientFileState(filename string, network NetworkInterface) (cfs *ClientFileState, err error) {
	// Assert file exists

	cfs = &ClientFileState{
		state:       ClientStateInit,
		filename:    filename,
		id:          NewFileId(filename),
		binaryState: BinaryState{},
		network:     network,
	}

	cfs.sendStartFile()
	cfs.state = ClientStateCheckingStatus

	return cfs, nil
}

func (cfs *ClientFileState) handleMessage(msg Message) error {
	if cfs.state == ClientStateCheckingStatus {
		if msg.t == MessageFileOK {
			cfs.state = ClientStateFileOK
			return nil
		}
		if msg.t == MessageFileVerification {
			// TODO: hash file for verification
			return nil
		}
		if msg.t == MessageFileMissing {
			cfs.sendDedupeHash()
			return nil
		}
		if msg.t == MessageRequestBinary {
			cfs.state = ClientStateSendingBinary
			cfs.sendFileChunk()
			return nil
		}
	} else if cfs.state == ClientStateSendingBinary {
		if msg.t == MessageRequestBinary {
			cfs.sendFileChunk()
			return nil
		}
		if msg.t == MessageFileOK {
			cfs.state = ClientStateFileOK
			return nil
		}
	}
	return errors.New("Unhandled message")
}

func (cfs *ClientFileState) sendStartFile() {
	info, err := os.Stat(cfs.filename)
	if err != nil {
		// TODO: Handle error
		log.Fatal(err)
	}

	cfs.binaryState.filesize = info.Size()

	msg := NewMessage(cfs.id, MessageStartFile)
	msg.encode(DataStartFile{
		Filename: cfs.filename,
		Modified: info.ModTime(),
	})
	cfs.network.send(*msg)
}

func (cfs *ClientFileState) sendDedupeHash() {
	var hash FileDedupeHash
	hash, err := imohash.SumFile(cfs.filename)
	if err != nil {
		// TODO: Handle error
		log.Fatal(err)
	}

	msg := NewMessage(cfs.id, MessageDedupeHash)
	msg.encode(DataDedupeHash{
		Hash: hash,
	})
	cfs.network.send(*msg)
}

func (cfs *ClientFileState) sendFileChunk() {
	_ = "breakpoint"
	file, err := os.Open(cfs.filename) // For read access.
	if err != nil {
		log.Fatal(err)
	}
	data := make([]byte, ChunkSize)
	count, err := file.Read(data)
	data = data[:count]
	if err != io.EOF && err != nil {
		log.Fatal(err)
	}
	msg := NewMessage(cfs.id, MessageFileChunk)
	msg.encode(DataFileChunk{
		Filesize: cfs.binaryState.filesize,
		Offset:   cfs.binaryState.offset,
		Chunk:    data,
	})
	cfs.network.send(*msg)

	cfs.binaryState.offset += int64(count)
}

// Server state
type ServerState int

type ServerStore interface {
	HasFile(string) bool
	IsExpired(string) bool
	GetVerification(FileId) DataFileVerification
	HasDedupeHash(FileDedupeHash) bool
	HasVerificationHash(FileVerificationHash) bool
	StoreBinary([]byte)
}

const (
	ServerStateInit                     ServerState = iota // Sent file, waiting for end or hash
	ServerStateCheckingDedupeHash                          // Awaiting file hash from client
	ServerStateCheckingVerificationHash                    // Awaiting file hash from client
	ServerStateGettingBinary                               // Awaiting binary from client
	ServerStateEndLink                                     // Link file to existing file
	ServerStateEndBinary                                   // Write new binary data
)

func (state ServerState) String() string {
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

type ServerFileState struct {
	state       ServerState
	filename    string
	id          FileId
	binaryState BinaryState
	network     NetworkInterface // TODO: Factor channel and store out
	store       ServerStore
}

func (sfs *ServerFileState) String() string {
	if sfs.state != ServerStateGettingBinary {
		return fmt.Sprintf("S[%v] %v", sfs.id, sfs.state)
	} else {
		return fmt.Sprintf("S[%v] %v %v", sfs.id, sfs.state, sfs.binaryState)
	}
}

func NewServerFileState(fileData DataStartFile, store ServerStore, network NetworkInterface) (sfs *ServerFileState, err error) {

	sfs = &ServerFileState{
		state:       ServerStateInit,
		filename:    fileData.Filename,
		id:          NewFileId(fileData.Filename),
		binaryState: BinaryState{},
		store:       store,
		network:     network,
	}

	return sfs, nil
}

func (sfs *ServerFileState) handleMessage(msg Message) error {
	if sfs.state == ServerStateInit {
		if msg.t == MessageStartFile {
			fileData := DataStartFile{}
			msg.decode(fileData)
			if !sfs.store.HasFile(fileData.Filename) {
				sfs.sendFileMissing()
				sfs.state = ServerStateCheckingDedupeHash
			} else if sfs.store.IsExpired(fileData.Filename) {
				sfs.sendFileVerification()
				sfs.state = ServerStateCheckingVerificationHash
			} else {
				sfs.sendFileOK()
				sfs.state = ServerStateEndLink
			}
			return nil
		}
	} else if sfs.state == ServerStateCheckingDedupeHash {
		if msg.t == MessageDedupeHash {
			dedupeData := DataDedupeHash{}
			msg.decode(dedupeData)
			if sfs.store.HasDedupeHash(dedupeData.Hash) {
				sfs.sendFileOK()
				sfs.state = ServerStateEndLink
			} else {
				sfs.sendRequestBinary()
				sfs.state = ServerStateGettingBinary
			}
			return nil
		}
	} else if sfs.state == ServerStateCheckingVerificationHash {
		if msg.t == MessageFileVerification {
			verificationData := DataFileVerification{}
			msg.decode(verificationData)
			if sfs.store.HasVerificationHash(verificationData.Hash) {
				sfs.sendFileOK()
				sfs.state = ServerStateEndLink
			} else {
				sfs.sendRequestBinary()
				sfs.state = ServerStateGettingBinary
			}
			return nil
		}
	} else if sfs.state == ServerStateGettingBinary {
		if msg.t == MessageFileChunk {
			dataChunk := DataFileChunk{}
			msg.decode(&dataChunk)
			sfs.store.StoreBinary(dataChunk.Chunk)
			if dataChunk.Filesize == dataChunk.Offset + int64(len(dataChunk.Chunk)) {
				sfs.sendFileOK()
				sfs.state = ServerStateEndBinary
			} else {
				sfs.sendRequestBinary()
			}
			return nil
		}
	}
	return errors.New("Unhandled message")
}

func (sfs *ServerFileState) newMessage(t MessageType) Message {
	return Message{
		id: sfs.id,
		t:  t,
	}
}

func (sfs *ServerFileState) sendFileOK() {
	sfs.network.send(*NewMessage(sfs.id, MessageFileOK))
}

func (sfs *ServerFileState) sendFileVerification() {
	// TODO: Factor getExpired out
	msg := *NewMessage(sfs.id, MessageFileVerification)
	sfs.network.send(msg)
}

func (sfs *ServerFileState) sendFileMissing() {
	sfs.network.send(*NewMessage(sfs.id, MessageFileMissing))
}

func (sfs *ServerFileState) sendRequestBinary() {
	sfs.network.send(*NewMessage(sfs.id, MessageRequestBinary))
}
