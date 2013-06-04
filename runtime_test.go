package goboxer

import (
	. "launchpad.net/gocheck"
)

func configure(message Message, r *Runtime) (Message, error) {
	m := Message{
		Id:   message.Id + "1",
		Type: message.Type,
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
		Id:   "123",
	}
	msg, _ := runtime.Receive(m)
	c.Assert(msg.Type, Equals, "aio:configure")
	c.Assert(msg.Id, Equals, "1231")
}
