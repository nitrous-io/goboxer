package runtime

import (
	"github.com/bitly/go-simplejson"
	"github.com/bmizerany/assert"
	"github.com/garyburd/redigo/redis"
	"sync"
	"testing"
)

type ConfigureMessageTest struct {
	Type string `json:"type" redis:"type" `
	Id   int    `json:"id" redis:"id" `
	Done bool   `json:"done" redis:"done" `
}

func configure(message *simplejson.Json) (Message, error) {
	id, _ := message.Get("id").Int()
	messageType, _ := message.Get("type").String()
	m := &ConfigureMessageTest{
		Id:   id,
		Type: messageType,
		Done: true,
	}
	return m, nil
}

func runtimeTearDown(conn redis.Conn, conns ...redis.Conn) {
	conn.Do("FLUSHDB")
	conn.Close()
	for _, c := range conns {
		c.Close()
	}
}

func newTestRuntime(boxerConn, coachConn redis.Conn) *Runtime {
	handles := map[string]Handler{
		"aio:configure": configure,
	}
	return &Runtime{
		BoxerConn: boxerConn,
		CoachConn: coachConn,
		Handles:   handles,
	}
}

func TestRuntimeHandle(t *testing.T) {
	conn, _ := redis.Dial("tcp", ":65000")
	boxerConn, _ := redis.Dial("tcp", ":65000")
	coachConn, _ := redis.Dial("tcp", ":65000")
	coachTestConn, _ := redis.Dial("tcp", ":65000")
	conn.Do("FLUSHDB")
	defer runtimeTearDown(conn, boxerConn, coachConn, coachTestConn)
	runtime := newTestRuntime(boxerConn, coachConn)

	// coachPSC is a test connection that receive message that boxer sends to coach
	coachPSC := redis.PubSubConn{coachTestConn}
	configureMessage := &ConfigureMessageTest{
		Type: "aio:configure",
		Id:   123,
		Done: false,
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// This goroutine receives and prints pushed notifications from the server.
	// The goroutine exits when there is an error.
	go func() {
		defer wg.Done()
		for {
			switch n := coachPSC.Receive().(type) {
			case redis.Message:
				m, _ := simplejson.NewJson(n.Data)
				done, _ := m.Get("done").Bool()
				id, _ := m.Get("id").Int()
				messageType, _ := m.Get("type").String()
				assert.Equal(t, done, true)
				assert.Equal(t, id, configureMessage.Id)
				assert.Equal(t, messageType, configureMessage.Type)
				return
			case error:
				t.Errorf("Received error %v", n)
				return
			}
		}
	}()

	// This goroutine manages subscriptions for the connection.
	go func() {
		defer wg.Done()
		runtime.Start()
		defer runtime.End()

		coachPSC.Subscribe("aio-coach-channel")
		m, _ := MessageJSON(configureMessage)
		conn.Do("PUBLISH", "aio-boxer-channel", m)
		coachPSC.Unsubscribe()
	}()

	wg.Wait()
}
