package goboxer

import (
	"encoding/json"
	. "launchpad.net/gocheck"
	"strings"
)

type BoxSuite struct{}

var _ = Suite(&BoxSuite{})

func (s *BoxSuite) TestTotalMemory(c *C) {
	b := &Box{Memory: "384M"}
	c.Assert(b.TotalMemory(), Equals, "1152M")
}

func (s *BoxSuite) TestTarName(c *C) {
	b := &Box{Id: "1234", Name: "test"}
	c.Assert(b.TarName(), Not(Equals), "test-1234.tar.gz")
	c.Assert(strings.Contains(b.TarName(), "test-1234"), Equals, true)
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
