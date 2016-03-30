package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type BackupServerConfig struct {
	configFile string
}

type ServerConfig struct {
	FilePath string `json:"file_path"`
	Port     int    `json:"port"`
}

type Server struct {
	config  ServerConfig
	storage *Storage
	network NetworkInterface
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

func (server *Server) absolutePath(filename string) string {
	return filepath.Join(server.config.FilePath, filename)
}

func (server *Server) HasFile(filename string) bool {
	// TODO: compare modification time
	_, err := os.Stat(server.absolutePath(filename))
	return err == nil
}

func (server *Server) IsExpired(filename string) bool {
	return false
}

func (server *Server) GetVerification(id FileId) FileVerificationHash {
	return server.storage.GetVerification(id)
}

func (server *Server) HasDedupeHash(hash FileDedupeHash) bool {
	return server.storage.HasDedupeHash(hash)
}

func (server *Server) HasVerificationHash(hash FileVerificationHash) bool {
	return server.storage.HasVerificationHash(hash)
}

func (server *Server) StoreBinary(filename string, chunk []byte) {
	server.storage.StoreBinary(filename, chunk)
	log.Printf("Got %v bytes", len(chunk))
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
