package goboxer

import (
	"bytes"
	"fmt"
	"io/ioutil"
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
	Box  *Box
	Name string
	Url  string
	Scm  string

	// cannot be less than a second -- only supported in curl 7.32.0 and boxes currently have 7.22.0
	RetrieveTimeout time.Duration
}

// possible SCM values, for now we only support Git and Mercurial
const (
	ScmUnsupported = "unsupported"
	ScmGit         = "git"
	ScmMercurial   = "mercurial"
	ScmTargz       = "targz"
)

const DefaultRepoRetrieveTimeout = 5 * time.Minute

// The response returned by Github when hitting username/repo/archive/branch.tar.gz if a repository is empty
const EmptyRepositoryResponse = "Not Found"

// all returned errors are passed on to Message and are user-facing
var (
	DefaultRetrieveError   = fmt.Errorf("Repository download failed")
	UnsupportedCloneError  = fmt.Errorf("URL does not look like a Git or Mercurial repository")
	DiskSpaceExceededError = fmt.Errorf("Repository size exceeds available disk space")
	RetrieveTimeoutError   = fmt.Errorf("Clone took too long")
	NoDestinationErrorFmt  = "Destination directory %s already exists.  Please rename it and try again"
	EmptyRepositoryError   = fmt.Errorf("Could not find files in repository.  Please check if repository is empty")
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
	if r.Box.State != "running" {
		logger.Printf("ERROR: attempt to retrieve %v when box %v is not running\n", r, r.Box)
		return DefaultRetrieveError
	}

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
func (r *Repo) download(downloadPath string) (err error) {
	defer func() {
		if err != nil {
			os.RemoveAll(path.Join(r.Box.RootFS(), downloadPath))
		}
	}()

	cmd := r.boxUserCommand("mkdir", "-p", downloadPath)
	if err = runAndCatchKnownErrors(cmd, "repo download mkdir"); err != nil {
		return err
	}

	var (
		reposRootPath   = path.Join(r.Box.RootFS(), "repos")
		archiveFile     = fmt.Sprintf("%s.tar.gz", r.Name)
		archiveUserPath = path.Join("/repos", archiveFile)
		archiveRootPath = path.Join(r.Box.RootFS(), archiveUserPath)
	)

	os.MkdirAll(reposRootPath, 0755)
	defer os.RemoveAll(reposRootPath)
	if err = r.downloadArchive(archiveRootPath); err != nil {
		return err
	}

	if err = runAndCatchKnownErrors(exec.Command("sync"), "repo download sync"); err != nil {
		return err
	}

	fi, err := os.Stat(archiveRootPath)
	if err != nil {
		logger.Printf("ERROR: repo download stat: %v\n", err)
		return DefaultRetrieveError
	}
	if fi.Size() == int64(len(EmptyRepositoryResponse)) {
		if dat, _ := ioutil.ReadFile(archiveRootPath); string(dat) == EmptyRepositoryResponse {
			return EmptyRepositoryError
		}
	}

	// assume time taken for tar is neglible compared to that of download, so
	// time taken extracting is not considered in RetrieveTimeout
	cmd = r.boxUserCommand("tar", "-xzSpf", archiveUserPath,
		"--strip", "1", // GNU tar specific option, strip leading path of reponame-branchname/ (Github convention)
		"-C", downloadPath)
	return runAndCatchKnownErrors(cmd, "repo download tar")
}

func (r *Repo) downloadArchive(archivePath string) error {
	cmd := exec.Command("curl", "--silent",
		"--location", r.Url,
		"--connect-timeout", "10",
		"--max-time", fmt.Sprintf("%.0f", r.RetrieveTimeout.Seconds()),
		"-o", archivePath)
	if err := runAndCatchKnownErrors(cmd, "repo download curl"); err != nil {
		return err
	}
	cmd = exec.Command("chmod", "a+r", archivePath)
	return runAndCatchKnownErrors(cmd, "repo download chmod")
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

// runAndCatchKnownErrors takes a command and a log description for logging error (if present) and command output.  If the command results in a known error, it will return a specific error, otherwise it returns a generic one
func runAndCatchKnownErrors(cmd *exec.Cmd, logSlug string) error {
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	// See http://curl.haxx.se/libcurl/c/libcurl-errors.html for CURLE errors
	switch {
	case err == nil:
		return nil
	case strings.Contains(output, "Disk quota exceeded"):
		return DiskSpaceExceededError
	case err.Error() == "exit status 23":
		// CURLE_WRITE_ERROR (23)
		return DiskSpaceExceededError
	case err.Error() == "exit status 28":
		// CURLE_OPERATION_TIMEDOUT (28)
		return RetrieveTimeoutError
	default:
		logger.Printf("ERROR: %s: %v, output: %s\n", logSlug, err,
			strings.Replace(output, "\n", "\\n", -1))
		return DefaultRetrieveError
	}
}
