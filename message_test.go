package goboxer

import (
	. "launchpad.net/gocheck"
	"strings"
)

type MessageSuite struct{}

var _ = Suite(&MessageSuite{})

func (s *MessageSuite) TestToJSONOmitEmpty(c *C) {
	m := Message{Id: "1"}
	json, _ := ToJSON(m)
	res := map[string]bool{
		"id":          true,
		"boxId":       false,
		"type":        false,
		"returnVars":  false,
		"freeMemory":  false,
		"runtimes":    false,
		"name":        false,
		"port":        false,
		"snapshotURL": false,
		"memory":      false,
		"diskSpace":   false,
		"sshKeyPair":  false,
		"publicKeys":  false,
		"dataType":    false,
		"data":        false,
		"success":     false,
		"info":        true,
		// seems that embeded struct will get serialized
		// anyway. It's ok for now to leave it here
	}

	for k, v := range res {
		c.Assert(strings.Contains(json, k), Equals, v, Commentf(k, v))
	}
}
