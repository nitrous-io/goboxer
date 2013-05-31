package goboxer

import (
	"github.com/bitly/go-simplejson"
	"github.com/garyburd/redigo/redis"
)

type CoachListener struct {
	Runtime      Runtime
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
			c.CoachConn.Do("PUBLISH", c.CoachChannel, "failed")
		}
		returnedMessage, err := c.Runtime.Receive(json)
		if err != nil {
			c.CoachConn.Do("PUBLISH", c.CoachChannel, "failed")
		}
		messageJSON, err := MessageJSON(returnedMessage)
		if err != nil {
			c.CoachConn.Do("PUBLISH", c.CoachChannel, "failed")
		}
		c.CoachConn.Do("PUBLISH", c.CoachChannel, messageJSON)
	case redis.Subscription:
		if msg.Count == 0 {
			return
		}
	}
}

// Advertise itself to Coach
func (c *CoachListener) Advertise() {
	message := c.Runtime.NewAdvertiseMessage()
	messageJSON, err := MessageJSON(message)
	Debug(messageJSON)
	if err != nil {
		c.CoachConn.Do("PUBLISH", c.CoachChannel, "failed")
	}
	c.CoachConn.Do("PUBLISH", c.CoachChannel, messageJSON)
}
