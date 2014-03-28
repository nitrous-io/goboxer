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
	Box          *Box
	Url          string
	Scm          string
	CloneTimeout time.Duration
}

// possible SCM values, for now we only support Git and Mercurial
const (
	ScmUnsupported = "unsupported"
	ScmGit         = "git"
	ScmMercurial   = "mercurial"
)

const (
	DefaultRepoCloneTimeout = 5 * time.Minute
)

// all returned errors are passed on to Message and are user-facing
var (
	DefaultCloneError      = fmt.Errorf("Clone command failed")
	UnsupportedCloneError  = fmt.Errorf("URL does not look like a Git or Mercurial repository")
	DiskSpaceExceededError = fmt.Errorf("Repository size exceeds available disk space")
	CloneTimeoutError      = fmt.Errorf("Clone took too long")
	NoDestinationErrorFmt  = "Destination directory %s already exists.  Please rename it and try again"
)

// Clone takes a parentPath relative to the box root and uses that as the parent directory of the cloned repository
func (r *Repo) Clone(parentPath string) error {
	var err error
	var cmd *exec.Cmd

	clonePath, err := r.clonePath(parentPath)
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

// clonePath takes a baseLocation relative to the box root and appends the repo basename to it.  Returns an error if this directory already exists
func (r *Repo) clonePath(parentPath string) (string, error) {
	parts := strings.Split(r.Url, "/")
	name := strings.TrimSuffix(parts[len(parts)-1], ".git")
	clonePath := path.Join(parentPath, name)
	if _, err := os.Stat(path.Join(r.Box.RootFS(), clonePath)); err == nil || !os.IsNotExist(err) {
		return "", fmt.Errorf(NoDestinationErrorFmt, clonePath)
	} else {
		return clonePath, nil
	}
}

// boxUserCommand is a wrapper for exec.Command that runs the command as the correct user and rootdir
func (r *Repo) boxUserCommand(cmd string, args ...string) *exec.Cmd {
	userspecArg := fmt.Sprintf("--userspec=%s:%s", r.Box.Uid(), r.Box.Gid())
	cmdArgs := []string{userspecArg, r.Box.RootFS()}
	cmdArgs = append(cmdArgs, cmd)
	cmdArgs = append(cmdArgs, args...)
	return exec.Command("chroot", cmdArgs...)
}
