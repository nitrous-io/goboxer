package goboxer

import (
	"github.com/action-io/goboxer/utils"
	. "github.com/motain/gocheck"
)

type CoachListenerSuite struct{}

var _ = Suite(&CoachListenerSuite{})

func (s *CoachListenerSuite) TestDispatchIgnoreBoxer(c *C) {
	listener := &CoachListener{
		Boxer: &Boxer{Id: "test-boxer"},
	}
	message := Message{
		Id:    "test-boxer-1",
		BoxId: 1,
		Type:  "aio:invalid_message",
	}
	json, _ := utils.ToJSON(message)
	_, err := listener.Dispatch([]byte(json))
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "NotHandle")
}
