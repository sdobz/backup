package gitignore

import "io"

type GitIgnore struct {}

func NewGitIgnore(base string, reader io.Reader) (*GitIgnore, error) {
	return &GitIgnore{}, nil
}


func (gi *GitIgnore) Match(path string, isDir bool) bool {
	return false;
}
