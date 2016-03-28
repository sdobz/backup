package main

import (
	"errors"
	"github.com/kalafut/imohash"
	"github.com/monochromegane/go-gitignore"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"time"
)

type ClientConfig struct {
	specFile string
}

type Client struct {
	gitignore   gitignore.IgnoreMatcher
	root        string
	network     NetworkInterface
	fileHandles map[string]*os.File
}

// Verify Client implements ClientInterface
var _ ClientInterface = (*Client)(nil)

func NewClient(specFile string, network NetworkInterface) (*Client, error) {
	if _, err := os.Open(specFile); os.IsNotExist(err) {
		return nil, errors.New("Client config does not exist")
	}

	ignorer, err := gitignore.NewGitIgnore(specFile)
	if err != nil {
		return nil, err
	}

	return &Client{
		gitignore:   ignorer,
		root:        path.Dir(specFile),
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

func (client *Client) Enumerate() <-chan string {
	ch := make(chan string, 100)
	// log.Print("Enumerate...")
	go func() {
		// For every glob
		filepath.Walk(client.root, filepath.WalkFunc(func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if client.gitignore.Match(path, false) {
				ch <- path
			}
			return nil
		}))
		close(ch)
	}()
	return ch
}

func PerformBackup(specFile string, network NetworkInterface) error {
	client, err := NewClient(specFile, network)
	if err != nil {
		return err
	}
	cs := NewClientState(client)

	err = cs.requestSession()
	if err != nil {
		return err
	}

	errc := make(chan error)

	for {
		msg := network.getMessage()
		go func() {
			log.Printf("Client got: %v", msg)
			err := cs.handleMessage(&msg)
			if err != nil {
				errc <- err
			}
		}()
	}

	return <-errc
}
