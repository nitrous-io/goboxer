package goboxer

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/action-io/goboxer/utils"
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

const CloneTimeout = 300 // seconds

func (r *Repo) Clone(baseLocation string) error {
	if r.Scm == 0 {
		r.identifyScm()
	}
	if r.Scm == ScmUnsupported {
		return fmt.Errorf("SCM for %s cannot be determined", r.Url)
	}

	destination, err := r.freshLocation(baseLocation)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd

	switch {
	case r.Scm == ScmGit:
		cmd = exec.Command("git", "clone", r.Url, destination)
	case r.Scm == ScmMercurial:
		cmd = exec.Command("hg", "clone", r.Url, destination)
	default:
		panic("Switch not exhaustive")
	}

	// TODO differentiate between clone timeout due to connection timeout (no
	// output) vs timeout due to download taking too long.  With typical
	// net.ipv4.tcp_syn_retries values this can be 2-3 mins.
	return utils.Exec(cmd, CloneTimeout*time.Second)
}

// freshLocation appends numbers to the default directory name for a clone
// until it finds a non-existent directory.  It gives up after 5 attempts (if
// '<name>4' does not exist.)
func (r *Repo) freshLocation(baseLocation string) (string, error) {
	validDir := func(dirPath string) bool {
		if _, err := os.Stat(dirPath); err == nil {
			return false
		} else {
			return os.IsNotExist(err)
		}
	}
	parts := strings.Split(r.Url, "/")
	name := strings.TrimSuffix(parts[len(parts)-1], ".git")
	dir := path.Join(baseLocation, name)
	for _, suffix := range []string{"", "1", "2", "3", "4"} {
		attempt := dir + suffix
		if validDir(attempt) {
			return attempt, nil
		}
	}
	return "", fmt.Errorf("No applicable destination directory")
}

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
