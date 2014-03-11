package goboxer

import (
	"fmt"
	"os"
	"os/exec"
	"path"

	. "github.com/motain/gocheck"
)

const (
	ScratchDir = "/tmp/repo-test"
)

var (
	FakeGitRepoPath = path.Join(ScratchDir, "fake-git")
	FakeHgRepoPath  = path.Join(ScratchDir, "fake-hg")
	BadRepoPath     = path.Join(ScratchDir, "fake-bad")
)

type RepoSuite struct {
	GitRepo, HgRepo, BadRepo Repo
}

var _ = Suite(&RepoSuite{})

func (s *RepoSuite) SetUpSuite(c *C) {
	fakeRepo := func(cmd *exec.Cmd, wd string) ([]byte, error) {
		if err := os.MkdirAll(wd, 0755); err != nil {
			return []byte{}, err
		}
		cmd.Dir = wd
		return cmd.CombinedOutput()
	}

	out, err := fakeRepo(exec.Command("git", "init"), FakeGitRepoPath)
	c.Assert(err, IsNil, Commentf("Error on git init: %s\nOutput: %s", err, out))

	out, err = fakeRepo(exec.Command("hg", "init"), FakeHgRepoPath)
	c.Assert(err, IsNil, Commentf("Error on hg init: %s\nOutput: %s", err, out))

	out, err = fakeRepo(exec.Command("echo", "no-op"), BadRepoPath)
	c.Assert(err, IsNil, Commentf("Error on no-op: %s\nOutput: %s", err, out))
}

func (s *RepoSuite) TearDownSuite(c *C) {
	os.RemoveAll(ScratchDir)
}

func (s *RepoSuite) SetUpTest(c *C) {
	s.GitRepo = Repo{Url: fmt.Sprintf("file://%s", FakeGitRepoPath)}
	s.HgRepo = Repo{Url: fmt.Sprintf("file://%s", FakeHgRepoPath)}
	s.BadRepo = Repo{Url: fmt.Sprintf("file://%s", BadRepoPath)}
}

func (s *RepoSuite) TestIsGit(c *C) {
	c.Assert(s.GitRepo.isGit(), Equals, true)
	c.Assert(s.HgRepo.isGit(), Equals, false)
	c.Assert(s.BadRepo.isGit(), Equals, false)
}

func (s *RepoSuite) TestIsHg(c *C) {
	c.Assert(s.HgRepo.isMercurial(), Equals, true)
	c.Assert(s.GitRepo.isMercurial(), Equals, false)
	c.Assert(s.BadRepo.isMercurial(), Equals, false)
}

func (s *RepoSuite) TestIdentifyScm(c *C) {
	s.GitRepo.identifyScm()
	c.Assert(s.GitRepo.Scm, Equals, ScmGit)
	s.HgRepo.identifyScm()
	c.Assert(s.HgRepo.Scm, Equals, ScmMercurial)
	s.BadRepo.identifyScm()
	c.Assert(s.BadRepo.Scm, Equals, ScmUnsupported)
}
