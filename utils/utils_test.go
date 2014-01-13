package utils

import (
	"fmt"
	. "launchpad.net/gocheck"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// hook up gocheck into go test runner
func Test(t *testing.T) { TestingT(t) }

type UtilSuite struct{}

var _ = Suite(&UtilSuite{})

func (s *UtilSuite) TestExecWithTimeout(c *C) {
	now := time.Now()
	// timedout before cmd finish
	err := Exec(exec.Command("sleep", "30"), 5*time.Millisecond)
	c.Assert(err, NotNil)
	c.Assert(time.Now().Sub(now) >= 5*time.Millisecond, Equals, true)
	c.Assert(time.Now().Sub(now) < 30*time.Second, Equals, true)

	// finish before timed out
	err = Exec(exec.Command("sleep", "0.01"), 50*time.Millisecond)
	c.Assert(err, IsNil)

	// cmd error
	err = Exec(exec.Command("invalidcmd"), 50*time.Millisecond)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(fmt.Sprintf("%v", err), "invalid cmd"), Equals, true)
	c.Assert(time.Now().Sub(now) >= 50*time.Millisecond, Equals, false)
}
