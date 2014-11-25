package goboxer

import (
	"encoding/json"

	"github.com/nitrous-io/goboxer/utils"
	"github.com/garyburd/redigo/redis"
)

type NotHandleError struct{}

func (e NotHandleError) Error() string { return "NotHandle" }

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
		go func() {
			var messageJSON string
			var err error
			returnedMessage, err := c.Dispatch(msg.Data)
			if err != nil {
				switch err.(type) {
				case NotHandleError:
					return
				default:
					c.SendInvalidParam()
					return
				}
			}
			if messageJSON, err = utils.ToJSON(returnedMessage); err != nil {
				logger.Println("ERROR: ToJSON ", err)
				c.SendInvalidParam()
				return
			}
			logger.Println("Response:", messageJSON)
			c.replyCoach(messageJSON)
		}()
	case redis.Subscription:
		logger.Println("ok. subscribed")
		if msg.Count == 0 {
			return
		}
	}
}

func (c *CoachListener) Dispatch(data []byte) (message Message, err error) {
	if err = json.Unmarshal(data, &message); err != nil {
		logger.Println("ERROR: Unmarshal", err)
		return
	}
	if message.Id != c.Boxer.Id {
		logger.Println("Message ignored:", string(data))
		err = NotHandleError{}
		return
	}
	logger.Println("Message received:", string(data))
	msgChan := c.Boxer.ReceiveAsync(message)
	message = <-msgChan
	return
}

// Advertise itself to Coach
func (c *CoachListener) Advertise() {
	message := c.Boxer.NewAdvertiseMessage()
	messageJSON, err := utils.ToJSON(message)
	if err != nil {
		c.SendInvalidParam()
		return
	}
	c.replyCoach(messageJSON)
}

func (c *CoachListener) SendInvalidParam() {
	message := c.Boxer.NewInvalidParamMessage()
	messageJSON, err := utils.ToJSON(message)
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
