package goboxer

import (
	"encoding/json"
	"github.com/garyburd/redigo/redis"
)

type CoachListener struct {
	Boxer        *Boxer
	Pool         *redis.Pool
	BoxerPubSub  redis.PubSubConn
	BoxerChannel string
	CoachChannel string
}

func (c *CoachListener) Listen() {
	switch msg := c.BoxerPubSub.Receive().(type) {
	case redis.Message:
		var messageJSON string
		var err error
		message := Message{}
		logger.Println("Message: ", string(msg.Data))
		if err = json.Unmarshal(msg.Data, &message); err != nil {
			logger.Println("ERROR: Unmarshal ", err)
			c.SendInvalidParam()
			return
		}
		returnedMessage := c.Boxer.Receive(message)
		if messageJSON, err = ToJSON(returnedMessage); err != nil {
			logger.Println("ERROR: ToJSON ", err)
			c.SendInvalidParam()
			return
		}
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
	message := c.Boxer.NewAdvertiseMessage()
	messageJSON, err := ToJSON(message)
	if err != nil {
		c.SendInvalidParam()
		return
	}
	c.replyCoach(messageJSON)
}

func (c *CoachListener) SendInvalidParam() {
	message := c.Boxer.NewInvalidParamMessage()
	messageJSON, err := ToJSON(message)
	if err != nil {
		c.replyCoach("{\"success\":false}")
		return
	}
	c.replyCoach(messageJSON)
}

func (c *CoachListener) replyCoach(msg string) {
	conn := c.Pool.Get()
	defer conn.Close()
	conn.Do("PUBLISH", c.CoachChannel, msg)
}
