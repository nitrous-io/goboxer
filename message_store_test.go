package goboxer

import (
	"encoding/json"
	"github.com/garyburd/redigo/redis"
	. "github.com/motain/gocheck"
	"time"
)

var handler = func(msg Message) Message {
	return msg
}

type MessageStoreSuite struct {
	Redis *redis.Pool
}

var _ = Suite(&MessageStoreSuite{})

func (s *MessageStoreSuite) SetUpTest(c *C) {
	s.Redis = NewRedisPool(":6379")
	conn := s.Redis.Get()
	defer conn.Close()
	conn.Do("FLUSHDB")
}

func (s *MessageStoreSuite) TearDownTest(c *C) {
	s.Redis.Close()
}

func (s *MessageStoreSuite) TestPushPop(c *C) {
	var err error
	conn := s.Redis.Get()
	defer conn.Close()

	ms := &MessageStore{Pool: s.Redis}
	err = ms.Push(Message{Id: "3"})
	c.Assert(err, IsNil)

	l, _ := redis.Int(conn.Do("LLEN", "aio:messages"))
	c.Assert(l, Equals, 1)

	msg, _ := ms.Pop()
	c.Log(msg)
	c.Assert(msg.Id, Equals, "3")

	l, _ = redis.Int(conn.Do("LLEN", "aio:messages"))
	c.Assert(l, Equals, 0)
}

func (s *MessageStoreSuite) TestPopShouldBlock(c *C) {
	ms := &MessageStore{Pool: s.Redis}
	resp := make(chan Message, 1)

	go func() {
		msg, _ := ms.Pop()
		resp <- msg
	}()

	go func() {
		// sleep a bit so we are sure that RPOP will fail
		time.Sleep(10 * time.Millisecond)
		ms.Push(Message{Id: "1"})
	}()

	select {
	case msg := <-resp:
		c.Assert(msg.Id, Equals, "1")
	case <-time.After(1 * time.Second):
		c.Fatal("failed to get response")
		c.FailNow()
	}
}

func (s *MessageStoreSuite) TestProcess(c *C) {
	conn := s.Redis.Get()
	defer conn.Close()

	ms := &MessageStore{
		Pool: s.Redis,
		Handler: func(msg Message) Message {
			// assert that the message is still in the job queue
			l, _ := redis.Int(conn.Do("HLEN", JOB_QUEUE))
			c.Assert(l, Equals, 1)
			msg.Id = "2"
			return msg
		},
	}

	in := make(chan Message, 1)
	resp := make(chan Message, 1)
	go func() {
		ms.Process(in, resp)
	}()

	go func() {
		ms.Push(Message{Id: "1"})
		msg, _ := ms.Pop()
		in <- msg
	}()

	select {
	case msg := <-resp:
		c.Assert(msg.Id, Equals, "2")
		// the message is no longer in the queue
		l, _ := redis.Int(conn.Do("HLEN", JOB_QUEUE))
		c.Assert(l, Equals, 0)
		close(in)
	case <-time.After(1 * time.Second):
		c.Fatal("failed to get response")
		close(in)
		c.FailNow()
	}
}

func (s *MessageStoreSuite) TestRetry(c *C) {
	conn := s.Redis.Get()
	defer conn.Close()

	ms := &MessageStore{
		Pool: s.Redis,
		Handler: func(msg Message) Message {
			msg.Id += "-done"
			return msg
		},
	}

	msgs := []Message{
		Message{Id: "1"},
		Message{Id: "1"},
	}

	// populate the job queue
	for _, msg := range msgs {
		id := generateId()
		data, _ := json.Marshal(msg)
		conn.Do("HSET", JOB_QUEUE, id, string(data))
	}

	resp, _ := ms.Retry()
	c.Assert(len(resp), Equals, 2)
	c.Assert(resp[0].Id, Equals, "1-done")

	// the message is no longer in the queue
	l, _ := redis.Int(conn.Do("HLEN", JOB_QUEUE))
	c.Assert(l, Equals, 0)
}
