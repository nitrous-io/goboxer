package goboxer

import (
	"github.com/bitly/go-simplejson"
	. "launchpad.net/gocheck"
)

type ConfigureMessageTest struct {
	Type string `json:"type" redis:"type" `
	Id   int    `json:"id" redis:"id" `
	Done bool   `json:"done" redis:"done" `
}

func configure(message *simplejson.Json) (Message, error) {
	id, _ := message.Get("id").Int()
	messageType, _ := message.Get("type").String()
	m := ConfigureMessageTest{
		Id:   id,
		Type: messageType,
		Done: true,
	}
	return m, nil
}

func newTestRuntime() *Runtime {
	return &Runtime{
		Handles: map[string]MessageHandler{
			"aio:configure": configure,
		},
	}
}

type RuntimeSuite struct{}

var _ = Suite(&RuntimeSuite{})

func (s *RuntimeSuite) TestReceive(c *C) {
	runtime := newTestRuntime()
	m, _ := MessageJSON(ConfigureMessageTest{
		Type: "aio:configure",
		Id:   123,
		Done: false,
	})
	json, _ := simplejson.NewJson([]byte(m))
	msg, _ := runtime.Receive(json)
	returnedMessage := msg.(ConfigureMessageTest)
	c.Assert(returnedMessage.Type, Equals, "aio:configure")
	c.Assert(returnedMessage.Id, Equals, 123)
	c.Assert(returnedMessage.Done, Equals, true)
}
