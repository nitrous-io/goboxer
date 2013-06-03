package host

import (
	. "launchpad.net/gocheck"
)

type HostSuite struct{}

var _ = Suite(&HostSuite{})

func (s *HostSuite) TestFreeMemory(c *C) {
	memory := FreeMemory()
	c.Assert(memory > 0, Equals)
}
