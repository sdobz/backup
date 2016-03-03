package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"path/filepath"
	"os"
)

type BackupServerConfig struct {
	configFile string
}

type ServerConfig struct {
	FilePath string `json:"file_path"`
	DbPath   string `json:"db_path"`
	Port int `json:"port"`
}

type Server struct {
	config ServerConfig
	db     *DB
}

func NewServer(config ServerConfig) (server *Server, err error) {
	db, err := NewDB(config.DbPath)
	if err != nil {
		return nil, err
	}

	return &Server{
		config: config,
		db: db,
	}, nil
}

func (server *Server) absolutePath(filename string) string {
	return filepath.Join(server.config.FilePath, filename)
}

func (server *Server) HasFile(filename string) bool {
	_, err := os.Stat(server.absolutePath(filename))
	return err == nil
}

func (server *Server) IsExpired(filename string) bool {
	return false
}

func (server *Server) GetVerification(id FileId) DataFileVerification {
	return server.db.getVerification(id)
}

func (server *Server) HasDedupeHash(hash FileDedupeHash) bool {
	return server.db.hasDedupeHash(hash)
}

func (server *Server) HasVerificationHash(hash FileVerificationHash) bool {
	return server.db.hasVerificationHash(hash)
}

func (server *Server) StoreBinary(chunk []byte) {
	log.Printf("Got %v bytes", len(chunk))
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

	server, err := NewServer(serverConfig)
	if err != nil {
		log.Panic(err)
	}

	fileStates := make(map[FileId]*ServerFileState)
	for {
		msg := network.getMessage()
		log.Printf("Server got: %v", msg)
		if msg.t == MessageStartFile {
			fileData := DataStartFile{}
			msg.decode(&fileData)
			sfs, err := NewServerFileState(fileData, server, network)
			if err != nil {
				log.Fatal(err)
			}

			if sfs.id != msg.id {
				log.Fatal("ServerFileState and message id disagree")
			}

			fileStates[sfs.id] = sfs
		}

		if sfs, ok := fileStates[msg.id]; ok {
			go func() {
				sfs.handleMessage(msg)
				log.Print(sfs)
			}()
		}
	}
}
