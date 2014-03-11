package goboxer

import (
	"fmt"
	"os/exec"
)

// a Repo is a link to a repository to be cloned into a box
type Repo struct {
	Url string
	Scm uint
}

// possible SCM values, for now we only support Git and Mercurial
const (
	ScmGit uint = iota
	ScmMercurial
	ScmUnsupported
)

// identifyScm populates the Scm field of given repo
func (r *Repo) identifyScm() {
	switch {
	case r.isGit(): // given relative SCM popularity, we always try git first
		r.Scm = ScmGit
	case r.isMercurial():
		r.Scm = ScmMercurial
	default:
		r.Scm = ScmUnsupported
	}
	return
}

func (r *Repo) isGit() bool {
	return exec.Command("git", "ls-remote", r.Url).Run() == nil
}

func (r *Repo) isMercurial() bool {
	return exec.Command("hg", "identify", r.Url).Run() == nil
}
