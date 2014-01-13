package utils

import (
	. "launchpad.net/gocheck"
	"os/exec"
	"testing"
	"time"
)

// hook up gocheck into go test runner
func Test(t *testing.T) { TestingT(t) }

type UtilSuite struct{}

var _ = Suite(&UtilSuite{})

func (s *UtilSuite) TestExecWithTimeout(c *C) {
	now := time.Now()
	cmd := exec.Command("sleep", "30")
	timeout := 5 * time.Millisecond
	err := Exec(cmd, timeout)
	c.Assert(err, NotNil)
	c.Assert(time.Now().Sub(now) >= timeout, Equals, true)
	c.Assert(time.Now().Sub(now) < 30*time.Second, Equals, true)
}
