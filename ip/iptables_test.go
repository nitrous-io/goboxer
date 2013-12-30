package ip

import (
	. "launchpad.net/gocheck"
	"os/exec"
	"testing"
)

// hook up gocheck into go test runner
func Test(t *testing.T) { TestingT(t) }

type IptablesSuite struct{}

var _ = Suite(&IptablesSuite{})

func assertExistRule(c *C, rule []string, exists bool) {
	cmd := exec.Command("iptables", rule...)
	cmd.Run()
	c.Assert(cmd.ProcessState.Success(), Equals, exists)
}

func (s *IptablesSuite) TestOpenClose(c *C) {
	rule := natRule{"PREROUTING", "80", "192.168.0.1:10001"}
	c.Assert(rule.exists(), Equals, false)
	Open("80", "192.168.0.1", "10001")
	defer Close("80", "192.168.0.1", "10001")
	c.Assert(rule.exists(), Equals, true)
}

func (s *IptablesSuite) TestOpenCloseInVagrant(c *C) {
	prerouting := natRule{"PREROUTING", "80", "192.168.0.1:10001"}
	output := natRule{"OUTPUT", "80", "192.168.0.1:10001"}
	c.Assert(prerouting.exists(), Equals, false)
	c.Assert(output.exists(), Equals, false)
	OpenInVagrant("80", "192.168.0.1", "10001")
	defer CloseInVagrant("80", "192.168.0.1", "10001")
	c.Assert(prerouting.exists(), Equals, true)
	c.Assert(output.exists(), Equals, true)
}
