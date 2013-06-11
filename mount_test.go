package goboxer

import (
	. "launchpad.net/gocheck"
	"os"
	"path/filepath"
)

type MountSuite struct {
	source string
	delta  string
	target string
}

var _ = Suite(&MountSuite{})

func (s *MountSuite) SetUpTest(c *C) {
	s.source, _ = filepath.Abs("/tmp/test_source")
	s.delta, _ = filepath.Abs("/tmp/test_source_delta")
	s.target, _ = filepath.Abs("/tmp/test_target")
	os.MkdirAll(s.source, 0755)
	os.MkdirAll(s.delta, 0755)
	os.MkdirAll(s.target, 0755)
}

func (s *MountSuite) TearDownTest(c *C) {
	os.RemoveAll(s.source)
	os.RemoveAll(s.delta)
	os.RemoveAll(s.target)
}

func (s *MountSuite) TestMount(c *C) {
	MountOverlay(s.source, s.delta, s.target)
	mounted, _ := Mounted(s.source)
	c.Assert(mounted, Equals, true)

	UnmountOverlay(s.source)
	mounted, _ = Mounted(s.source)
	c.Assert(mounted, Equals, false)
}
