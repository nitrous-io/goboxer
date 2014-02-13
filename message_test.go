package goboxer

import (
	"encoding/json"
	"github.com/action-io/goboxer/utils"
	. "launchpad.net/gocheck"
	"strings"
)

type MessageSuite struct{}

var _ = Suite(&MessageSuite{})

func (s *MessageSuite) TestToJSONOmitEmpty(c *C) {
	m := Message{Id: "1"}
	json, _ := utils.ToJSON(m)
	res := map[string]bool{
		"id":          true,
		"boxId":       false,
		"type":        false,
		"returnVars":  false,
		"freeMemory":  false,
		"runtimes":    false,
		"name":        false,
		"port":        false,
		"snapshotUrl": false,
		"memory":      false,
		"diskSpace":   false,
		"sshKeyPair":  false,
		"publicKeys":  false,
		"dataType":    false,
		"data":        false,
		"info":        true,
		// seems that embeded struct will get serialized
		// anyway. It's ok for now to leave it here
	}

	for k, v := range res {
		c.Assert(strings.Contains(json, k), Equals, v, Commentf(k, v))
	}
}

func (s *MessageSuite) TestToJSONSuccessPresent(c *C) {
	msg := Message{Id: "1"}
	json, _ := utils.ToJSON(msg)
	c.Assert(strings.Contains(json, `"success":false`), Equals, true)

	msg = Message{Id: "1", Success: true}
	json, _ = utils.ToJSON(msg)
	c.Assert(strings.Contains(json, `"success":true`), Equals, true)
}

func (s *MessageSuite) TestFromJSON(c *C) {
	msg := "{\"id\":\"46.137.252.155\",\"name\":\"rbtest1-372\",\"data\":[\"HEROKU_POSTGRES_URL\"],\"dataType\":\"aio:configure:remove_env_variables\"" +
		",\"type\":\"aio:configure\"}"
	message := Message{}
	json.Unmarshal([]byte(msg), &message)
	data := []string{}
	vdata, _ := (message.Data).([]interface{})
	for i := 0; i < len(vdata); i++ {
		data = append(data, vdata[i].(string))
	}
	c.Assert(data[0], Equals, "HEROKU_POSTGRES_URL")
}
