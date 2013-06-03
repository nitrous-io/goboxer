package goboxer

import (
	"github.com/bitly/go-simplejson"
	"github.com/garyburd/redigo/redis"
)

type CoachListener struct {
	Runtime      *Runtime
	BoxerConn    redis.Conn
	CoachConn    redis.Conn
	BoxerPubSub  redis.PubSubConn
	BoxerChannel string
	CoachChannel string
}

func (c *CoachListener) Listen() {
	switch msg := c.BoxerPubSub.Receive().(type) {
	case redis.Message:
		json, err := simplejson.NewJson(msg.Data)
		if err != nil {
			c.SendInvalidParam()
			return
		}
		returnedMessage, err := c.Runtime.Receive(json)
		if err != nil {
			c.SendInvalidParam()
			return
		}
		messageJSON, err := ToJSON(returnedMessage)
		if err != nil {
			c.SendInvalidParam()
			return
		}
		c.replyCoach(messageJSON)
	case redis.Subscription:
		if msg.Count == 0 {
			return
		}
	}
}

// Advertise itself to Coach
func (c *CoachListener) Advertise() {
	message := c.Runtime.NewAdvertiseMessage()
	messageJSON, err := ToJSON(message)
	if err != nil {
		c.SendInvalidParam()
	}
	c.CoachConn.Do("PUBLISH", c.CoachChannel, messageJSON)
}

func (c *CoachListener) SendInvalidParam() {
	message := c.Runtime.NewInvalidParamMessage()
	messageJSON, err := ToJSON(message)
	if err != nil {
		c.replyCoach("{\"success\":false}")
		return
	}
	c.replyCoach(messageJSON)
}

func (c *CoachListener) replyCoach(msg string) {
	c.CoachConn.Do("PUBLISH", c.CoachChannel, msg)
}
