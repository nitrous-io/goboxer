package goboxer

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/action-io/goboxer/utils"
)

// a Repo is a link to a repository to be cloned into a box
type Repo struct {
	Box             *Box
	Name            string
	Url             string
	Scm             string
	RetrieveTimeout time.Duration
}

// possible SCM values, for now we only support Git and Mercurial
const (
	ScmUnsupported = "unsupported"
	ScmGit         = "git"
	ScmMercurial   = "mercurial"
	ScmTargz       = "targz"
)

const (
	DefaultRepoRetrieveTimeout = 5 * time.Minute
)

// all returned errors are passed on to Message and are user-facing
var (
	DefaultRetrieveError   = fmt.Errorf("Repository download failed")
	UnsupportedCloneError  = fmt.Errorf("URL does not look like a Git or Mercurial repository")
	DiskSpaceExceededError = fmt.Errorf("Repository size exceeds available disk space")
	RetrieveTimeoutError   = fmt.Errorf("Clone took too long")
	NoDestinationErrorFmt  = "Destination directory %s already exists.  Please rename it and try again"
)

// ValidateRepo returns an error if any Repo fields are invalid, otherwise sets the Name field and returns nil
func ValidateRepo(repo *Repo) error {
	var err error
	if repo.Url == "" {
		return fmt.Errorf("Missing repository url")
	}
	if repo.Scm == "" {
		return fmt.Errorf("Missing repository scm")
	}
	repo.Name, err = repo.name()
	if err != nil {
		return fmt.Errorf("Bad repository url: %s", err)
	}

	return nil
}

// Retrieve takes a parentPath relative to the box root and uses that as the parent directory when cloning or downloading a repository
func (r *Repo) Retrieve(parentPath string) error {
	if r.RetrieveTimeout == 0 {
		r.RetrieveTimeout = DefaultRepoRetrieveTimeout
	}
	retrievePath, err := r.retrievePath(parentPath)
	if err != nil {
		return err
	}

	switch r.Scm {
	case ScmGit, ScmMercurial:
		return r.clone(retrievePath)
	case ScmTargz:
		return r.download(retrievePath)
	case ScmUnsupported:
		return UnsupportedCloneError
	default:
		panic("Switch not exhaustive")
	}
}

// clone is the entry point for cloning a repository, used for ScmGit and ScmMercurial
func (r *Repo) clone(clonePath string) error {
	var cmd *exec.Cmd
	url := utils.ShellEscape(r.Url)
	clonePath = utils.ShellEscape(clonePath)

	switch {
	case r.Scm == ScmGit:
		cmd = r.boxUserCommand("git", "clone", "--depth", "1", url, clonePath)
	case r.Scm == ScmMercurial:
		// TODO use shallow clones when Mercurial supports it
		// http://mercurial.selenic.com/wiki/ShallowClone
		cmd = r.boxUserCommand("hg", "clone", url, clonePath)
	default:
		return fmt.Errorf("clone expected a Git or Mercurial repo, got %s", r.Scm)
	}

	// TODO differentiate between clone timeout due to connection timeout (no
	// output) vs timeout due to download taking too long.  With typical
	// net.ipv4.tcp_syn_retries values the timeout for former can be 2-3 mins.
	err := r.Runner(cmd, r.RetrieveTimeout)

	if err != nil {
		os.RemoveAll(path.Join(r.Box.RootFS(), clonePath))
		return err
	}
	return nil
}

// download is the entry point for downloading an archive, used for ScmTargz
func (r *Repo) download(downloadPath string) error {
	archivePath := path.Join("/tmp", r.Name+".tar.gz")

	err := r.downloadArchive(archivePath)
	if err != nil {
		return err
	}
	defer os.RemoveAll(path.Join(r.Box.RootFS(), archivePath))

	cmd := r.boxUserCommand("mkdir", "-p", downloadPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Println("ERROR: repo download mkdir %v, output: %s\n", err, output)
		return DefaultRetrieveError
	}
	// TODO catch disk space exceeded
	cmd = r.boxUserCommand("tar", "-xzSpf", archivePath,
		"--strip", "1", // GNU tar specific option, strip leading path of reponame-branchname/ (Github convention)
		"-C", downloadPath)

	// assume time taken for tar is neglible compared to that of download, so
	// time taken extracting is not considered in RetrieveTimeout
	output, err = cmd.CombinedOutput()
	if err != nil {
		logger.Println("ERROR: repo download tar %v, output: %s\n", err, output)
		return DefaultRetrieveError
	}
	return nil
}

func (r *Repo) downloadArchive(archivePath string) error {
	// TODO catch disk space exceeded
	cmd := r.boxUserCommand("/bin/sh", "-c",
		fmt.Sprintf("curl --silent --location %s --connect-timeout 10 --max-time %.0f > %s", r.Url, r.RetrieveTimeout.Seconds(), archivePath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Printf("ERROR: repo download curl %v, output: %s\n", err, output)
		return DefaultRetrieveError
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
		return DefaultRetrieveError
	}

	go func() {
		c <- cmd.Wait()
	}()

	select {
	case <-time.After(timeout):
		cmd.Process.Kill()
		return RetrieveTimeoutError
	case err := <-c:
		if err == nil {
			return nil
		}
		if strings.Contains(string(stderr.Bytes()), "Disk quota exceeded") {
			return DiskSpaceExceededError
		}
		logger.Printf("ERROR: Unexpected git clone error. Stderr: %v, Err: %v\n", string(stderr.Bytes()), err)
		return DefaultRetrieveError
	}
	return nil // never reached, satisfy compiler
}

// retrievePath takes a baseLocation relative to the box root and appends the repo basename to it.  Returns an error if this directory already exists
func (r *Repo) retrievePath(parentPath string) (string, error) {
	retrievePath := path.Join(parentPath, r.Name)
	if _, err := os.Stat(path.Join(r.Box.RootFS(), retrievePath)); err == nil || !os.IsNotExist(err) {
		return "", fmt.Errorf(NoDestinationErrorFmt, retrievePath)
	} else {
		return retrievePath, nil
	}
}

// name returns the name of the repository sans git if present.  It requires hardcoded knowledge of urls since it has to handle both archive and git clone urls.  Presently only Github urls are supported.
func (r *Repo) name() (string, error) {
	// invalid urls are rejected at the message handler level
	u, err := url.Parse(r.Url)
	if err != nil {
		return "", err
	}
	parts := strings.Split(u.Path, "/")

	// Expected urls:
	// - clone https://github.com/nitrous-io/autoparts.git
	// - archive https://github.com/nitrous-io/autoparts/archive/master.tar.gz
	switch r.Scm {
	case ScmGit, ScmMercurial:
		// TODO check path structure in a way that wouldn't break tests
		return strings.TrimSuffix(parts[len(parts)-1], ".git"), nil
	case ScmTargz:
		return parts[len(parts)-3], nil
	}
	return "", fmt.Errorf("Unexpected Scm %s", r.Scm)
}

// boxUserCommand is a wrapper for exec.Command that runs the command as the correct user and rootdir
func (r *Repo) boxUserCommand(cmd string, args ...string) *exec.Cmd {
	userspecArg := fmt.Sprintf("--userspec=%s:%s", r.Box.Uid(), r.Box.Gid())
	cmdArgs := []string{userspecArg, r.Box.RootFS()}
	cmdArgs = append(cmdArgs, cmd)
	for _, arg := range args {
		cmdArgs = append(cmdArgs, arg)
	}
	return exec.Command("chroot", cmdArgs...)
}
