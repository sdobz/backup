package main

import (
	"encoding/json"
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
	DbPath   string `json:"db_path"`
	Port     int    `json:"port"`
}

type Server struct {
	config  ServerConfig
	db      *DB
	network NetworkInterface
}

var _ ServerInterface = (*Server)(nil)

func NewServer(config ServerConfig, network NetworkInterface) (server *Server, err error) {
	db, err := NewDB(config.DbPath)
	if err != nil {
		return nil, err
	}

	return &Server{
		config:  config,
		db:      db,
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
	return server.db.getVerification(id)
}

func (server *Server) HasDedupeHash(hash FileDedupeHash) bool {
	return server.db.hasDedupeHash(hash)
}

func (server *Server) HasVerificationHash(hash FileVerificationHash) bool {
	return server.db.hasVerificationHash(hash)
}

func (server *Server) StoreBinary(filename string, chunk []byte) {
	log.Printf("Got %v bytes", len(chunk))
}

func (server *Server) Send(msg *Message) {
	server.network.send(msg)
}

func (server *Server) NewSession() Session {
	return NewSession()
}

// Get session start request
// Form backup location, get storage object

func parseServerConfig(file []byte) (serverConfig ServerConfig, err error) {
	serverConfig = ServerConfig{}
	err = json.Unmarshal(file, &serverConfig)
	if err != nil {
		return ServerConfig{}, err
	}
	return serverConfig, nil
}

func ServeBackup(config BackupServerConfig, network NetworkInterface) {
	configFile := ""
	if config.configFile == "" {
		configFile = ".backup-server"
	} else {
		configFile = config.configFile
	}

	configFile = expandTilde(configFile)

	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Panic(err)
	}

	serverConfig, err := parseServerConfig(file)
	if err != nil {
		log.Panic(err)
	}

	serverConfig.FilePath = expandTilde(serverConfig.FilePath)
	serverConfig.DbPath = expandTilde(serverConfig.DbPath)

	server, err := NewServer(serverConfig, network)
	if err != nil {
		log.Panic(err)
	}
	serverState := NewServerState(server)

	for {
		msg := network.getMessage()
		serverState.handleMessage(&msg)
	}
}
