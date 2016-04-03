package main

import (
	"crypto/md5"
	"encoding/hex"
	"github.com/kalafut/imohash"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"github.com/sdobz/backup/gitignore"
)

type ClientConfig struct {
	spec string
	root string
	name string
}

type Client struct {
	gitignore   *gitignore.GitIgnore
	root        string
	name        string
	network     NetworkInterface
	fileHandles map[string]*os.File
}

// Verify Client implements ClientInterface
var _ ClientInterface = (*Client)(nil)

func NewClient(config ClientConfig, network NetworkInterface) (*Client, error) {
	ignorer, err := gitignore.NewGitIgnore(config.root, strings.NewReader(config.spec))
	if err != nil {
		return nil, err
	}

	return &Client{
		gitignore:   ignorer,
		root:        config.root,
		name:        config.name,
		network:     network,
		fileHandles: make(map[string]*os.File),
	}, nil
}

func (client *Client) Send(msg *Message) {
	log.Printf("Client sending: %v", msg)
	client.network.send(msg)
}

type PartialFileInfo interface {
	Size() int64
	ModTime() time.Time
}

func (client *Client) GetFileInfo(filename string) PartialFileInfo {
	info, err := os.Stat(filename)
	if err != nil {
		// TODO: Handle error
		log.Print(err)
	}

	return info
}

func (client *Client) GetDedupeHash(filename string) FileDedupeHash {
	var hash FileDedupeHash
	hash, err := imohash.SumFile(filename)
	if err != nil {
		// TODO: Handle error
		log.Print(err)
	}

	return hash
}

func (client *Client) GetFileChunk(filename string, size int64, offset int64) []byte {
	data := make([]byte, size)
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	file.Seek(offset, os.SEEK_SET)
	count, err := file.Read(data)
	if err != io.EOF && err != nil {
		// TODO: error handling
		log.Fatal(err)
	}

	return data[:count]
}

func (client *Client) Enumerate() <-chan string {
	ch := make(chan string)
	log.Print("Enumerate...")
	go func() {
		// For every glob
		log.Print("In Enumerate gofunc, walking %v", client.root)
		filepath.Walk(client.root, filepath.WalkFunc(func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if !client.gitignore.Match(path, false) {
				log.Printf("Path not ignored: %v", path)
				ch <- path
			}
			return nil
		}))
		close(ch)
	}()
	return ch
}

func (client *Client) GetClientInfo() (ClientInfo, error) {
	identity, err := client.getIdentity()
	if err != nil {
		return ClientInfo{}, nil
	}

	return ClientInfo{
		Identity:   identity,
		BackupName: client.name,
		Session:    client.getSession(),
	}, nil
}

func (client *Client) getIdentity() (string, error) {
	// TODO: Base on private key
	hasher := md5.New()

	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, interf := range interfaces {
		if len(interf.HardwareAddr) > 0 {
			hasher.Write(interf.HardwareAddr)
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (client *Client) getSession() string {
	return time.Now().Format("2006-01-02")
}

func PerformBackup(specFile string, network NetworkInterface) <-chan error {
	errc := make(chan error)

	file, err := os.Open(specFile)
	if err != nil {
		errc <- err
		defer close(errc)
		return errc
	}

	specBytes, err := ioutil.ReadAll(file)

	client, err := NewClient(ClientConfig{
		spec: string(specBytes),
		root: path.Dir(specFile),
		name: path.Base(specFile),
	}, network)
	if err != nil {
		errc <- err
		defer close(errc)
		return errc
	}

	cs := NewClientState(client)

	err = cs.requestSession()
	if err != nil {
		errc <- err
		defer close(errc)
		return errc
	}

	go func() {
		// if handleMessage async updates to Done then this will never terminate
		for cs.state != ClientStateDone {
			msg := network.getMessage()
			go func() {
				err := cs.handleMessage(&msg)
				if err != nil {
					errc <- err
				}
			}()
		}
		close(errc)
	}()

	return errc
}
