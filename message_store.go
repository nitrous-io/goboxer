package goboxer

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"io"
)

// Message store. All message waiting to be executed should be stored here.
const STORE = "aio:messages"

// Job queue. All message that is executing should be stored here
//
// This is a hash of auto-generated id : message pairs
const JOB_QUEUE = "aio:jobs"

type messageHandler func(msg Message) Message

type MessageStore struct {
	Pool    *redis.Pool
	Handler messageHandler
}

func (s *MessageStore) Push(msg Message) error {
	conn := s.Pool.Get()
	defer conn.Close()
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = conn.Do("LPUSH", STORE, string(data))
	return err
}

func (s *MessageStore) Pop() (Message, error) {
	var msg Message
	conn := s.Pool.Get()
	defer conn.Close()
	data, err := redis.Strings(conn.Do("BRPOP", STORE, 0))
	if err != nil {
		return msg, err
	}
	err = json.Unmarshal([]byte(data[1]), &msg)
	return msg, err
}

// Light-wrapper around Process so called dont have to setup a separate channel
// Use this instead of Process if you are consumer of MessageStore
func (s *MessageStore) Listen(notify chan Message) {
	in := make(chan Message, 10)

	go func() {
		s.Process(in, notify)
	}()

	for {
		msg, _ := s.Pop()
		in <- msg
	}
}

// Fetch the latest messages to be worker on, then work on it, then return
// the replyMessage to the supplied channel.
func (s *MessageStore) Process(in chan Message, out chan Message) error {
	errChan := make(chan error, 1)
	// processing message from the input queue and pass the response back to
	// the output queue
	go func() {
		for {
			msg, ok := <-in
			if !ok {
				errChan <- fmt.Errorf("store: failed to read. Maybe closed?")
			} else {
				out <- s.handleMessage(msg)
			}
		}
	}()

	select {
	case err := <-errChan:
		return err
	}
}

// read all existing items in the job queue and process them all
// FIXME: this may not respect order of the messages. But maybe we dont really
// care about them when we want to retry all messages ??
func (s *MessageStore) Retry() ([]Message, error) {
	response := []Message{}
	conn := s.Pool.Get()
	defer conn.Close()
	values, err := redis.Values(conn.Do("HGETALL", JOB_QUEUE))
	if err != nil {
		return response, err
	}
	for i := 0; i < len(values); i += 2 {
		id := values[i].([]byte)
		data := values[i+1].([]byte)
		msg := Message{}
		if err := json.Unmarshal(data, &msg); err != nil {
			return response, err
		}
		resp := s.Handler(msg)
		response = append(response, resp)
		conn.Do("HDEL", JOB_QUEUE, id)
	}
	return response, nil
}

func (s *MessageStore) handleMessage(msg Message) Message {
	id := generateId()
	conn := s.Pool.Get()
	defer func() {
		conn.Do("HDEL", JOB_QUEUE, id)
		conn.Close()
	}()
	data, _ := json.Marshal(msg)
	conn.Do("HSET", JOB_QUEUE, id, string(data))
	resp := s.Handler(msg)
	return resp
}

func generateId() string {
	id := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(id)
}
