package ip

import (
	. "launchpad.net/gocheck"
	"os/exec"
	"testing"
)

// hook up gocheck into go test runner
func Test(t *testing.T) { TestingT(t) }

func assertExistRule(c *C, rule []string, exists bool) {
	cmd := exec.Command("iptables", rule...)
	cmd.Run()
	c.Assert(cmd.ProcessState.Success(), Equals, exists)
}
