package goboxer

import (
	"github.com/bitly/go-simplejson"
	. "launchpad.net/gocheck"
)

func configure(message *simplejson.Json) (Message, error) {
	id, _ := message.Get("id").Int()
	messageType, _ := message.Get("type").String()
	m := Message{
		Id:   id + 1,
		Type: messageType,
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
	m := Message{
		Type: "aio:configure",
		Id:   123,
	}
	json, _ := simplejson.NewJson([]byte(m))
	msg, _ := runtime.Receive(json)
	returnedMessage := msg.(ConfigureMessageTest)
	c.Assert(returnedMessage.Type, Equals, "aio:configure")
	c.Assert(returnedMessage.Id, Equals, 124)
}
