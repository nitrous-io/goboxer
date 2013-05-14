package goboxer

import (
	. "launchpad.net/gocheck"
)

type ResourceTestSuite struct{}

var _ = Suite(&ResourceTestSuite{})

func (s *ResourceTestSuite) TestToJSON(c *C) {
	resource := &Resource{
		Ip:               "192.168.0.253",
		Gateway:          "192.168.0.1",
		NetworkInterface: "br-lxc-254",
		Port:             "10085"}
	rjson, _ := resource.ToJSON()
	expected := "{\"ip\":\"192.168.0.253\",\"gateway\":\"192.168.0.1\",\"interface\":\"br-lxc-254\",\"port\":\"10085\"}"
	c.Assert(rjson, Equals, expected)
}
