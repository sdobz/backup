package main

import (
	"bufio"
	"github.com/gobwas/glob"
	"github.com/kalafut/imohash"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

type ClientConfig struct {
	specFile string
}

type BackupGlob struct {
	include  bool
	abs      bool
	root     string
	origGlob string
	glob     glob.Glob
}

type BackupSpec struct {
	globs []BackupGlob
}

type BackupStats struct {
	start        time.Time
	end          time.Time
	filesChecked uint64
	globsChecked uint64
}

func (backupStats *BackupStats) duration() time.Duration {
	return backupStats.end.Sub(backupStats.start)
}

type Client struct {
	backupStats BackupStats
	backupSpec  BackupSpec
	network     NetworkInterface
}

// Verify Client implements ClientInterface
var _ ClientInterface = (*Client)(nil)

func NewClient(backupSpec BackupSpec, network NetworkInterface) *Client {
	return &Client{
		backupStats: BackupStats{},
		backupSpec:  backupSpec,
		network:     network,
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

func (client *Client) GetFileChunk(filename string, offset int) []byte {
	// TODO: Track handles
	file, err := os.Open(filename) // For read access.
	if err != nil {
		log.Fatal(err)
	}
	data := make([]byte, ChunkSize)
	count, err := file.Read(data)
	if err != io.EOF && err != nil {
		log.Fatal(err)
	}

	return data[:count]
}

func (client *Client) Enumerate() <-chan string {
	client.backupStats.start = time.Now()

	ch := make(chan string, 100)
	// log.Print("Enumerate...")
	go func() {
		// For every glob
		for _, glob := range client.backupSpec.globs {
			if glob.include && glob.root != "" {
				// log.Print("Walk: ", glob.root)
				// Walk over any absolute included paths
				filepath.Walk(glob.root, filepath.WalkFunc(func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}

					client.backupStats.filesChecked += 1

					if info.IsDir() {
						return nil
					}

					// log.Print("  Candidate: ", path)

					// Check the matched path against all globs
					include := false
					for _, elimGlob := range client.backupSpec.globs {
						// If it matches a glob then mark the file as included or excluded depending on the glob
						// If the glob include matches the current include state then it cannot change the value
						//   and does not need to be evaluated
						if elimGlob.include != include {
							// log.Print("  Checking against glob ", elimGlob.origGlob, " <- ", path)
							if elimGlob.glob.Match(path) {
								client.backupStats.globsChecked += 1
								include = elimGlob.include
								// log.Print("    Match! Include: ", include)
							}
						}
					}
					if include {
						ch <- path
					}
					return nil
				}))
			}
		}
		close(ch)
		client.backupStats.end = time.Now()
	}()
	return ch
}

func parseBackupSpec(specReader io.Reader) (spec BackupSpec, err error) {
	scanner := bufio.NewScanner(specReader)
	stringSep := string(os.PathSeparator)

	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return spec, err
		}
		line := scanner.Text()
		line = strings.TrimSpace(line)
		include := true

		// # marks comment
		if string(line[:1]) == "#" {
			continue
		}
		// Nothing or + marks as inclusion
		if string(line[:1]) == "+" {
			line = line[1:]
		}
		// - marks as exclusion
		if string(line[:1]) == "-" {
			include = false
			line = line[1:]
		}

		// Space is trimmed
		line = strings.TrimSpace(line)
		// ~/ is expanded
		line = expandTilde(line)

		root := ""
		if filepath.IsAbs(line) {
			// If there is a * in the path then everything to the left of it is considered the root and walks will start there
			root = strings.SplitN(line, "*", 2)[0]
		}

		// If the path ends in / match everything under it
		if line[len(line)-1:] == stringSep {
			line = line + "**"
		}

		glob := BackupGlob{
			include:  include,
			root:     root,
			glob:     glob.MustCompile(line, os.PathSeparator),
			origGlob: line,
		}

		spec.globs = append(spec.globs, glob)
	}

	return spec, nil
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

	file, err := os.Open(specFile)
	if err != nil {
		log.Panic(err)
	}

	backupSpec, err := parseBackupSpec(file)
	if err != nil {
		log.Panic(err)
	}

	client := NewClient(backupSpec, network)
	cs := NewClientState(client)

	go func() {
		for {
			msg := network.getMessage()
			log.Printf("Client got: %v", msg)
			cs.handleMessage(&msg)
		}
	}()

	log.Print("Took: ", client.backupStats.duration().Seconds(), "s")
}
