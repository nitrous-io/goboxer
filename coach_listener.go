package goboxer

import (
	"encoding/json"

	"github.com/action-io/goboxer/utils"
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
	Store        *MessageStore
}

func (c *CoachListener) Listen() {
	// initialize the store
	if c.Store == nil {
		c.Store = &MessageStore{
			Pool:    c.Pool,
			Handler: boxerHandler(c),
		}
	}

	// we will reply all coach message using this channel
	notify := make(chan Message, 10)
	go c.reply(notify)

	// first retry all current messages in the store
	c.retryOldJobs(notify)

	// Listen to new messages now
	go c.Store.Listen(notify)

	switch msg := c.BoxerPubSub.Receive().(type) {
	case redis.Message:
		go c.Dispatch(notify, msg.Data)
	case redis.Subscription:
		logger.Println("ok. subscribed")
		if msg.Count == 0 {
			return
		}
	}
}

// parse the json messages,
// - if the message is invalid, notify that this is an invalid param msg
// - if the message is for another boxer, does not do anything
// - otherwise, push this message into the store, so message store can pick it
// up for the job queue
func (c *CoachListener) Dispatch(notify chan Message, data []byte) (err error) {
	var message Message
	if err = json.Unmarshal(data, &message); err != nil {
		logger.Println("ERROR: Unmarshal", err)
		notify <- c.Boxer.NewInvalidParamMessage()
		return
	}
	if message.Id != c.Boxer.Id {
		logger.Println("Message ignored:", string(data))
		err = NotHandleError{}
		return
	}
	logger.Println("Message received:", string(data))
	return c.Store.Push(message)
}

// handle the communication from boxer to coach. This gets all replied message
// and forward it to coach
func (c *CoachListener) reply(notify chan Message) {
	for res := range notify {
		messageJSON, err := utils.ToJSON(res)
		if err != nil {
			logger.Println("ERROR: ToJSON ", err)
			c.SendInvalidParam()
			return
		}
		logger.Println("Response:", messageJSON)
		c.replyCoach(messageJSON)
	}
}

// retry all current messages in the job queue
func (c *CoachListener) retryOldJobs(notify chan Message) {
	messages, err := c.Store.Retry()
	if err != nil {
		logger.Println("ERROR: failed to retry all messages", err)
	}
	for _, msg := range messages {
		notify <- msg
	}
}

func boxerHandler(c *CoachListener) messageHandler {
	return func(msg Message) Message {
		msgChan := c.Boxer.ReceiveAsync(msg)
		return <-msgChan
	}
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
	_, err := conn.Do("PUBLISH", c.CoachChannel, msg)
	if err != nil {
		logger.Printf("coach: Failed to publish %s to coach, err: %v", msg, err)
	}
}
