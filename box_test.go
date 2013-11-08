package goboxer

import (
	"encoding/json"
	. "launchpad.net/gocheck"
)

type BoxSuite struct{}

var _ = Suite(&BoxSuite{})

func (s *BoxSuite) TestTotalMemory(c *C) {
	b := &Box{Memory: "384M"}
	c.Assert(b.TotalMemory(), Equals, "1152M")
}

func (s *BoxSuite) TestMarshal(c *C) {
	testData := struct {
		Id   string `json:"id"`
		Port int    `json:"port"`
	}{"1", 10000}
	b, _ := ToJSON(testData)
	box := &Box{}
	json.Unmarshal([]byte(b), box)
	c.Assert(box.Port, Equals, 10000)
}

type testMountMethod struct {
	name     string
	expected string
}

func (s *BoxSuite) TestMountMethod(c *C) {
	b := &Box{}
	c.Assert(b.GetMountMethod(), Equals, "overlayfs")

	b.MountMethod = "aufs"
	c.Assert(b.GetMountMethod(), Equals, "aufs")

	b.MountMethod = ""
	b.Aufs = true
	c.Assert(b.GetMountMethod(), Equals, "aufs")

	b.MountMethod = "lvm"
	c.Assert(b.GetMountMethod(), Equals, "lvm")
}

func (s *BoxSuite) TestQuotaMethod(c *C) {
	b := &Box{}
	c.Assert(b.GetQuotaMethod(), Equals, "prjquota")

	b.QuotaMethod = "usrquota"
	c.Assert(b.GetQuotaMethod(), Equals, "usrquota")
}
