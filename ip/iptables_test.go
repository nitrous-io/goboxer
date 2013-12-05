package ip

import (
	. "launchpad.net/gocheck"
)

type IptablesSuite struct{}

var _ = Suite(&IptablesSuite{})

func (s *IptablesSuite) TestOpenClose(c *C) {
	rule := natRule("-C", "PREROUTING", "80", "192.168.0.1:10001")
	assertExistRule(c, rule, false)
	Open("80", "192.168.0.1", "10001")
	defer Close("80", "192.168.0.1", "10001")
	assertExistRule(c, rule, true)
}

func (s *IptablesSuite) TestOpenCloseInVagrant(c *C) {
	prerouting := natRule("-C", "PREROUTING", "80", "192.168.0.1:10001")
	output := natRule("-C", "OUTPUT", "80", "192.168.0.1:10001")
	assertExistRule(c, prerouting, false)
	assertExistRule(c, output, false)
	OpenInVagrant("80", "192.168.0.1", "10001")
	defer CloseInVagrant("80", "192.168.0.1", "10001")
	assertExistRule(c, prerouting, true)
	assertExistRule(c, output, true)
}

func (s *IptablesSuite) TestMarkUnmark(c *C) {
	mangle := mangleRule("-C", "OUTPUT", "10001", "10")
	assertExistRule(c, mangle, false)
	Mark("10001", "10")
	defer Unmark("10001", "10")
	assertExistRule(c, mangle, true)
}
