package gitignore

import (
	"bufio"
	"bytes"
	"github.com/gobwas/glob"
	"io"
	"os"
)

type GitIgnore struct {
	patterns []pattern
}

type pattern interface {
	Match(path string, isDir bool) bool
	Inverted() bool
}

type globPattern struct {
	invert  bool
	dirOnly bool
	orig    string
	glob    glob.Glob
}

func (gp *globPattern) String() string {
	return gp.orig
}

func (gp *globPattern) Inverted() bool {
	return gp.invert
}

func (gp *globPattern) Match(path string, isDir bool) bool {
	if gp.dirOnly && !isDir {
		return false
	}
	return gp.glob.Match(path)
}

func trimTrailingSpace(in []byte) []byte {
	// TODO: handle escaped last spaces
	return bytes.Trim(in, " ")
}

func NewGitIgnore(base string, reader io.Reader) (*GitIgnore, error) {
	scn := bufio.NewScanner(reader)
	gi := &GitIgnore{}
	for scn.Scan() {
		line := scn.Bytes()
		line = trimTrailingSpace(line)
		if len(line) == 0 {
			continue
		}
		// TODO: Escape \# to #
		if line[0] == '#' {
			continue
		}

		gp := &globPattern{
			orig: string(line),
		}

		// TODO: Escape \! to !
		if line[0] == '!' {
			// Strip first char
			line = line[1:]
			gp.invert = true
		}

		if line[len(line)-1] == '/' {
			// Strip trailing slash
			line = line[0 : len(line)-1]
			gp.dirOnly = true
		}

		gp.glob = glob.MustCompile(string(line), os.PathSeparator)
		gi.patterns = append(gi.patterns, gp)
	}
	return gi, nil
}

func (gi *GitIgnore) Match(path string, isDir bool) bool {
	for i := len(gi.patterns) - 1; i > 0; i-- {
		gp := gi.patterns[i]
		if gp.Match(path, isDir) {
			return !gp.Inverted()
		}
	}
	return false
}
