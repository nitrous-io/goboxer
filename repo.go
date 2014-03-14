package goboxer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// a Repo is a link to a repository to be cloned into a box
type Repo struct {
	Box             *Box
	Url             string
	Scm             uint
	CloneTimeout    time.Duration
	IdentifyTimeout time.Duration
}

// possible SCM values, for now we only support Git and Mercurial
const (
	ScmGit uint = 1 + iota
	ScmMercurial
	ScmUnsupported
)

const (
	DefaultRepoCloneTimeout    = 5 * time.Minute
	DefaultRepoIdentifyTimeout = 1 * time.Minute
)

// all returned errors are passed on to Message and are user-facing
// these are all formatted strings and intended to be passed into fmt.Errorf
var (
	DefaultCloneError      = fmt.Errorf("Error cloning repository")
	UnsupportedCloneError  = fmt.Errorf("URL does not look like a Git or Mercurial repository")
	DiskSpaceExceededError = fmt.Errorf("Repository size exceeds available disk space")
	NoDestinationError     = fmt.Errorf("Could not determine destination directory")
	CloneTimeoutError      = fmt.Errorf("Clone took too long")
)

// Clone takes a baseLocation relative to the box root and uses that as the parent directory of the cloned repository
func (r *Repo) Clone(baseLocation string) error {
	var err error
	if r.Scm == 0 {
		r.identifyScm()
	}
	var cmd *exec.Cmd

	clonePath, err := r.freshLocation(baseLocation)
	if err != nil {
		return err
	}

	switch {
	case r.Scm == ScmGit:
		cmd = r.boxUserCommand("git", "clone", r.Url, clonePath)
	case r.Scm == ScmMercurial:
		cmd = r.boxUserCommand("hg", "clone", r.Url, clonePath)
	case r.Scm == ScmUnsupported:
		return UnsupportedCloneError
	default:
		panic("Switch not exhaustive")
	}

	// TODO differentiate between clone timeout due to connection timeout (no
	// output) vs timeout due to download taking too long.  With typical
	// net.ipv4.tcp_syn_retries values the timeout for former can be 2-3 mins.
	timeout := r.CloneTimeout
	if timeout == 0 {
		timeout = DefaultRepoCloneTimeout
	}
	err = r.Runner(cmd, timeout)

	if err != nil {
		os.RemoveAll(path.Join(r.Box.RootFS(), clonePath))
		return err
	}
	return nil
}

// Runner is a superset of utils.Exec.  The main difference is that it checks the stderr of the result for clone-related errors.
func (r *Repo) Runner(cmd *exec.Cmd, timeout time.Duration) error {
	var stderr bytes.Buffer
	c := make(chan error, 1)
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		logger.Println("ERROR: command error", fmt.Errorf("%v: invalid cmd, %v", cmd, err))
		return DefaultCloneError
	}

	go func() {
		c <- cmd.Wait()
	}()

	select {
	case <-time.After(timeout):
		cmd.Process.Kill()
		return CloneTimeoutError
	case err := <-c:
		if err == nil {
			return nil
		}
		if strings.Contains(string(stderr.Bytes()), "Disk quota exceeded") {
			return DiskSpaceExceededError
		}
		logger.Printf("ERROR: Unexpected git clone error. Stderr: %v, Err: %v\n", string(stderr.Bytes()), err)
		return DefaultCloneError
	}
	return nil // never reached, satisfy compiler
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
	return "", NoDestinationError
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
	timeout := r.IdentifyTimeout
	if timeout == 0 {
		timeout = DefaultRepoIdentifyTimeout
	}
	return r.Runner(r.boxUserCommand("git", "ls-remote", r.Url), timeout) == nil
}

func (r *Repo) isMercurial() bool {
	timeout := r.IdentifyTimeout
	if timeout == 0 {
		timeout = DefaultRepoIdentifyTimeout
	}
	return r.Runner(r.boxUserCommand("hg", "identify", r.Url), timeout) == nil
}

// boxUserCommand is a wrapper for exec.Command that runs the command as the correct user and rootdir
func (r *Repo) boxUserCommand(cmd string, args ...string) *exec.Cmd {
	userspecArg := fmt.Sprintf("--userspec=%s:%s", r.Box.Uid(), r.Box.Gid())
	cmdArgs := []string{userspecArg, r.Box.RootFS()}
	cmdArgs = append(cmdArgs, cmd)
	cmdArgs = append(cmdArgs, args...)
	return exec.Command("chroot", cmdArgs...)
}
