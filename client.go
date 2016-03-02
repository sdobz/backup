package main

import (
	"bufio"
	"github.com/gobwas/glob"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

type BackupClientConfig struct {
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

type BackupFile struct {
	path string
	err  error
}

type BackupStats struct {
	start time.Time
	end time.Time
	filesChecked uint64
	globsChecked uint64
}

func (backupStats *BackupStats) duration() time.Duration {
	return backupStats.end.Sub(backupStats.start)
}

func (spec *BackupSpec) enumerate(backupStats *BackupStats) <-chan BackupFile {
	backupStats.start = time.Now()

	ch := make(chan BackupFile, 100)
	// log.Print("Enumerate...")
	go func() {
		// For every glob
		for _, glob := range spec.globs {
			if glob.include && glob.root != "" {
				// log.Print("Walk: ", glob.root)
				// Walk over any absolute included paths
				filepath.Walk(glob.root, filepath.WalkFunc(func(path string, info os.FileInfo, err error) error {
					backupStats.filesChecked += 1

					if err != nil {
						ch <- BackupFile{"", err}
						return nil
					}

					if !!info.IsDir() {
						return nil
					}

					// log.Print("  Candidate: ", path)

					// Check the matched path against all globs
					include := false
					for _, elimGlob := range spec.globs {
						// If it matches a glob then mark the file as included or excluded depending on the glob
						// If the glob include matches the current include state then it cannot change the value
						//   and does not need to be evaluated
						if elimGlob.include != include {
							// log.Print("  Checking against glob ", elimGlob.origGlob, " <- ", path)
							if elimGlob.glob.Match(path) {
								backupStats.globsChecked += 1
								include = elimGlob.include
								// log.Print("    Match! Include: ", include)
							}
						}
					}
					if include {
						ch <- BackupFile{path, nil}
					}
					return nil
				}))
			}
		}
		close(ch)
		backupStats.end = time.Now()
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


func enumerate(config BackupClientConfig) {
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

	backupStats := BackupStats{}
	for backupFile := range backupSpec.enumerate(&backupStats) {
		if backupFile.err != nil {
			log.Print(backupFile.err)
		} else {
			log.Print("--Got: ", backupFile.path)
		}
	}
	log.Print("Took: ", backupStats.duration().Seconds(), "s")
}
