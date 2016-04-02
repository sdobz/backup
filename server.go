package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"os"
)

type BackupServerConfig struct {
	configFile string
}

type ServerConfig struct {
	FilePath string `json:"file_path"`
	Port     int    `json:"port"`
}

type Server struct {
	config     ServerConfig
	clientInfo ClientInfo
	storage    *Storage
	network    NetworkInterface
}

var _ ServerInterface = (*Server)(nil)

func NewServer(config ServerConfig, network NetworkInterface) (server *Server, err error) {
	storage, err := NewStorage(config.FilePath)
	if err != nil {
		return nil, err
	}

	return &Server{
		config:  config,
		storage: storage,
		network: network,
	}, nil
}

func (server *Server) metaForFilename(filename string) *FileMeta {
	return &FileMeta{
		Identity:   server.clientInfo.Identity,
		BackupName: server.clientInfo.BackupName,
		Session:    server.clientInfo.Session,
		FileName:   filename,
	}
}

func (server *Server) GetVerification(filename string) FileVerificationHash {
	return FileVerificationHash{}
}

func (server *Server) SetClientInfo(clientInfo ClientInfo) {
	server.clientInfo = clientInfo
}

func (server *Server) WriteChunk(filename string, chunk []byte) error {
	log.Printf("Got %v bytes", len(chunk))
	return server.storage.WriteChunk(server.metaForFilename(filename), chunk)
}

func (server *Server) LinkExisting(filename string) (bool, error) {
	log.Printf("Storing existing")
	return server.storage.LinkFromOtherSession(server.metaForFilename(filename))
}

func (server *Server) LinkDedupe(filename string, dedupe FileDedupeHash) (bool, error) {
	log.Printf("Storing link")
	return server.storage.LinkDedupe(dedupe, server.metaForFilename(filename))
}

func (server *Server) Send(msg *Message) {
	log.Printf("Server sending: %v", msg)
	server.network.send(msg)
}

// Get session start request
// Form backup location, get storage object

func parseServerConfig(file io.Reader) (serverConfig ServerConfig, err error) {
	serverConfig = ServerConfig{}
	contents, err := ioutil.ReadAll(file)
	if err != nil {
		return ServerConfig{}, err
	}
	err = json.Unmarshal(contents, &serverConfig)
	if err != nil {
		return ServerConfig{}, err
	}
	return serverConfig, nil
}

func ServeBackup(configFile string, network NetworkInterface) <-chan error {
	errc := make(chan error)

	log.Printf("Serving with %v", configFile)
	file, err := os.Open(configFile)
	if err != nil {
		errc <- err
		defer close(errc)
	}

	serverConfig, err := parseServerConfig(file)
	if err != nil {
		errc <- err
		defer close(errc)
	}

	server, err := NewServer(serverConfig, network)
	if err != nil {
		errc <- err
		defer close(errc)
	}
	ss := NewServerState(server)

	go func() {
		// Never terminates
		for {
			msg := network.getMessage()
			go func() {
				err := ss.handleMessage(&msg)
				if err != nil {
					errc <- err
				}
			}()
		}
		close(errc)
	}()

	return errc
}
