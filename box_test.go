package goboxer

import (
	"encoding/json"
	"github.com/action-io/goboxer/utils"
	. "launchpad.net/gocheck"
)

type BoxSuite struct{}

var _ = Suite(&BoxSuite{})

func (s *BoxSuite) TestTotalMemory(c *C) {
	b := &Box{Memory: "384M"}
	c.Assert(b.TotalMemory(), Equals, "768M")
}

func (s *BoxSuite) TestMarshal(c *C) {
	testData := struct {
		Id   string `json:"id"`
		Port int    `json:"port"`
	}{"1", 10000}
	b, _ := utils.ToJSON(testData)
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
	c.Assert(b.getMountMethod(), Equals, "overlayfs")

	b.MountMethod = "aufs"
	c.Assert(b.getMountMethod(), Equals, "aufs")

	b.MountMethod = ""
	b.Aufs = true
	c.Assert(b.getMountMethod(), Equals, "aufs")

	b.MountMethod = "lvm"
	c.Assert(b.getMountMethod(), Equals, "lvm")
}

func (s *BoxSuite) TestQuotaMethod(c *C) {
	b := &Box{}
	c.Assert(b.getQuotaMethod(), Equals, "prjquota")

	b.QuotaMethod = "usrquota"
	c.Assert(b.getQuotaMethod(), Equals, "usrquota")
}

func (s *BoxSuite) TestBoxAuthIsValid(c *C) {
	var b BoxAuth
	b = BoxAuth{
		Username: "hello",
		Password: "world",
	}
	c.Assert(b.IsValid(), Equals, true)

	b = BoxAuth{
		Username: "hello",
	}
	c.Assert(b.IsValid(), Equals, false)

	b = BoxAuth{
		Password: "hello",
	}
	c.Assert(b.IsValid(), Equals, false)

	b = BoxAuth{}
	c.Assert(b.IsValid(), Equals, true)
}
