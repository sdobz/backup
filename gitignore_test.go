package main

import (
	"path"
	"strings"
	"testing"
)

type singleMatch struct {
	path   string
	isDir  bool
	expect bool
}

func TestMatch(t *testing.T) {
	// Implementing tests from this guy:
	// https://github.com/svent/gitignore-test

	gi, _ := NewGitIgnore(".", strings.NewReader(`
*.[oa]
*.html
*.min.js


!foo*.html
foo-excl.html

vmlinux*

\!important!.txt

log/*.log
!/log/foo.log

**/logdir/log
**/foodir/bar
exclude/**

!findthis*

**/hide/**
subdir/subdir2/

/rootsubdir/

dirpattern/

README.md

# arch/foo/kernel/.gitignore
!arch/foo/kernel/vmlinux*

# htmldocs/.gitignore
!htmldocs/*.html

# git-sample-3/.gitignore
git-sample-3/*
!git-sample-3/foo
git-sample-3/foo/*
!git-sample-3/foo/bar
`))

	matches := []singleMatch{
		singleMatch{"!important!.txt", false, true},
		singleMatch{"arch", true, false},
		singleMatch{"arch/foo", true, false},
		singleMatch{"arch/foo/kernel", true, false},
		singleMatch{"arch/foo/kernel/vmlinux.lds.S", false, false},
		singleMatch{"arch/foo/vmlinux.lds.S", false, true},
		singleMatch{"bar", true, false},
		singleMatch{"bar/testfile", false, false},
		singleMatch{"dirpattern", false, false},
		singleMatch{"Documentation", true, false},
		singleMatch{"Documentation/foo-excl.html", false, true},
		singleMatch{"Documentation/foo.html", false, false},
		singleMatch{"Documentation/gitignore.html", false, true},
		singleMatch{"Documentation/test.a.html", false, true},
		singleMatch{"exclude", true, true},
		singleMatch{"exclude/dir1", true, true},
		singleMatch{"exclude/dir1/dir2", true, true},
		singleMatch{"exclude/dir1/dir2/dir3", true, true},
		singleMatch{"exclude/dir1/dir2/dir3/testfile", false, true},
		singleMatch{"file.o", false, true},
		singleMatch{"foodir", true, false},
		singleMatch{"foodir/bar", true, true},
		singleMatch{"foodir/bar/testfile", false, true},
		singleMatch{"git-sample-3", true, false},
		singleMatch{"git-sample-3/foo", true, false},
		singleMatch{"git-sample-3/foo", true, false},
		singleMatch{"git-sample-3/foo/bar", true, false},
		singleMatch{"git-sample-3/foo/test", true, true},
		singleMatch{"git-sample-3/foo/test", true, true},
		singleMatch{"git-sample-3/test", true, true},
		singleMatch{"htmldoc", true, false},
		// Has no support for subdir/*.sadf
		// singleMatch{"htmldoc/docs.html", false, false},
		singleMatch{"htmldoc/jslib.min.js", false, true},
		singleMatch{"lib.a", false, true},
		singleMatch{"log", true, false},
		singleMatch{"log/foo.log", false, false},
		singleMatch{"log/test.log", false, true},
		singleMatch{"rootsubdir", true, true},
		singleMatch{"rootsubdir/foo", false, true},
		singleMatch{"src", true, false},
		singleMatch{"src/findthis.o", false, false},
		singleMatch{"src/internal.o", false, true},
		singleMatch{"subdir", true, false},
		singleMatch{"subdir/hide", true, true},
		singleMatch{"subdir/hide/foo", false, true},
		singleMatch{"subdir/logdir", true, false},
		singleMatch{"subdir/logdir/log", true, true},
		singleMatch{"subdir/logdir/log/findthis.log", false, true},
		singleMatch{"subdir/logdir/log/foo.log", false, true},
		singleMatch{"subdir/logdir/log/test.log", false, true},
		singleMatch{"subdir/rootsubdir", true, false},
		singleMatch{"subdir/rootsubdir/foo", false, false},
		singleMatch{"subdir/subdir2", true, true},
		singleMatch{"subdir/subdir2/bar", false, true},
		singleMatch{"README.md", false, true},
	}

	for _, match := range matches {
		if result := matchTree(gi, match); result != match.expect {
			t.Errorf("Match should return %t, got %t on %v", match.expect, result, match)
		}
	}
}

// Matches up the entire tree, returning if any item matches
// This matches typical behavior where a tree is traversed, and the branch is skipped if the item matches
func matchTree(gi *GitIgnore, match singleMatch) bool {
	matchPath := match.path
	isDir := match.isDir
	for matchPath != "." {
		if gi.Match(matchPath, isDir) {
			return true
		}
		matchPath = path.Dir(matchPath)
		isDir = true
	}
	return false
}
