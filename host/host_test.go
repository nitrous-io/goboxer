package host

import (
	. "launchpad.net/gocheck"
	"testing"
)

// hook up gocheck into go test runner
func Test(t *testing.T) { TestingT(t) }

type HostSuite struct{}

var _ = Suite(&HostSuite{})

func (s *HostSuite) TestFreeMemory(c *C) {
	memory := FreeMemory()
	c.Assert(memory > 0, Equals, true)
}
