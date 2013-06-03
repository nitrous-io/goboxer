package goboxer

import (
	"encoding/json"
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
		var returnedMessage Message
		var messageJSON string
		var err error
		message := Message{}
		Debug(msg.Data)
		if err = json.Unmarshal(msg.Data, &message); err != nil {
			c.SendInvalidParam()
			return
		}
		Debug(message)
		if returnedMessage, err = c.Runtime.Receive(message); err != nil {
			c.SendInvalidParam()
			return
		}
		Debug(returnedMessage)
		if messageJSON, err = ToJSON(returnedMessage); err != nil {
			c.SendInvalidParam()
			return
		}
		Debug(messageJSON)
		c.replyCoach(messageJSON)
	case redis.Subscription:
		Debug("ok. subscribed")
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
		return
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
