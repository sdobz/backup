package main

import (
	"github.com/kalafut/imohash"
	"github.com/monochromegane/go-gitignore"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

type ClientConfig struct {
	specFile string
}

type Enumerator struct {
	gitignore gitignore.IgnoreMatcher
	root      string
}

type Client struct {
	enumerator  Enumerator
	network     NetworkInterface
	fileHandles map[string]*os.File
}

// Verify Client implements ClientInterface
var _ ClientInterface = (*Client)(nil)

func NewClient(enumerator Enumerator, network NetworkInterface) *Client {
	return &Client{
		enumerator:  enumerator,
		network:     network,
		fileHandles: make(map[string]*os.File),
	}
}

func (client *Client) Send(msg *Message) {
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
		log.Fatal(err)
	}

	return info
}

func (client *Client) GetDedupeHash(filename string) FileDedupeHash {
	var hash FileDedupeHash
	hash, err := imohash.SumFile(filename)
	if err != nil {
		// TODO: Handle error
		log.Fatal(err)
	}

	return hash
}

func (client *Client) GetFileChunk(filename string, size int64, offset int64) []byte {
	_, ok := client.fileHandles[filename]
	if !ok {
		file, err := os.Open(filename)

		if err != nil {
			log.Fatal(err)
		}
		client.fileHandles[filename] = file
	}

	file := client.fileHandles[filename]
	data := make([]byte, size)
	file.Seek(offset, os.SEEK_SET)
	count, err := file.Read(data)
	if err != io.EOF && err != nil {
		log.Fatal(err)
	}

	return data[:count]
}

func (e *Enumerator) Enumerate() <-chan string {
	ch := make(chan string, 100)
	// log.Print("Enumerate...")
	go func() {
		// For every glob
		filepath.Walk(e.root, filepath.WalkFunc(func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if e.gitignore.Match(path, false) {
				ch <- path
			}
			return nil
		}))
		close(ch)
	}()
	return ch
}

func (client *Client) Enumerate() <-chan string{
	return client.enumerator.Enumerate()
}

func expandTilde(path string) string {
	usr, _ := user.Current()

	if path[:2] == "~/" {
		path = usr.HomeDir + string(os.PathSeparator) + path[2:]
	}

	return path
}

// Tell server to start backup session, specifying a directory
func PerformBackup(config ClientConfig, network NetworkInterface) {
	specFile := ""
	if config.specFile == "" {
		specFile = "~/.backup"
	} else {
		specFile = config.specFile
	}

	specFile = expandTilde(specFile)

	gitignore, err := gitignore.NewGitIgnore(specFile)
	if err != nil {
		log.Fatal(err)
	}

	client := NewClient(Enumerator{
		gitignore: gitignore,
		root:      filepath.Dir(specFile),
	}, network)
	cs := NewClientState(client)
	cs.requestSession()

	go func() {
		for {
			msg := network.getMessage()
			log.Printf("Client got: %v", msg)
			cs.handleMessage(&msg)
		}
	}()
}
