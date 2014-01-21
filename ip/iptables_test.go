package ip

import (
	. "launchpad.net/gocheck"
	"os/exec"
	"strconv"
	"strings"
	"sync"
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

func (s *IptablesSuite) TestMultipleOpen(c *C) {
	rule := natRule{"PREROUTING", "80", "192.168.0.1:10001"}
	c.Assert(rule.exists(), Equals, false)
	var err error

	err = Open("80", "192.168.0.1", "10001")
	c.Assert(err, IsNil)
	err = Open("80", "192.168.0.1", "10001")
	c.Assert(err, IsNil)
	c.Assert(rule.exists(), Equals, true)

	output, _ := exec.Command("iptables", "-L", "-t", "nat").Output()
	c.Assert(strings.Count(string(output), "192.168.0.1:10001"), Equals, 1)

	err = Close("80", "192.168.0.1", "10001")
	c.Assert(err, IsNil)
	err = Close("80", "192.168.0.1", "10001")
	c.Assert(err, IsNil)
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

func (s *IptablesSuite) TestConcurrentIptablesOpenClose(c *C) {
	var wg sync.WaitGroup
	for i := 1; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			port := strconv.Itoa(10000 + i)
			ip := "192.168.0." + strconv.Itoa(i)
			c.Assert(Open("80", ip, port), IsNil)
			c.Assert(Close("80", ip, port), IsNil)
		}(i)
	}
	wg.Wait()
}
