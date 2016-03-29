package main

import (
	"github.com/kalafut/imohash"
	"github.com/monochromegane/go-gitignore"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
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

func NewClient(spec string, root string, network NetworkInterface) *Client {
	ignorer := gitignore.NewGitIgnoreFromReader(root, strings.NewReader(spec))

	return &Client{
		gitignore:   ignorer,
		root:        root,
		network:     network,
		fileHandles: make(map[string]*os.File),
	}
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
	_, ok := client.fileHandles[filename]
	if !ok {
		file, err := os.Open(filename)

		if err != nil {
			log.Print(err)
		}
		client.fileHandles[filename] = file
	}

	file := client.fileHandles[filename]
	data := make([]byte, size)
	file.Seek(offset, os.SEEK_SET)
	count, err := file.Read(data)
	if err != io.EOF && err != nil {
		log.Print(err)
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

func PerformBackup(specFile string, network NetworkInterface) <-chan error {
	errc := make(chan error)

	file, err := os.Open(specFile)
	if err != nil {
		errc <- err
		defer close(errc)
	}

	specBytes, err := ioutil.ReadAll(file)

	client := NewClient(string(specBytes), path.Dir(specFile), network)

	cs := NewClientState(client)

	err = cs.requestSession()
	if err != nil {
		errc <- err
		defer close(errc)
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
