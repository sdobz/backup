// sift
// Copyright (C) 2014-2016 Sven Taute
// Slimmed by Vincent Khougaz 2016
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, version 3 of the License.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

/*
Package gitignore provides a Checker that can be used to determine
whether a specific file is excluded by a .gitignore file.
This package targets to support the full gitignore pattern syntax
documented here: https://git-scm.com/docs/gitignore
Multiple .gitignore files with multiple matching patterns
are supported. A cache is used to prevent loading the same
.gitignore file again when checking different paths.
*/
package main

import (
	"bufio"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type GitIgnore struct {
	basePath string
	patterns []patternMatcher
}

// patternMatcher is an interface for all pattern types.
type patternMatcher interface {
	Matches(string, bool) bool
	Negated() bool
}

var _ patternMatcher = (*simplePattern)(nil)
var _ patternMatcher = (*filePattern)(nil)
var _ patternMatcher = (*pathPattern)(nil)
var _ patternMatcher = (*regexPattern)(nil)

// basePattern holds the properties of a gitignore pattern.
type basePattern struct {
	// the base path of the corresponding .gitignore file
	basePath string
	// only match directories (pattern ends with "/")
	matchDirOnly bool
	// include (pattern starts with "!")
	negated bool
	// match against root of basePath (pattern starts with "/")
	leadingSlash bool
	// the normalized content of the pattern
	content string
}

// simplePattern describes a pattern matching filenames.
// The pattern must not contain special characters.
type simplePattern struct {
	basePattern
}

// filePattern describes a pattern with special characters matching filenames.
// The pattern may contain special characters.
type filePattern struct {
	basePattern
}

// pathPattern describes a pattern matching a partial or complete file path.
type pathPattern struct {
	basePattern
	// depth describes the depth (number of folders) of the pattern
	depth int
}

// regexPattern describes a regex pattern matching the file path.
type regexPattern struct {
	basePattern
	re *regexp.Regexp
}

func NewGitIgnore(basePath string, reader io.Reader) (*GitIgnore, error) {
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}
	gitIgnore := &GitIgnore{basePath: absPath}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		gitIgnore.addPattern(scanner.Text(), absPath)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return gitIgnore, nil
}

// check checks whether the given path is excluded by the gitIgnore instance.
func (gi GitIgnore) Match(path string, isDir bool) bool {
	fullpath, _ := filepath.Abs(path)
	if len(fullpath) <= len(gi.basePath) || !strings.HasPrefix(fullpath, gi.basePath) {
		return false
	}

	if strings.HasSuffix(path, "findthis.log") {
		_ = "breakpoint"
	}
	testpath := fullpath[len(gi.basePath)+1:]
	for i := len(gi.patterns) - 1; i >= 0; i-- {
		p := gi.patterns[i]
		if strings.HasSuffix(path, "findthis.log") {
			_ = "breakpoint"
		}
		if p.Matches(testpath, isDir) {
			return !p.Negated()
		}
	}
	return false
}

// addPattern parses the given pattern and adds it to
// the gitIgnore instance.
func (c *GitIgnore) addPattern(pattern string, basePath string) {
	negated := false
	matchDirOnly := false
	leadingSlash := false

	if strings.Trim(pattern, " ") == "" {
		return
	}
	if strings.HasPrefix(pattern, "#") {
		return
	}

	if strings.HasPrefix(pattern, "!") {
		negated = true
		pattern = pattern[1:]
	} else if strings.HasPrefix(pattern, `\!`) {
		pattern = pattern[1:]
	}
	if strings.HasPrefix(pattern, "/") {
		leadingSlash = true
		pattern = pattern[1:]
	}
	if strings.HasSuffix(pattern, "/") {
		matchDirOnly = true
		pattern = pattern[:len(pattern)-1]
	}

	var p patternMatcher
	var base basePattern
	base = basePattern{
		basePath:     basePath,
		content:      pattern,
		negated:      negated,
		leadingSlash: leadingSlash,
		matchDirOnly: matchDirOnly,
	}
	if strings.Contains(pattern, "**") {
		p = newRegexPattern(base)
	} else {
		if strings.Contains(pattern, "/") || leadingSlash {
			p = newPathPattern(base)
		} else {
			if strings.ContainsAny(pattern, "*?[") {
				p = newFilePattern(base)
			} else {
				p = newSimplePattern(base)
			}
		}
	}
	c.patterns = append(c.patterns, p)
}

func (p basePattern) Negated() bool {
	return p.negated
}

func newSimplePattern(base basePattern) patternMatcher {
	return simplePattern{base}
}

func (p simplePattern) Matches(filePath string, isDir bool) bool {
	if p.matchDirOnly && !isDir {
		return false
	}
	filename := path.Base(filePath)
	return filename == p.content
}

func newFilePattern(base basePattern) patternMatcher {
	return filePattern{base}
}

func (p filePattern) Matches(filePath string, isDir bool) bool {
	if p.matchDirOnly && !isDir {
		return false
	}
	filename := path.Base(filePath)
	res, err := filepath.Match(p.content, filename)
	if err != nil {
		return false
	}
	return res
}

func newPathPattern(base basePattern) patternMatcher {
	depth := 0
	if !base.leadingSlash {
		depth = strings.Count(base.content, "/")
	}
	p := pathPattern{base, depth}
	return p
}

func (p pathPattern) Matches(path string, isDir bool) bool {
	if p.matchDirOnly && !isDir {
		return false
	}
	if runtime.GOOS == "windows" {
		path = filepath.ToSlash(path)
	}
	if p.leadingSlash {
		res, err := filepath.Match(p.content, path)
		if err != nil {
			return false
		}
		return res
	} else {
		slashes := 0
		pos := 0
		for pos = len(path) - 1; pos >= 0; pos-- {
			if path[pos:pos+1] == "/" {
				slashes++
				if slashes > p.depth {
					break
				}
			}
		}
		if slashes < p.depth {
			return false
		}
		checkpath := path[pos+1:]
		res, err := filepath.Match(p.content, checkpath)
		if err != nil {
			return false
		}
		return res
	}
}

func newRegexPattern(base basePattern) patternMatcher {
	matchStart := false
	matchEnd := false
	content := base.content
	if strings.HasPrefix(content, "**/") {
		content = content[3:]
	} else {
		matchStart = true
	}
	if strings.HasSuffix(content, "/**") {
		content = content[:len(content)-3]
	} else {
		matchEnd = true
	}

	parts := strings.Split(content, "**")
	for i, _ := range parts {
		parts[i] = regexp.QuoteMeta(parts[i])
	}
	pattern := strings.Join(parts, ".*?")
	if matchStart {
		pattern = "^" + pattern
	}
	if matchEnd {
		pattern = pattern + "$"
	}

	re := regexp.MustCompile(pattern)
	p := regexPattern{base, re}
	return p
}

func (p regexPattern) Matches(path string, isDir bool) bool {
	if p.matchDirOnly && !isDir {
		return false
	}
	if runtime.GOOS == "windows" {
		path = filepath.ToSlash(path)
	}
	return p.re.MatchString(path)
}
