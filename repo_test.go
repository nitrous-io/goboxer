package goboxer

import (
	"fmt"
	"os"
	"os/exec"
	"path"

	. "github.com/motain/gocheck"
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
	os.RemoveAll(ScratchDir)
	err := MakeRepo(FakeGitRepoPath, ScmGit)
	c.Assert(err, IsNil, Commentf("Error on git fake: %s\n", err))

	err = MakeRepo(FakeHgRepoPath, ScmMercurial)
	c.Assert(err, IsNil, Commentf("Error on hg fake: %s\n", err))

	err = MakeRepo(BadRepoPath, ScmUnsupported)
	c.Assert(err, IsNil, Commentf("Error on unsupported fake: %s\n", err))
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

func (s *RepoSuite) TestFreshLocation(c *C) {
	url := "https://github.com/papers-we-love/papers-we-love.git"
	dir, _ := (&Repo{Url: url}).freshLocation(ScratchDir)
	c.Assert(dir, Equals, path.Join(ScratchDir, "papers-we-love"))

	for _, suffix := range []string{"", "1", "2", "3"} {
		os.Mkdir(path.Join(ScratchDir, "papers-we-love"+suffix), 0755)
	}

	dir, _ = (&Repo{Url: url}).freshLocation(ScratchDir)
	c.Assert(dir, Equals, path.Join(ScratchDir, "papers-we-love4"))

	os.Mkdir(path.Join(ScratchDir, "papers-we-love4"), 0755)
	_, err := (&Repo{Url: url}).freshLocation(ScratchDir)
	c.Check(err, NotNil)
	c.Assert(err.Error(), Equals, "No applicable destination directory")
}

func (s *RepoSuite) TestCloneGit(c *C) {
	cloneDir := path.Join(ScratchDir, "clone-dest")
	os.MkdirAll(cloneDir, 0755)
	s.GitRepo.Clone(cloneDir)
	cmd := exec.Command("git", "status")
	cmd.Dir = path.Join(cloneDir, "fake-git")
	c.Assert(cmd.Run(), IsNil)
}

func (s *RepoSuite) TestCloneHg(c *C) {
	cloneDir := path.Join(ScratchDir, "clone-dest")
	os.MkdirAll(cloneDir, 0755)
	s.HgRepo.Clone(cloneDir)
	cmd := exec.Command("hg", "summary")
	cmd.Dir = path.Join(cloneDir, "fake-hg")
	c.Assert(cmd.Run(), IsNil)
}

func (s *RepoSuite) TestCloneUnsupported(c *C) {
	cloneDir := path.Join(ScratchDir, "clone-dest")
	os.MkdirAll(cloneDir, 0755)
	err := s.BadRepo.Clone(cloneDir)
	c.Check(err, NotNil)
	c.Assert(err.Error(), Equals,
		fmt.Sprintf("SCM for %s cannot be determined", s.BadRepo.Url))
}

func (s *RepoSuite) TestCloneWithExecCommandError(c *C) {
	cloneDir := path.Join(ScratchDir, "clone-dest")
	os.MkdirAll(cloneDir, 0755)
	s.GitRepo.Scm = ScmMercurial
	err := s.GitRepo.Clone(cloneDir)

	// FIXME: need to change Clone() to return more descriptive errors
	c.Assert(err, ErrorMatches, "exit status 255")
}
