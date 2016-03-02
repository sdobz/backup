package main

import (
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"github.com/kalafut/imohash"
	"log"
	"os"
	"time"
	"bytes"
)

// File state
type FileId string // TODO: Fixed size bytes
type FileDedupeHash [imohash.Size]byte
type FileVerificationHash string

func getFileId(str string) FileId {
	hasher := md5.New()
	hasher.Write([]byte(str))
	return FileId(hex.EncodeToString(hasher.Sum(nil)))
}

// Message information
type MessageType int

const (
	// All messages include id
	MessageTimeModified  MessageType = iota // Client -> Server {filename, modified}
	MessageFileOK                           // Client <- Server
	MessageFileVerification                 // Client <- Server {file date, md5}
	MessageFileMissing                      // Client <- Server
	MessageDedupeHash                       // Client -> Server {dedupehash}
	MessageRequestBinary                    // Client <- Server
	MessageFileChunk                        // Client -> Server {filesize, offset, chunk}
)

type Message struct {
	id FileId
	t  MessageType
	d  []byte
}

func NewMessage(id FileId, t MessageType) *Message {
	return &Message{
		id: id,
		t: t,
	}
}

func (msg *Message) encode(data interface{}) Message {
	enc := gob.NewEncoder(bytes.NewBuffer(msg.d))
	enc.Encode(data)
	return *msg
}

func (msg *Message) decode(data interface{}) {
	enc := gob.NewDecoder(bytes.NewBuffer(msg.d))
	enc.Decode(data)
}

type DataTimeModified struct {
	filename string
	modified time.Time
}

type DataFileVerification struct {
	hash FileVerificationHash
}

type DataDedupeHash struct {
	hash FileDedupeHash
}

type DataFileChunk struct {
	filesize int64
	offset   int64
	chunk    []byte
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

type BinaryState struct {
	filesize int64
	offset   int64
}

type ClientFileState struct {
	state       ClientState
	filename    string
	id          FileId
	binaryState BinaryState
	out         chan<- Message
}

func NewClientFileState(filename string, out chan<- Message) (cfs ClientFileState, err error) {
	// Assert file exists

	cfs = ClientFileState{
		state:       ClientStateInit,
		filename:    filename,
		id:          getFileId(filename),
		binaryState: BinaryState{},
		out:         out,
	}

	cfs.sendTimeModified()
	cfs.state = ClientStateCheckingStatus

	return cfs, nil
}

func (cfs *ClientFileState) handleMessage(msg Message, out chan<- Message) error {
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

func (cfs *ClientFileState) sendTimeModified() {
	info, err := os.Stat(cfs.filename)
	if err != nil {
		// TODO: Handle error
		log.Fatal(err)
	}

	cfs.binaryState.filesize = info.Size()

	msg := NewMessage(cfs.id, MessageTimeModified)
	msg.encode(DataTimeModified{
		filename: cfs.filename,
		modified: info.ModTime(),
	})
	cfs.out <- *msg
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
		hash: hash,
	})
	cfs.out <- *msg
}

func (cfs *ClientFileState) sendFileChunk() {
	file, err := os.Open(cfs.filename) // For read access.
	if err != nil {
		log.Fatal(err)
	}
	data := make([]byte, ChunkSize)
	count, err := file.Read(data)
	if err != nil {
		log.Fatal(err)
	}

	msg := NewMessage(cfs.id, MessageFileChunk)
	msg.encode(DataFileChunk{
		filesize: cfs.binaryState.filesize,
		offset:   cfs.binaryState.offset,
		chunk:    data,
	})
	cfs.out <- *msg

	cfs.binaryState.offset += int64(count)
}

// Client state
type ServerState int

type ServerStore interface {
	hasFile(FileId) bool
	getVerification(FileId) DataFileVerification
	isExpired(FileId) bool
	hasDedupeHash(FileDedupeHash) bool
	hasVerificationHash(FileVerificationHash) bool
	storeBinary([]byte)
}

const (
	ServerStateInit               ServerState = iota // Sent file, waiting for end or hash
	ServerStateCheckingDedupeHash                    // Awaiting file hash from client
	ServerStateCheckingVerificationHash                     // Awaiting file hash from client
	ServerStateGettingBinary                         // Awaiting binary from client
	ServerStateEndLink                               // Link file to existing file
	ServerStateEndBinary                             // Write new binary data
)

type ServerFileState struct {
	state       ServerState
	filename    string
	id          FileId
	binaryState BinaryState
	out         chan<- Message // TODO: Factor channel and store out
	store       ServerStore
}

func NewServerFileState(filename string, store ServerStore, out chan<- Message) (sfs *ServerFileState, err error) {

	sfs = &ServerFileState{
		state:       ServerStateInit,
		filename:    filename,
		id:          getFileId(filename),
		binaryState: BinaryState{},
		store:       store,
		out:         out,
	}

	return sfs, nil
}

func (sfs *ServerFileState) handleMessage(msg Message, out chan<- Message) error {
	if sfs.state == ServerStateInit {
		if msg.t == MessageTimeModified {
			if !sfs.store.hasFile(msg.id) {
				sfs.sendFileMissing()
				sfs.state = ServerStateCheckingDedupeHash
			} else if sfs.store.isExpired(msg.id) {
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
			if sfs.store.hasDedupeHash(dedupeData.hash) {
				sfs.sendFileOK()
				sfs.state = ServerStateEndLink
			} else {
				sfs.sendRequestBinary()
				sfs.state = ServerStateGettingBinary
			}
			return nil
		}
	} else if sfs.state == ServerStateCheckingVerificationHash {
		verificationData := DataFileVerification{}
		msg.decode(verificationData)
		if sfs.store.hasVerificationHash(verificationData.hash) {
			sfs.sendFileOK()
			sfs.state = ServerStateEndLink
		} else {
			sfs.sendRequestBinary()
			sfs.state = ServerStateGettingBinary
		}
	} else if sfs.state == ServerStateGettingBinary {
		dataChunk := DataFileChunk{}
		msg.decode(&dataChunk)
		sfs.store.storeBinary(dataChunk.chunk)
		if dataChunk.filesize == dataChunk.offset + int64(len(dataChunk.chunk)) {
			sfs.sendFileOK()
		} else {
			sfs.sendRequestBinary()
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
	sfs.out <- *NewMessage(sfs.id, MessageFileOK)
}

func (sfs *ServerFileState) sendFileVerification() {
	// TODO: Factor getExpired out
	msg := *NewMessage(sfs.id, MessageFileVerification)
	sfs.out <- msg
}

func (sfs *ServerFileState) sendFileMissing() {
	sfs.out <- *NewMessage(sfs.id, MessageFileMissing)
}

func (sfs *ServerFileState) sendRequestBinary() {
	sfs.out <- *NewMessage(sfs.id, MessageRequestBinary)
}
